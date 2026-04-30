package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAll(t *testing.T) {
	// Find the spec dir relative to repo root
	specDir := findSpecDir(t)
	if specDir == "" {
		t.Skip("spec dir not found")
	}

	catalogs, err := LoadAll(specDir)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if len(catalogs) < 1 {
		t.Fatalf("expected at least 1 catalog, got %d", len(catalogs))
	}

	for _, cat := range catalogs {
		t.Logf("Catalog %q: %d components", cat.CatalogID, len(cat.Components))
		for name := range cat.Components {
			t.Logf("  - %s", name)
		}
	}

	merged := MergeCatalogs(catalogs)
	t.Logf("Merged: %d total components", len(merged.Components))

	if len(merged.Components) == 0 {
		t.Fatal("merged catalog has no components")
	}
}

func TestLoadFromCatalogFile(t *testing.T) {
	specDir := findSpecDir(t)
	if specDir == "" {
		t.Skip("spec dir not found")
	}

	minimalPath := filepath.Join(specDir, "catalogs", "minimal", "minimal_catalog.json")
	if _, err := os.Stat(minimalPath); os.IsNotExist(err) {
		t.Skip("minimal catalog not found")
	}

	cat, err := LoadFromCatalogFile(minimalPath)
	if err != nil {
		t.Fatalf("LoadFromCatalogFile: %v", err)
	}

	if len(cat.Components) == 0 {
		t.Fatal("minimal catalog has no components")
	}

	t.Logf("Minimal catalog %q: %d components", cat.CatalogID, len(cat.Components))
	for name := range cat.Components {
		t.Logf("  - %s", name)
	}
}

func findSpecDir(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 10; i++ {
		candidate := filepath.Join(dir, "..", "..", "..", "specification", "v0_9", "json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
