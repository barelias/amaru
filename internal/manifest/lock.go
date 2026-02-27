package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const LockFile = "amaru.lock"

type Lock struct {
	LockedAt string                  `json:"locked_at"`
	Skills   map[string]LockedEntry  `json:"skills,omitempty"`
	Commands map[string]LockedEntry  `json:"commands,omitempty"`
}

type LockedEntry struct {
	Version     string `json:"version"`
	Registry    string `json:"registry"`
	Hash        string `json:"hash"`
	InstalledAt string `json:"installed_at"`
}

// LoadLock reads and parses amaru.lock from the given directory.
func LoadLock(dir string) (*Lock, error) {
	path := filepath.Join(dir, LockFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Lock{
				Skills:   make(map[string]LockedEntry),
				Commands: make(map[string]LockedEntry),
			}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", LockFile, err)
	}

	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", LockFile, err)
	}
	if l.Skills == nil {
		l.Skills = make(map[string]LockedEntry)
	}
	if l.Commands == nil {
		l.Commands = make(map[string]LockedEntry)
	}
	return &l, nil
}

// SaveLock writes amaru.lock to the given directory.
func SaveLock(dir string, l *Lock) error {
	l.LockedAt = time.Now().UTC().Format(time.RFC3339)
	path := filepath.Join(dir, LockFile)
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", LockFile, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// NewLockedEntry creates a new lock entry with the current timestamp.
func NewLockedEntry(version, registry, hash string) LockedEntry {
	return LockedEntry{
		Version:     version,
		Registry:    registry,
		Hash:        hash,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
}
