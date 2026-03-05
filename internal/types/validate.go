package types

import (
	"fmt"
	"regexp"
)

var validNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

// ValidateItemName checks that a name is safe for directories, git tags, and JSON keys.
// Rules: lowercase alphanumeric + hyphens, starts with letter, 2-64 chars.
func ValidateItemName(name string) error {
	if name == "" {
		return fmt.Errorf("item name cannot be empty")
	}
	if !validNamePattern.MatchString(name) {
		return fmt.Errorf("invalid item name %q: must be 2-64 chars, lowercase alphanumeric and hyphens, starting with a letter", name)
	}
	return nil
}
