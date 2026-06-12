package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
	"github.com/kubepilot/backend/internal/models"
	"github.com/kubepilot/backend/internal/store"
	"go.uber.org/zap"
)

const imageTagCacheTTL = 30 * time.Minute

// ImageWatcher polls container registries to detect new image tags.
type ImageWatcher struct {
	store      *store.Store
	httpClient *http.Client
	logger     *zap.Logger
	stopCh     chan struct{}
}

// NewImageWatcher creates a new ImageWatcher.
func NewImageWatcher(s *store.Store, logger *zap.Logger) *ImageWatcher {
	return &ImageWatcher{
		store: s,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Run starts the image watcher loop.
func (iw *ImageWatcher) Run(ctx context.Context, interval time.Duration) {
	iw.logger.Info("image watcher started")
	iw.check(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			iw.check(ctx)
		case <-iw.stopCh:
			iw.logger.Info("image watcher stopped")
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop signals the watcher to stop.
func (iw *ImageWatcher) Stop() {
	close(iw.stopCh)
}

func (iw *ImageWatcher) check(ctx context.Context) {
	images, err := iw.store.ListAllContainerImages(ctx)
	if err != nil {
		iw.logger.Error("image watcher: list images", zap.Error(err))
		return
	}

	for _, img := range images {
		img := img
		if err := iw.CheckImage(ctx, &img); err != nil {
			// Private/auth-gated registries (self-signed TLS, 401/403), unreachable
			// hosts and orphaned references are expected in many environments and
			// would otherwise flood the logs every cycle — keep them at debug.
			if isExpectedImageError(err) {
				iw.logger.Debug("image watcher: skipped image",
					zap.String("image", img.Image),
					zap.Error(err),
				)
			} else {
				iw.logger.Warn("image watcher: check image",
					zap.String("image", img.Image),
					zap.Error(err),
				)
			}
		}
	}
}

// isExpectedImageError reports whether err is an environmental/expected failure
// (registry auth, TLS trust, unreachable host, orphaned workload) rather than a
// genuine bug worth a warning.
func isExpectedImageError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"certificate signed by unknown authority",
		"failed to verify certificate",
		"x509:",
		"registry returned 401",
		"registry returned 403",
		"unauthorized",
		"authentication required",
		"\"code\":\"denied\"",
		"forbidden",
		"record not found",
		"connection refused",
		"no such host",
		"i/o timeout",
		"deadline exceeded",
		"connectex:", // windows connect failure
		"wsarecv",     // windows connection aborted
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// CheckImage fetches available tags for the image from its registry and creates
// an UpdateFinding if a newer version is detected.
func (iw *ImageWatcher) CheckImage(ctx context.Context, image *models.ContainerImage) error {
	cacheKey := fmt.Sprintf("imgtags:%s/%s", image.Registry, image.Repository)

	// Try cache first.
	cachedTags, err := iw.getCachedTags(ctx, cacheKey)
	if err != nil || cachedTags == nil {
		// Fetch from registry.
		tags, fetchErr := iw.fetchTags(ctx, image)
		if fetchErr != nil {
			return fmt.Errorf("fetch tags for %s: %w", image.Image, fetchErr)
		}
		cachedTags = tags
		iw.cacheTags(ctx, cacheKey, tags)
	}

	// Determine whether a newer version exists.
	newerTag, updateType := findNewerTag(image.Tag, cachedTags)
	if newerTag == "" {
		// No update available — resolve any finding that was previously open for this image.
		if err := iw.store.ResolveActiveFindingForImage(ctx, image.ID); err != nil {
			iw.logger.Warn("resolve image finding", zap.String("image", image.Image), zap.Error(err))
		}
		return nil
	}

	// Record the observation.
	obs := &models.ImageTagObservation{
		ID:         uuid.New(),
		ImageID:    image.ID,
		Tag:        newerTag,
		ObservedAt: time.Now(),
		IsLatest:   true,
	}
	if err := iw.store.UpsertImageTagObservation(ctx, obs); err != nil {
		iw.logger.Warn("upsert image tag observation", zap.Error(err))
	}

	// Fetch workload to populate the finding.
	workload, err := iw.store.GetWorkload(ctx, image.WorkloadID.String())
	if err != nil {
		return fmt.Errorf("get workload %s: %w", image.WorkloadID, err)
	}

	title := fmt.Sprintf("%s/%s: update available (%s → %s)",
		image.Registry, image.Repository, image.Tag, newerTag)

	finding := &models.UpdateFinding{
		ID:               uuid.New(),
		ClusterID:        workload.ClusterID,
		NamespaceName:    workload.NamespaceName,
		WorkloadID:       &workload.ID,
		ContainerImageID: &image.ID,
		Kind:             models.FindingKindImage,
		UpdateType:       updateType,
		Severity:         updateTypeSeverity(updateType),
		Status:           models.FindingStatusOpen,
		CurrentVersion:   image.Tag,
		LatestVersion:    newerTag,
		Title:            title,
		Description: fmt.Sprintf(
			"Container image %s/%s is at version %s; version %s is available.",
			image.Registry, image.Repository, image.Tag, newerTag,
		),
		FirstDetectedAt: time.Now(),
		LastObservedAt:  time.Now(),
	}

	return iw.store.UpsertFinding(ctx, finding)
}

// fetchTags calls the OCI registry API to list available tags.
func (iw *ImageWatcher) fetchTags(ctx context.Context, image *models.ContainerImage) ([]string, error) {
	registry := image.Registry
	if registry == "docker.io" {
		registry = "registry-1.docker.io"
	}

	url := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, image.Repository)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Support basic auth if registry credentials are configured.
	if image.ImageRegistry != nil {
		var authCfg map[string]string
		if err := json.Unmarshal(image.ImageRegistry.AuthConfig, &authCfg); err == nil {
			if u, ok := authCfg["username"]; ok {
				if p, ok := authCfg["password"]; ok {
					req.SetBasicAuth(u, p)
				}
			}
		}
	}

	resp, err := iw.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Docker Hub returns 401 for public images without a token; get one and retry.
	if resp.StatusCode == http.StatusUnauthorized {
		token, tokenErr := iw.getDockerHubToken(ctx, image.Repository)
		if tokenErr != nil {
			return nil, fmt.Errorf("docker hub auth: %w", tokenErr)
		}
		resp.Body.Close()

		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		resp, err = iw.httpClient.Do(req2)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("registry returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tags, nil
}

// getDockerHubToken retrieves an anonymous Bearer token for Docker Hub.
func (iw *ImageWatcher) getDockerHubToken(ctx context.Context, repository string) (string, error) {
	// Ensure repository is prefixed for official images.
	if !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	url := fmt.Sprintf(
		"https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull",
		repository,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := iw.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Token, nil
}

// getCachedTags returns cached tag list from the cache, or nil if not found.
func (iw *ImageWatcher) getCachedTags(ctx context.Context, key string) ([]string, error) {
	val, err := iw.store.Cache.Get(ctx, key)
	if err != nil {
		return nil, nil // Cache miss is not an error.
	}
	var tags []string
	if err := json.Unmarshal([]byte(val), &tags); err != nil {
		return nil, nil
	}
	return tags, nil
}

// cacheTags stores a tag list in the cache with the default TTL.
func (iw *ImageWatcher) cacheTags(ctx context.Context, key string, tags []string) {
	data, err := json.Marshal(tags)
	if err != nil {
		return
	}
	_ = iw.store.Cache.Set(ctx, key, data, imageTagCacheTTL)
}

// findNewerTag compares the current tag against the available list and returns
// the highest newer semver tag, or empty string if no update is found.
func findNewerTag(currentTag string, availableTags []string) (string, string) {
	currentSV, err := semver.NewVersion(currentTag)
	if err != nil {
		// Non-semver tag (e.g., "latest", "main") — we can't compare versions.
		return "", models.UpdateTypeUnknown
	}

	var best *semver.Version
	for _, t := range availableTags {
		sv, err := semver.NewVersion(t)
		if err != nil {
			continue
		}
		// Skip pre-release tags unless the current tag is also pre-release.
		if sv.Prerelease() != "" && currentSV.Prerelease() == "" {
			continue
		}
		if sv.GreaterThan(currentSV) {
			if best == nil || sv.GreaterThan(best) {
				best = sv
			}
		}
	}

	if best == nil {
		return "", ""
	}

	updateType := classifyUpdate(currentSV, best)
	return best.Original(), updateType
}

// classifyUpdate determines whether the jump is a patch, minor or major update.
func classifyUpdate(from, to *semver.Version) string {
	if to.Major() > from.Major() {
		return models.UpdateTypeMajor
	}
	if to.Minor() > from.Minor() {
		return models.UpdateTypeMinor
	}
	return models.UpdateTypePatch
}

// updateTypeSeverity maps an update type to a default severity.
func updateTypeSeverity(updateType string) string {
	switch updateType {
	case models.UpdateTypeMajor:
		return models.SeverityHigh
	case models.UpdateTypeMinor:
		return models.SeverityMedium
	case models.UpdateTypePatch:
		return models.SeverityLow
	default:
		return models.SeverityInfo
	}
}
