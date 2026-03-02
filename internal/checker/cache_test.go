package checker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCache(t *testing.T) {
	dir := t.TempDir()
	result := &CheckResult{
		Updates: []UpdateInfo{
			{Name: "research", ItemType: "skill", Current: "1.0.0", Latest: "1.1.0", Category: "minor"},
		},
		Drifts:   []DriftInfo{},
		UpToDate: 2,
	}

	if err := SaveCache(dir, result); err != nil {
		t.Fatalf("SaveCache error: %v", err)
	}

	loaded := LoadCache(dir)
	if loaded == nil {
		t.Fatal("expected non-nil cache result")
	}
	if len(loaded.Updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(loaded.Updates))
	}
	if loaded.Updates[0].Name != "research" {
		t.Errorf("expected research, got %s", loaded.Updates[0].Name)
	}
	if loaded.UpToDate != 2 {
		t.Errorf("expected 2 up-to-date, got %d", loaded.UpToDate)
	}
}

func TestLoadCacheExpired(t *testing.T) {
	dir := t.TempDir()

	// Write a cache entry with a timestamp in the past (beyond TTL)
	entry := CacheEntry{
		CheckedAt: time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339),
		Result:    &CheckResult{UpToDate: 1},
	}
	data, _ := json.MarshalIndent(entry, "", "  ")
	cacheDir := filepath.Join(dir, cacheDir)
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, cacheFile), data, 0644)

	loaded := LoadCache(dir)
	if loaded != nil {
		t.Error("expected nil for expired cache")
	}
}

func TestLoadCacheMissing(t *testing.T) {
	dir := t.TempDir()
	loaded := LoadCache(dir)
	if loaded != nil {
		t.Error("expected nil for missing cache")
	}
}

func TestLoadCacheCorrupt(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, cacheDir)
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, cacheFile), []byte("not json"), 0644)

	loaded := LoadCache(dir)
	if loaded != nil {
		t.Error("expected nil for corrupt cache")
	}
}
