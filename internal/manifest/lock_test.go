package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLockNotExist(t *testing.T) {
	dir := t.TempDir()
	lock, err := LoadLock(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lock == nil {
		t.Fatal("expected non-nil lock")
	}
	if len(lock.Skills) != 0 || len(lock.Commands) != 0 {
		t.Error("expected empty maps for missing lock file")
	}
}

func TestSaveAndLoadLock(t *testing.T) {
	dir := t.TempDir()
	lock := &Lock{
		Skills: map[string]LockedEntry{
			"research": {
				Version:     "1.0.3",
				Registry:    "main",
				Hash:        "a1b2c3d4e5f6",
				InstalledAt: "2026-02-25T10:00:00Z",
			},
		},
		Commands: map[string]LockedEntry{
			"dev/bootstrap": {
				Version:     "2.0.0",
				Registry:    "main",
				Hash:        "c3d4e5f6a1b2",
				InstalledAt: "2026-02-26T09:00:00Z",
			},
		},
	}

	if err := SaveLock(dir, lock); err != nil {
		t.Fatalf("SaveLock error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, LockFile)); os.IsNotExist(err) {
		t.Fatal("lock file not created")
	}

	loaded, err := LoadLock(dir)
	if err != nil {
		t.Fatalf("LoadLock error: %v", err)
	}
	if loaded.Skills["research"].Version != "1.0.3" {
		t.Errorf("expected 1.0.3, got %s", loaded.Skills["research"].Version)
	}
	if loaded.Skills["research"].Hash != "a1b2c3d4e5f6" {
		t.Errorf("expected hash a1b2c3d4e5f6, got %s", loaded.Skills["research"].Hash)
	}
	if loaded.Commands["dev/bootstrap"].Version != "2.0.0" {
		t.Errorf("expected 2.0.0, got %s", loaded.Commands["dev/bootstrap"].Version)
	}
	if loaded.LockedAt == "" {
		t.Error("expected locked_at to be set")
	}
}
