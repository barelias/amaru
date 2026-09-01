package ctxdocs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/vcs"
)

const (
	// CloneDir is where the sparse context checkout lives inside the project.
	CloneDir = ".claude/.amaru-context"
)

// Config holds the resolved context configuration.
type Config struct {
	Registry  manifest.RegistryConfig
	RegAlias  string
	Project   string
	LocalPath string // Where context docs are symlinked (e.g. "docs/context")
}

// ResolveConfig reads context configuration from the manifest.
func ResolveConfig(m *manifest.Manifest) (*Config, error) {
	if m.Context == nil {
		return nil, fmt.Errorf("no context configuration in amaru.json")
	}

	regAlias := m.Context.Registry
	reg, ok := m.Registries[regAlias]
	if !ok {
		return nil, fmt.Errorf("context registry %q not found in manifest", regAlias)
	}

	localPath := m.Context.Path
	if localPath == "" {
		localPath = "docs/context"
	}

	return &Config{
		Registry:  reg,
		RegAlias:  regAlias,
		Project:   m.Context.Project,
		LocalPath: localPath,
	}, nil
}

// RepoURL converts the registry URL format to a cloneable URL.
func (c *Config) RepoURL() (string, error) {
	url := c.Registry.URL
	if strings.HasPrefix(url, "github:") {
		return "https://github.com/" + strings.TrimPrefix(url, "github:") + ".git", nil
	}
	return url, nil
}

// SparsePaths returns the paths to include in the sparse checkout for git.
// Includes both the legacy nested path and the flat v2 path so the same
// sparse checkout works against either layout — git silently skips paths
// that don't exist in the cloned repo.
func (c *Config) SparsePaths() []string {
	return []string{
		".amaru_registry/context/" + c.Project,
		"context/" + c.Project,
		"AGENTS.md",
	}
}

// resolveContextSrc returns whichever of the two candidate context source
// paths actually exists in the sparse checkout. Falls back to the legacy
// path so callers always get a non-empty result (even if the path doesn't
// exist yet — surfaces a clearer error downstream).
func resolveContextSrc(cloneTarget, project string) string {
	flat := filepath.Join(cloneTarget, "context", project)
	if _, err := os.Stat(flat); err == nil {
		return flat
	}
	return filepath.Join(cloneTarget, ".amaru_registry", "context", project)
}

// EnsureSymlink guarantees the configured local path is a symlink into the
// context checkout, layout-aware (prefers the flat v2 path, falls back to
// legacy nested). A live symlink is left alone; a missing or broken one is
// (re)created; a real file or directory at the path is an error — amaru never
// clobbers user data. Returns true when it (re)created the link.
func EnsureSymlink(projectDir string, cfg *Config) (bool, error) {
	cloneTarget := filepath.Join(projectDir, CloneDir)
	contextSrc := resolveContextSrc(cloneTarget, cfg.Project)
	contextDst := filepath.Join(projectDir, cfg.LocalPath)

	if info, err := os.Lstat(contextDst); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return false, fmt.Errorf(
				"%s exists and is not a symlink; move it aside so amaru can manage it",
				cfg.LocalPath,
			)
		}
		if _, err := os.Stat(contextDst); err == nil {
			return false, nil // live link — nothing to do
		}
		// Broken link (target moved or checkout relaid): replace it.
		if err := os.Remove(contextDst); err != nil {
			return false, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(contextDst), 0755); err != nil {
		return false, err
	}

	// Make the symlink relative for portability
	relSrc, err := filepath.Rel(filepath.Dir(contextDst), contextSrc)
	if err != nil {
		relSrc = contextSrc
	}

	if err := os.Symlink(relSrc, contextDst); err != nil {
		return false, fmt.Errorf("creating symlink: %w", err)
	}
	return true, nil
}

// Init sets up context sync for the current project. Idempotent: with the
// checkout already in place it repairs whatever is missing (the symlink
// included) instead of refusing — a live checkout with a deleted symlink used
// to dead-end here (init refused, sync didn't recreate), silently stalling
// the context channel for good.
func Init(ctx context.Context, projectDir string, cfg *Config, backend vcs.Backend) error {
	repoURL, err := cfg.RepoURL()
	if err != nil {
		return err
	}

	cloneTarget := filepath.Join(projectDir, CloneDir)

	if _, err := os.Stat(cloneTarget); err == nil {
		_, err := EnsureSymlink(projectDir, cfg)
		return err
	}

	var paths []string
	if backend.Name() == "sapling" {
		paths = []string{cfg.Project}
	} else {
		paths = cfg.SparsePaths()
	}

	if err := backend.SparseClone(ctx, repoURL, cloneTarget, paths); err != nil {
		return fmt.Errorf("sparse clone failed: %w", err)
	}

	if _, err := EnsureSymlink(projectDir, cfg); err != nil {
		return err
	}

	return nil
}

// Sync pulls latest context from the centralized repo and repairs the local
// symlink if it went missing — the pull alone can't restore it.
func Sync(ctx context.Context, projectDir string, cfg *Config, backend vcs.Backend) error {
	cloneDir := filepath.Join(projectDir, CloneDir)

	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		return fmt.Errorf("context not initialized. Run 'amaru context init' first")
	}

	if err := backend.Pull(ctx, cloneDir); err != nil {
		return err
	}

	_, err := EnsureSymlink(projectDir, cfg)
	return err
}

// Push stages, commits, and pushes local context changes.
func Push(ctx context.Context, projectDir string, cfg *Config, backend vcs.Backend, message string) error {
	cloneDir := filepath.Join(projectDir, CloneDir)

	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		return fmt.Errorf("context not initialized. Run 'amaru context init' first")
	}

	if !backend.HasChanges(ctx, cloneDir) {
		return nil // Nothing to push
	}

	// Stage whichever layout's context path actually exists in the clone.
	contextPath := "context/" + cfg.Project
	if _, err := os.Stat(filepath.Join(cloneDir, contextPath)); err != nil {
		contextPath = ".amaru_registry/context/" + cfg.Project
	}
	if err := backend.Add(ctx, cloneDir, []string{contextPath}); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	if message == "" {
		message = fmt.Sprintf("amaru: update context for %s", cfg.Project)
	}

	return backend.CommitAndPush(ctx, cloneDir, message)
}

// EnsureGitIgnore adds the context clone dir to .gitignore if not present.
func EnsureGitIgnore(projectDir string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	entry := CloneDir + "/"

	existing, err := os.ReadFile(gitignorePath)
	if err == nil {
		if strings.Contains(string(existing), entry) {
			return nil
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString("\n# amaru context (sparse clone)\n" + entry + "\n")
	return err
}

// LocalPath returns the configured local path for context docs.
func LocalPath(m *manifest.Manifest) string {
	if m.Context == nil {
		return ""
	}
	if m.Context.Path != "" {
		return m.Context.Path
	}
	return "docs/context"
}
