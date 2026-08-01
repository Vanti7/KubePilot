package watcher

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/kubepilot/backend/internal/models"
)

// maxMajorGap bounds how far above the current version line a candidate major may
// sit. A single repository often carries stray tags from an unrelated numbering
// scheme — docker.io/library/mysql publishes a "26.7" next to the 5.x–9.x lines —
// and those win every raw semver comparison, producing a permanent false positive.
const maxMajorGap = 5

// tagShape describes the lexical form of a tag. Two tags are only comparable when
// their shapes match: "v2.14.3" and "4.0" are both valid semver but belong to
// different lines of goharbor/redis-photon (the Harbor release vs the bundled
// Redis version), and "8.0" is a rolling tag that must not be compared to "8.0.36".
type tagShape struct {
	prefix   string // leading non-digit run: "v", "release-", ""
	segments int    // dot-separated numeric components: 2 for "8.0", 3 for "2.14.3"
	variant  string // build flavour, digits stripped: "alpine", "oraclelinux"
}

var tagCoreRe = regexp.MustCompile(`^([^0-9]*)([0-9]+(?:\.[0-9]+)*)(.*)$`)

// prereleaseWords are suffix tokens that mark a pre-release of the same build
// rather than a distinct flavour. Everything else ("alpine", "oraclelinux",
// "ubi") identifies a variant that a candidate tag must match exactly, otherwise
// an alpine deployment gets offered a debian image as its "update".
var prereleaseWords = map[string]bool{
	"rc":       true,
	"alpha":    true,
	"beta":     true,
	"dev":      true,
	"pre":      true,
	"preview":  true,
	"snapshot": true,
	"next":     true,
	"canary":   true,
	"nightly":  true,
}

// parseTagShape extracts the comparable shape of a tag. It reports false for tags
// with no numeric component at all ("latest", "main").
func parseTagShape(tag string) (tagShape, bool) {
	m := tagCoreRe.FindStringSubmatch(tag)
	if m == nil {
		return tagShape{}, false
	}

	shape := tagShape{
		prefix:   m[1],
		segments: strings.Count(m[2], ".") + 1,
	}

	var variants []string
	for _, token := range splitSuffix(m[3]) {
		word := strings.TrimRight(token, "0123456789")
		// Purely numeric tokens are part of the flavour they follow
		// ("alpine3.19" → "alpine", "19"), not a flavour of their own.
		if word == "" || prereleaseWords[word] {
			continue
		}
		variants = append(variants, word)
	}
	shape.variant = strings.Join(variants, "-")

	return shape, true
}

func splitSuffix(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == '+'
	})
}

// findNewerTag compares the current tag against the available list and returns the
// highest newer tag of the same version line, or an empty string if there is none.
func findNewerTag(currentTag string, availableTags []string) (string, string) {
	currentSV, err := semver.NewVersion(currentTag)
	if err != nil {
		// Non-semver tag (e.g., "latest", "main") — we can't compare versions.
		return "", models.UpdateTypeUnknown
	}
	currentShape, ok := parseTagShape(currentTag)
	if !ok {
		return "", models.UpdateTypeUnknown
	}

	sameLine := make([]*semver.Version, 0, len(availableTags))
	for _, t := range availableTags {
		shape, ok := parseTagShape(t)
		if !ok || shape != currentShape {
			continue
		}
		sv, err := semver.NewVersion(t)
		if err != nil {
			continue
		}
		// Skip pre-release tags unless the current tag is also pre-release.
		if sv.Prerelease() != "" && currentSV.Prerelease() == "" {
			continue
		}
		sameLine = append(sameLine, sv)
	}

	reachableMajor := highestReachableMajor(currentSV.Major(), sameLine)

	var best *semver.Version
	for _, sv := range sameLine {
		if sv.Major() > reachableMajor {
			continue
		}
		if !sv.GreaterThan(currentSV) {
			continue
		}
		if best == nil || sv.GreaterThan(best) {
			best = sv
		}
	}

	if best == nil {
		return "", ""
	}

	return best.Original(), classifyUpdate(currentSV, best)
}

// highestReachableMajor walks the majors present in the tag line upward from the
// current one and stops at the first gap wider than maxMajorGap. Majors below the
// current one still count: they are what establishes the line as continuous.
func highestReachableMajor(current uint64, versions []*semver.Version) uint64 {
	seen := make(map[uint64]bool, len(versions))
	majors := make([]uint64, 0, len(versions))
	for _, v := range versions {
		if m := v.Major(); !seen[m] {
			seen[m] = true
			majors = append(majors, m)
		}
	}
	sort.Slice(majors, func(i, j int) bool { return majors[i] < majors[j] })

	reachable := current
	for _, m := range majors {
		if m <= reachable {
			continue
		}
		if m-reachable > maxMajorGap {
			break
		}
		reachable = m
	}
	return reachable
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
