package ctxdocs

import (
	"context"
	"errors"
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

// ErrNotInitialized marks a missing or unusable context checkout. Callers in
// hook position (post-commit push, post-merge sync) treat it as "nothing to
// do here" and exit clean — a git worktree never carries the checkout (it is
// gitignored and lives only in the main working tree).
var ErrNotInitialized = errors.New("context checkout not initialized")

// CheckoutState classifies what sits at CloneDir.
type CheckoutState int

const (
	// CheckoutMissing: no directory at all.
	CheckoutMissing CheckoutState = iota
	// CheckoutValid: a self-contained VCS checkout (.git or .sl present).
	CheckoutValid
	// CheckoutInvalid: a directory WITHOUT its own repository. Running VCS
	// commands with cwd here makes git discover the CONSUMER repo upward —
	// an add/commit would land context files inside the user's project
	// (measured live: a post-commit push in a worktree committed all 104
	// RFC files into the consumer). Never run backend ops against it.
	CheckoutInvalid
)

// StateOf inspects the context checkout under projectDir.
func StateOf(projectDir string) CheckoutState {
	cloneDir := filepath.Join(projectDir, CloneDir)
	if _, err := os.Stat(cloneDir); err != nil {
		return CheckoutMissing
	}
	for _, marker := range []string{".git", ".sl"} {
		if _, err := os.Lstat(filepath.Join(cloneDir, marker)); err == nil {
			return CheckoutValid
		}
	}
	return CheckoutInvalid
}

// Config holds the resolved context configuration.
type Config struct {
	Registry  manifest.RegistryConfig
	RegAlias  string
	Project   string
	LocalPath string // Where context docs are symlinked (e.g. "docs/context")
}

// ResolveConfigs reads every context mount from the manifest. All mounts
// share ONE sparse checkout (CloneDir), so they must point at the same
// registry; paths and projects must not collide.
func ResolveConfigs(m *manifest.Manifest) ([]*Config, error) {
	if len(m.Context) == 0 {
		return nil, fmt.Errorf("no context configuration in amaru.json")
	}

	seenPath := map[string]bool{}
	seenProject := map[string]bool{}
	var cfgs []*Config
	for _, mount := range m.Context {
		regAlias := mount.Registry
		reg, ok := m.Registries[regAlias]
		if !ok {
			return nil, fmt.Errorf("context registry %q not found in manifest", regAlias)
		}
		if len(cfgs) > 0 && regAlias != cfgs[0].RegAlias {
			return nil, fmt.Errorf(
				"context mounts must share one registry (a single checkout at %s): found %q and %q",
				CloneDir, cfgs[0].RegAlias, regAlias)
		}

		localPath := mount.Path
		if localPath == "" {
			localPath = "docs/context"
		}
		if seenPath[localPath] {
			return nil, fmt.Errorf("duplicate context path %q in amaru.json — each mount needs its own path", localPath)
		}
		if seenProject[mount.Project] {
			return nil, fmt.Errorf("duplicate context project %q in amaru.json", mount.Project)
		}
		seenPath[localPath] = true
		seenProject[mount.Project] = true

		cfgs = append(cfgs, &Config{
			Registry:  reg,
			RegAlias:  regAlias,
			Project:   mount.Project,
			LocalPath: localPath,
		})
	}
	return cfgs, nil
}

// FilterByProject narrows mounts to one project (the --project flag).
func FilterByProject(cfgs []*Config, project string) ([]*Config, error) {
	if project == "" {
		return cfgs, nil
	}
	for _, cfg := range cfgs {
		if cfg.Project == project {
			return []*Config{cfg}, nil
		}
	}
	return nil, fmt.Errorf("no context mount for project %q in amaru.json", project)
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

// UnionSparsePaths merges every mount's sparse paths, deduplicated in order —
// the single checkout materializes all projects at once.
func UnionSparsePaths(cfgs []*Config) []string {
	seen := map[string]bool{}
	var out []string
	for _, cfg := range cfgs {
		for _, p := range cfg.SparsePaths() {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// resolveContextSrc returns whichever of the two candidate context source
// paths actually exists in the sparse checkout (flat v2 preferred, legacy
// nested as fallback). ok=false means the project simply isn't in the
// checkout — the registry doesn't have it. The old unconditional fallback
// here is what minted dangling symlinks with .amaru_registry in the target
// when a project didn't exist yet.
func resolveContextSrc(cloneTarget, project string) (string, bool) {
	flat := filepath.Join(cloneTarget, "context", project)
	if _, err := os.Stat(flat); err == nil {
		return flat, true
	}
	legacy := filepath.Join(cloneTarget, ".amaru_registry", "context", project)
	if _, err := os.Stat(legacy); err == nil {
		return legacy, true
	}
	return flat, false
}

// EnsureSymlink guarantees the configured local path is a symlink into the
// context checkout, layout-aware (prefers the flat v2 path, falls back to
// legacy nested). A live symlink is left alone; a missing or broken one is
// (re)created; a real file or directory at the path is an error — amaru never
// clobbers user data. Returns true when it (re)created the link.
func EnsureSymlink(projectDir string, cfg *Config) (bool, error) {
	cloneTarget := filepath.Join(projectDir, CloneDir)
	contextSrc, srcExists := resolveContextSrc(cloneTarget, cfg.Project)
	contextDst := filepath.Join(projectDir, cfg.LocalPath)

	if !srcExists {
		// Never symlink into nothing. If a previous (buggy) run left OUR
		// dangling link behind, clean it up on the way out.
		if info, err := os.Lstat(contextDst); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(contextDst); err == nil && strings.Contains(target, ".amaru-context") {
				if _, err := os.Stat(contextDst); err != nil {
					_ = os.Remove(contextDst)
				}
			}
		}
		return false, fmt.Errorf(
			"project %q not found in registry %q (the checkout has no context/%s) — is it published?",
			cfg.Project, cfg.RegAlias, cfg.Project)
	}

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
func Init(ctx context.Context, projectDir string, cfgs []*Config, backend vcs.Backend) error {
	if len(cfgs) == 0 {
		return fmt.Errorf("no context mounts to initialize")
	}
	repoURL, err := cfgs[0].RepoURL()
	if err != nil {
		return err
	}

	cloneTarget := filepath.Join(projectDir, CloneDir)

	switch StateOf(projectDir) {
	case CheckoutValid:
		// Already cloned: widen the sparse profile to any mount added since,
		// then make sure every mount's symlink stands.
		if err := backend.EnsureSparsePaths(ctx, cloneTarget, sparsePathsFor(cfgs, backend)); err != nil {
			return fmt.Errorf("updating sparse paths: %w", err)
		}
		return ensureAllSymlinks(projectDir, cfgs)
	case CheckoutInvalid:
		return fmt.Errorf("%s exists but is not a repository — remove it and run 'amaru context init' again", cloneTarget)
	}

	if err := backend.SparseClone(ctx, repoURL, cloneTarget, sparsePathsFor(cfgs, backend)); err != nil {
		return fmt.Errorf("sparse clone failed: %w", err)
	}

	return ensureAllSymlinks(projectDir, cfgs)
}

// ensureAllSymlinks mounts every config, collecting per-mount failures so one
// unpublished project doesn't stop the valid mounts from working — the run
// still exits non-zero, naming each missing project.
func ensureAllSymlinks(projectDir string, cfgs []*Config) error {
	var errs []error
	for _, cfg := range cfgs {
		if _, err := EnsureSymlink(projectDir, cfg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sparsePathsFor picks the path vocabulary the backend understands.
func sparsePathsFor(cfgs []*Config, backend vcs.Backend) []string {
	if backend.Name() == "sapling" {
		var out []string
		for _, cfg := range cfgs {
			out = append(out, cfg.Project)
		}
		return out
	}
	return UnionSparsePaths(cfgs)
}

// Sync pulls latest context from the centralized repo and repairs the local
// symlink if it went missing — the pull alone can't restore it.
func Sync(ctx context.Context, projectDir string, cfgs []*Config, backend vcs.Backend) error {
	cloneDir := filepath.Join(projectDir, CloneDir)

	switch StateOf(projectDir) {
	case CheckoutMissing:
		return fmt.Errorf("%w at %s — run 'amaru context init' first", ErrNotInitialized, cloneDir)
	case CheckoutInvalid:
		return fmt.Errorf("%w: %s exists but is not a repository — refusing to run VCS commands there (they would hit the surrounding project); remove the directory and run 'amaru context init'", ErrNotInitialized, cloneDir)
	}

	// A mount added to amaru.json after init materializes on the next sync.
	if err := backend.EnsureSparsePaths(ctx, cloneDir, sparsePathsFor(cfgs, backend)); err != nil {
		return fmt.Errorf("updating sparse paths: %w", err)
	}

	if err := backend.Pull(ctx, cloneDir); err != nil {
		return err
	}

	return ensureAllSymlinks(projectDir, cfgs)
}

// Push stages, commits, and pushes local context changes.
func Push(ctx context.Context, projectDir string, cfgs []*Config, backend vcs.Backend, message string) error {
	cloneDir := filepath.Join(projectDir, CloneDir)

	switch StateOf(projectDir) {
	case CheckoutMissing:
		return fmt.Errorf("%w at %s — nothing to push", ErrNotInitialized, cloneDir)
	case CheckoutInvalid:
		return fmt.Errorf("%w: %s exists but is not a repository — refusing to run VCS commands there (they would hit the surrounding project); remove the directory and run 'amaru context init'", ErrNotInitialized, cloneDir)
	}

	if !backend.HasChanges(ctx, cloneDir) {
		return nil // Nothing to push
	}

	// Stage every mount's context path (whichever layout exists in the clone);
	// one commit carries whatever actually changed across mounts.
	var projects []string
	for _, cfg := range cfgs {
		contextPath := "context/" + cfg.Project
		if _, err := os.Stat(filepath.Join(cloneDir, contextPath)); err != nil {
			contextPath = ".amaru_registry/context/" + cfg.Project
		}
		if err := backend.Add(ctx, cloneDir, []string{contextPath}); err != nil {
			return fmt.Errorf("staging changes for %s: %w", cfg.Project, err)
		}
		projects = append(projects, cfg.Project)
	}

	if message == "" {
		message = fmt.Sprintf("amaru: update context for %s", strings.Join(projects, ", "))
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

// LocalPaths returns the configured local paths for every context mount.
func LocalPaths(m *manifest.Manifest) []string {
	var out []string
	for _, mount := range m.Context {
		if mount.Path != "" {
			out = append(out, mount.Path)
		} else {
			out = append(out, "docs/context")
		}
	}
	return out
}
