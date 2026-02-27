package checker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheDir  = ".claude"
	cacheFile = ".amaru-check-cache"
	cacheTTL  = 4 * time.Hour
)

// CacheEntry stores the result of a check for caching.
type CacheEntry struct {
	CheckedAt string       `json:"checked_at"`
	Result    *CheckResult `json:"result"`
}

// LoadCache reads the check cache from the project directory.
// Returns nil if cache is missing or expired.
func LoadCache(projectDir string) *CheckResult {
	path := filepath.Join(projectDir, cacheDir, cacheFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}

	checkedAt, err := time.Parse(time.RFC3339, entry.CheckedAt)
	if err != nil {
		return nil
	}

	if time.Since(checkedAt) > cacheTTL {
		return nil
	}

	return entry.Result
}

// SaveCache writes the check result to the cache file.
func SaveCache(projectDir string, result *CheckResult) error {
	dir := filepath.Join(projectDir, cacheDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	entry := CacheEntry{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Result:    result,
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(filepath.Join(dir, cacheFile), data, 0644)
}
