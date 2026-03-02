package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	HooksDir = ".git/hooks"
)

// PostCheckoutScript returns the content of the post-checkout hook.
func PostCheckoutScript() string {
	return `#!/bin/bash
# amaru: auto-sync context after checkout
# Installed by: amaru context init

if [ -f "amaru.json" ]; then
    if command -v amaru &> /dev/null; then
        amaru context sync 2>/dev/null || true
    fi
fi
`
}

// PostCommitScript returns the content of the post-commit hook.
func PostCommitScript() string {
	return `#!/bin/bash
# amaru: auto-push context changes after commit
# Installed by: amaru context init

if [ -f "amaru.json" ]; then
    if command -v amaru &> /dev/null; then
        CONTEXT_PATH=$(amaru context path 2>/dev/null)
        if [ -n "$CONTEXT_PATH" ] && git diff-tree --no-commit-id --name-only -r HEAD | grep -q "^${CONTEXT_PATH}"; then
            amaru context push -m "auto-push: context updated via commit hook" 2>/dev/null || true
        fi
    fi
fi
`
}

// InstallHook writes a hook script, appending to an existing hook if present.
func InstallHook(projectDir, hookName, script string) error {
	hookPath := filepath.Join(projectDir, HooksDir, hookName)

	existing, err := os.ReadFile(hookPath)
	if err == nil {
		if strings.Contains(string(existing), "amaru:") {
			return nil // Already installed
		}
		// Append to existing hook
		f, err := os.OpenFile(hookPath, os.O_APPEND|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("opening existing hook: %w", err)
		}
		defer f.Close()
		_, err = f.WriteString("\n" + script)
		return err
	}

	// Create new hook
	if err := os.MkdirAll(filepath.Dir(hookPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(hookPath, []byte(script), 0755)
}
