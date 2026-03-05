package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

const registryIndexFile = "amaru_registry.json"

// FindRegistryRoot checks that dir contains amaru_registry.json.
// Does NOT walk up — registry commands must run at the root.
func FindRegistryRoot(dir string) (string, error) {
	path := filepath.Join(dir, registryIndexFile)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("no %s found in %s (are you in a registry root?)", registryIndexFile, dir)
	}
	return dir, nil
}
