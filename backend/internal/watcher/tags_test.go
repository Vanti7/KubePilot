package watcher

import (
	"testing"

	"github.com/kubepilot/backend/internal/models"
)

func TestParseTagShape(t *testing.T) {
	tests := []struct {
		tag  string
		want tagShape
		ok   bool
	}{
		{tag: "v2.14.3", want: tagShape{prefix: "v", segments: 3}, ok: true},
		{tag: "4.0", want: tagShape{prefix: "", segments: 2}, ok: true},
		{tag: "8.0", want: tagShape{prefix: "", segments: 2}, ok: true},
		{tag: "8.0.36", want: tagShape{prefix: "", segments: 3}, ok: true},
		{tag: "26", want: tagShape{prefix: "", segments: 1}, ok: true},
		{tag: "8.0-oraclelinux9", want: tagShape{prefix: "", segments: 2, variant: "oraclelinux"}, ok: true},
		{tag: "1.27.3-alpine3.19", want: tagShape{prefix: "", segments: 3, variant: "alpine"}, ok: true},
		{tag: "v1.10.14-rc2", want: tagShape{prefix: "v", segments: 3}, ok: true},
		{tag: "v2.9.1-dev", want: tagShape{prefix: "v", segments: 3}, ok: true},
		{tag: "release-1.2.3", want: tagShape{prefix: "release-", segments: 3}, ok: true},
		{tag: "latest", ok: false},
		{tag: "main", ok: false},
	}

	for _, tc := range tests {
		got, ok := parseTagShape(tc.tag)
		if ok != tc.ok {
			t.Errorf("parseTagShape(%q) ok = %v, want %v", tc.tag, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parseTagShape(%q) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

func TestFindNewerTag(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		available  []string
		wantTag    string
		wantUpdate string
	}{
		{
			name:       "patch update",
			current:    "1.2.3",
			available:  []string{"1.2.1", "1.2.3", "1.2.4"},
			wantTag:    "1.2.4",
			wantUpdate: models.UpdateTypePatch,
		},
		{
			name:       "minor beats patch",
			current:    "1.2.3",
			available:  []string{"1.2.4", "1.3.0"},
			wantTag:    "1.3.0",
			wantUpdate: models.UpdateTypeMinor,
		},
		{
			name:       "major update",
			current:    "v2.11.0",
			available:  []string{"v2.11.1", "v3.0.0", "v3.1.2"},
			wantTag:    "v3.1.2",
			wantUpdate: models.UpdateTypeMajor,
		},
		{
			name:      "already latest",
			current:   "1.4.0",
			available: []string{"1.2.0", "1.3.0", "1.4.0"},
			wantTag:   "",
		},
		{
			name:       "non-semver current tag",
			current:    "latest",
			available:  []string{"1.0.0", "2.0.0"},
			wantTag:    "",
			wantUpdate: models.UpdateTypeUnknown,
		},
		{
			name:      "pre-releases ignored for a stable current tag",
			current:   "1.2.3",
			available: []string{"1.2.4-rc1", "1.3.0-beta.2"},
			wantTag:   "",
		},
		{
			name:       "pre-release current tag can move to stable",
			current:    "v1.2.3-rc1",
			available:  []string{"v1.2.3-rc2", "v1.2.3"},
			wantTag:    "v1.2.3",
			wantUpdate: models.UpdateTypePatch,
		},

		// Regressions observed against the Lab Cyllene cluster (2026-08-01).
		{
			name:    "goharbor/redis-photon: bundled Redis version is a different line",
			current: "v2.14.3",
			available: []string{
				"4.0", "dev", "dev-arm",
				"v1.10.0", "v1.10.1", "v2.9.5",
				"v2.14.3", "v2.14.4", "v2.14.4-rc1",
			},
			wantTag:    "v2.14.4",
			wantUpdate: models.UpdateTypePatch,
		},
		{
			name:    "mysql: stray 26.x tags do not outrank the 9.x line",
			current: "8.0",
			available: []string{
				"5.5", "5.6", "5.7", "8.0", "8.1", "8.2", "8.3", "8.4",
				"9.0", "9.5", "9.6", "9.7",
				"26", "26.7", "26.7.0", "26.7-oracle", "26.7-oraclelinux9",
			},
			wantTag:    "9.7",
			wantUpdate: models.UpdateTypeMajor,
		},
		{
			name:      "rolling two-segment tag is not compared to three-segment tags",
			current:   "8.0",
			available: []string{"8.0", "8.0.36", "8.0.42"},
			wantTag:   "",
		},
		{
			name:       "build flavour must match",
			current:    "1.27.3-alpine",
			available:  []string{"1.27.4", "1.28.0-bookworm", "1.27.4-alpine"},
			wantTag:    "1.27.4-alpine",
			wantUpdate: models.UpdateTypePatch,
		},
		{
			name:      "plain tag is not offered a flavoured update",
			current:   "1.27.3",
			available: []string{"1.27.4-alpine", "1.28.0-bookworm"},
			wantTag:   "",
		},
		{
			name:       "contiguous majors stay reachable across several steps",
			current:    "1.0.0",
			available:  []string{"2.0.0", "3.0.0", "4.0.0", "5.0.0"},
			wantTag:    "5.0.0",
			wantUpdate: models.UpdateTypeMajor,
		},
		{
			name:      "isolated far-off major is rejected",
			current:   "1.0.0",
			available: []string{"1.0.1", "42.0.0"},
			wantTag:   "1.0.1",
			// classified below
			wantUpdate: models.UpdateTypePatch,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTag, gotUpdate := findNewerTag(tc.current, tc.available)
			if gotTag != tc.wantTag {
				t.Errorf("findNewerTag(%q) tag = %q, want %q", tc.current, gotTag, tc.wantTag)
			}
			if tc.wantUpdate != "" && gotUpdate != tc.wantUpdate {
				t.Errorf("findNewerTag(%q) updateType = %q, want %q", tc.current, gotUpdate, tc.wantUpdate)
			}
		})
	}
}

func TestUpdateTypeSeverity(t *testing.T) {
	tests := map[string]string{
		models.UpdateTypeMajor:   models.SeverityHigh,
		models.UpdateTypeMinor:   models.SeverityMedium,
		models.UpdateTypePatch:   models.SeverityLow,
		models.UpdateTypeUnknown: models.SeverityInfo,
	}
	for updateType, want := range tests {
		if got := updateTypeSeverity(updateType); got != want {
			t.Errorf("updateTypeSeverity(%q) = %q, want %q", updateType, got, want)
		}
	}
}
