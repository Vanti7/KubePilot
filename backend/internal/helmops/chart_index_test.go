package helmops

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func parseIndex(t *testing.T, raw string) *chartIndex {
	t.Helper()
	var idx chartIndex
	if err := yaml.Unmarshal([]byte(raw), &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	return &idx
}

const sampleIndex = `
apiVersion: v1
entries:
  traefik:
    - name: traefik
      version: 39.0.7
      urls:
        - traefik-39.0.7.tgz
    - name: traefik
      version: 41.1.0
      urls:
        - https://example.com/charts/traefik-41.1.0.tgz
`

func TestResolveChartTarballURL(t *testing.T) {
	idx := parseIndex(t, sampleIndex)

	t.Run("relative url resolved against repo base", func(t *testing.T) {
		got, err := resolveChartTarballURL(idx, "https://traefik.github.io/charts", "traefik", "39.0.7")
		if err != nil {
			t.Fatalf("resolveChartTarballURL: %v", err)
		}
		want := "https://traefik.github.io/charts/traefik-39.0.7.tgz"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("absolute url passed through unchanged", func(t *testing.T) {
		got, err := resolveChartTarballURL(idx, "https://traefik.github.io/charts", "traefik", "41.1.0")
		if err != nil {
			t.Fatalf("resolveChartTarballURL: %v", err)
		}
		want := "https://example.com/charts/traefik-41.1.0.tgz"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("unknown chart name", func(t *testing.T) {
		_, err := resolveChartTarballURL(idx, "https://traefik.github.io/charts", "nginx", "1.0.0")
		if err == nil {
			t.Fatal("expected an error for an unknown chart name, got nil")
		}
	})

	t.Run("known chart, unknown version", func(t *testing.T) {
		_, err := resolveChartTarballURL(idx, "https://traefik.github.io/charts", "traefik", "99.0.0")
		if err == nil {
			t.Fatal("expected an error for an unknown version, got nil")
		}
	})
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		raw     string
		want    string
	}{
		{"relative, no trailing slash on repo", "https://example.com/charts", "app-1.0.0.tgz", "https://example.com/charts/app-1.0.0.tgz"},
		{"relative, trailing slash on repo", "https://example.com/charts/", "app-1.0.0.tgz", "https://example.com/charts/app-1.0.0.tgz"},
		{"absolute http", "https://example.com/charts", "http://cdn.example.com/app-1.0.0.tgz", "http://cdn.example.com/app-1.0.0.tgz"},
		{"absolute https", "https://example.com/charts", "https://cdn.example.com/app-1.0.0.tgz", "https://cdn.example.com/app-1.0.0.tgz"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveURL(tc.repoURL, tc.raw)
			if err != nil {
				t.Fatalf("resolveURL: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchChartIndexParsing(t *testing.T) {
	idx := parseIndex(t, sampleIndex)
	entries, ok := idx.Entries["traefik"]
	if !ok {
		t.Fatal("expected an entries[\"traefik\"] key")
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Version != "39.0.7" || len(entries[0].Urls) != 1 {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
}
