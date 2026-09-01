package ctxdocs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/useamaru/amaru/internal/manifest"
)

func TestResolveConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		m := &manifest.Manifest{
			Registries: map[string]manifest.RegistryConfig{
				"main": {URL: "github:acme/registry", Auth: "github"},
			},
			Context: manifest.ContextMounts{{
				Registry: "main",
				Project:  "myapp",
				Path:     "docs/ctx",
			}},
		}

		cfgs, err := ResolveConfigs(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfgs[0].RegAlias != "main" {
			t.Errorf("expected alias main, got %s", cfgs[0].RegAlias)
		}
		if cfgs[0].Project != "myapp" {
			t.Errorf("expected project myapp, got %s", cfgs[0].Project)
		}
		if cfgs[0].LocalPath != "docs/ctx" {
			t.Errorf("expected path docs/ctx, got %s", cfgs[0].LocalPath)
		}
	})

	t.Run("default path", func(t *testing.T) {
		m := &manifest.Manifest{
			Registries: map[string]manifest.RegistryConfig{
				"main": {URL: "github:acme/registry", Auth: "github"},
			},
			Context: manifest.ContextMounts{{
				Registry: "main",
				Project:  "myapp",
			}},
		}

		cfgs, err := ResolveConfigs(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfgs[0].LocalPath != "docs/context" {
			t.Errorf("expected default path docs/context, got %s", cfgs[0].LocalPath)
		}
	})

	t.Run("missing context", func(t *testing.T) {
		m := &manifest.Manifest{}
		_, err := ResolveConfigs(m)
		if err == nil {
			t.Error("expected error for nil context")
		}
	})

	t.Run("missing registry", func(t *testing.T) {
		m := &manifest.Manifest{
			Registries: map[string]manifest.RegistryConfig{},
			Context: manifest.ContextMounts{{
				Registry: "missing",
				Project:  "myapp",
			}},
		}
		_, err := ResolveConfigs(m)
		if err == nil {
			t.Error("expected error for missing registry")
		}
	})
}

func TestRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantURL string
	}{
		{"github shorthand", "github:acme/registry", "https://github.com/acme/registry.git"},
		{"plain URL", "https://example.com/repo.git", "https://example.com/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Registry: manifest.RegistryConfig{URL: tt.url}}
			got, err := cfg.RepoURL()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantURL {
				t.Errorf("RepoURL() = %s, want %s", got, tt.wantURL)
			}
		})
	}
}

func TestSparsePaths(t *testing.T) {
	cfg := &Config{Project: "myapp"}
	paths := cfg.SparsePaths()
	want := []string{
		".amaru_registry/context/myapp", // legacy (v1) — kept for back-compat
		"context/myapp",                 // flat (v2)
		"AGENTS.md",
	}
	if len(paths) != len(want) {
		t.Fatalf("expected %d paths, got %d (%v)", len(want), len(paths), paths)
	}
	for i, w := range want {
		if paths[i] != w {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], w)
		}
	}
}

func TestEnsureGitIgnore(t *testing.T) {
	t.Run("creates new gitignore", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureGitIgnore(dir); err != nil {
			t.Fatalf("EnsureGitIgnore error: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), CloneDir+"/") {
			t.Error("expected clone dir in .gitignore")
		}
	})

	t.Run("appends to existing", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0644)

		if err := EnsureGitIgnore(dir); err != nil {
			t.Fatalf("EnsureGitIgnore error: %v", err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		content := string(data)
		if !strings.Contains(content, "node_modules/") {
			t.Error("existing content should be preserved")
		}
		if !strings.Contains(content, CloneDir+"/") {
			t.Error("expected clone dir appended")
		}
	})

	t.Run("skips duplicate", func(t *testing.T) {
		dir := t.TempDir()
		existing := "# ignore\n" + CloneDir + "/\n"
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644)

		if err := EnsureGitIgnore(dir); err != nil {
			t.Fatalf("EnsureGitIgnore error: %v", err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		count := strings.Count(string(data), CloneDir+"/")
		if count != 1 {
			t.Errorf("expected 1 entry, got %d", count)
		}
	})
}

func TestLocalPath(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		m := &manifest.Manifest{}
		if got := LocalPaths(m); len(got) != 0 {
			t.Errorf("expected empty, got %s", got)
		}
	})

	t.Run("custom path", func(t *testing.T) {
		m := &manifest.Manifest{Context: manifest.ContextMounts{{Path: "custom/path"}}}
		if got := LocalPaths(m); len(got) != 1 || got[0] != "custom/path" {
			t.Errorf("expected custom/path, got %s", got)
		}
	})

	t.Run("default path", func(t *testing.T) {
		m := &manifest.Manifest{Context: manifest.ContextMounts{{}}}
		if got := LocalPaths(m); len(got) != 1 || got[0] != "docs/context" {
			t.Errorf("expected docs/context, got %s", got)
		}
	})
}

// fakeCheckout fabricates a VALID context checkout: the content dirs plus a
// .git marker — without it the guard classifies the dir as CheckoutInvalid.
func fakeCheckout(t *testing.T, dir string, withProject bool) {
	t.Helper()
	base := filepath.Join(dir, CloneDir)
	if withProject {
		base = filepath.Join(base, ".amaru_registry", "context", "myapp")
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, CloneDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
}

// mockBackend implements vcs.Backend for testing.
type mockBackend struct {
	name        string
	cloneErr    error
	pullErr     error
	hasChanges  bool
	addErr      error
	pushErr     error
	calls       []string
	sparsePaths []string
}

func (m *mockBackend) Name() string { return m.name }
func (m *mockBackend) SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error {
	m.calls = append(m.calls, "SparseClone")
	if m.cloneErr != nil {
		return m.cloneErr
	}
	// Create the target directory to simulate a successful clone
	return os.MkdirAll(filepath.Join(targetDir, ".amaru_registry", "context"), 0755)
}
func (m *mockBackend) EnsureSparsePaths(ctx context.Context, dir string, paths []string) error {
	m.calls = append(m.calls, "EnsureSparsePaths")
	m.sparsePaths = paths
	return nil
}
func (m *mockBackend) Pull(ctx context.Context, dir string) error {
	m.calls = append(m.calls, "Pull")
	return m.pullErr
}
func (m *mockBackend) HasChanges(ctx context.Context, dir string) bool {
	m.calls = append(m.calls, "HasChanges")
	return m.hasChanges
}
func (m *mockBackend) Add(ctx context.Context, dir string, paths []string) error {
	m.calls = append(m.calls, "Add")
	return m.addErr
}
func (m *mockBackend) CommitAndPush(ctx context.Context, dir, message string) error {
	m.calls = append(m.calls, "CommitAndPush")
	return m.pushErr
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Registry:  manifest.RegistryConfig{URL: "github:acme/registry", Auth: "none"},
		Project:   "myapp",
		LocalPath: "docs/context",
	}

	backend := &mockBackend{name: "git"}
	err := Init(context.Background(), dir, []*Config{cfg}, backend)
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if len(backend.calls) != 1 || backend.calls[0] != "SparseClone" {
		t.Errorf("expected [SparseClone], got %v", backend.calls)
	}

	// Verify symlink was created
	linkPath := filepath.Join(dir, "docs", "context")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink")
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	dir := t.TempDir()
	// Live checkout, but the symlink was deleted (the real-world failure: init
	// used to refuse and sync didn't recreate — the channel stalled silently).
	fakeCheckout(t, dir, true)

	cfg := &Config{
		Registry:  manifest.RegistryConfig{URL: "github:acme/registry", Auth: "none"},
		Project:   "myapp",
		LocalPath: "docs/context",
	}

	backend := &mockBackend{name: "git"}
	err := Init(context.Background(), dir, []*Config{cfg}, backend)
	if err != nil {
		t.Fatalf("expected idempotent init to repair, got error: %v", err)
	}
	// Repair widens the sparse profile (a mount may have been added) but never
	// re-clones.
	for _, call := range backend.calls {
		if call == "SparseClone" {
			t.Errorf("expected no clone on already-initialized repair, got %v", backend.calls)
		}
	}
	info, err := os.Lstat(filepath.Join(dir, "docs", "context"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected repaired symlink, got info=%v err=%v", info, err)
	}
}

func TestEnsureSymlink(t *testing.T) {
	newCfg := func() *Config {
		return &Config{Project: "myapp", LocalPath: "docs/context"}
	}
	setupClone := func(dir string) {
		fakeCheckout(t, dir, true)
	}

	t.Run("creates missing symlink", func(t *testing.T) {
		dir := t.TempDir()
		setupClone(dir)
		created, err := EnsureSymlink(dir, newCfg())
		if err != nil || !created {
			t.Fatalf("expected created=true, got created=%v err=%v", created, err)
		}
		target, err := os.Readlink(filepath.Join(dir, "docs", "context"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(target, "myapp") {
			t.Errorf("symlink should point at the project dir, got %s", target)
		}
	})

	t.Run("leaves a live symlink alone", func(t *testing.T) {
		dir := t.TempDir()
		setupClone(dir)
		if _, err := EnsureSymlink(dir, newCfg()); err != nil {
			t.Fatal(err)
		}
		created, err := EnsureSymlink(dir, newCfg())
		if err != nil || created {
			t.Errorf("expected created=false on live link, got created=%v err=%v", created, err)
		}
	})

	t.Run("replaces a broken symlink", func(t *testing.T) {
		dir := t.TempDir()
		setupClone(dir)
		os.MkdirAll(filepath.Join(dir, "docs"), 0755)
		os.Symlink("nowhere-real", filepath.Join(dir, "docs", "context"))
		created, err := EnsureSymlink(dir, newCfg())
		if err != nil || !created {
			t.Fatalf("expected broken link replaced, got created=%v err=%v", created, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "docs", "context")); err != nil {
			t.Errorf("replaced link should resolve: %v", err)
		}
	})

	t.Run("refuses to clobber a real directory", func(t *testing.T) {
		dir := t.TempDir()
		setupClone(dir)
		os.MkdirAll(filepath.Join(dir, "docs", "context"), 0755)
		if _, err := EnsureSymlink(dir, newCfg()); err == nil {
			t.Error("expected error for non-symlink at the local path")
		}
	})
}

func TestSyncRepairsMissingSymlink(t *testing.T) {
	dir := t.TempDir()
	fakeCheckout(t, dir, true)

	cfg := &Config{Project: "myapp", LocalPath: "docs/context"}
	backend := &mockBackend{name: "git"}

	if err := Sync(context.Background(), dir, []*Config{cfg}, backend); err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if len(backend.calls) != 2 || backend.calls[1] != "Pull" {
		t.Errorf("expected [EnsureSparsePaths Pull], got %v", backend.calls)
	}
	info, err := os.Lstat(filepath.Join(dir, "docs", "context"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected sync to repair the symlink, got info=%v err=%v", info, err)
	}
}

func TestInitSaplingPaths(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Registry:  manifest.RegistryConfig{URL: "github:acme/registry", Auth: "none"},
		Project:   "myapp",
		LocalPath: "docs/context",
	}

	backend := &mockBackend{name: "sapling"}
	Init(context.Background(), dir, []*Config{cfg}, backend)

	// Sapling backend should have been called
	if len(backend.calls) != 1 || backend.calls[0] != "SparseClone" {
		t.Errorf("expected [SparseClone], got %v", backend.calls)
	}
}

func TestSync(t *testing.T) {
	dir := t.TempDir()
	// Create clone dir to simulate initialized state
	fakeCheckout(t, dir, false)

	cfg := &Config{Project: "myapp", LocalPath: "docs/context"}
	backend := &mockBackend{name: "git"}

	err := Sync(context.Background(), dir, []*Config{cfg}, backend)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if len(backend.calls) != 2 || backend.calls[1] != "Pull" {
		t.Errorf("expected [EnsureSparsePaths Pull], got %v", backend.calls)
	}
}

func TestSyncNotInitialized(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git"}

	err := Sync(context.Background(), dir, []*Config{cfg}, backend)
	if err == nil {
		t.Error("expected error for not initialized")
	}
}

func TestPush(t *testing.T) {
	dir := t.TempDir()
	fakeCheckout(t, dir, false)

	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git", hasChanges: true}

	err := Push(context.Background(), dir, []*Config{cfg}, backend, "test commit")
	if err != nil {
		t.Fatalf("Push error: %v", err)
	}
	if len(backend.calls) != 3 {
		t.Fatalf("expected 3 calls, got %v", backend.calls)
	}
	if backend.calls[0] != "HasChanges" || backend.calls[1] != "Add" || backend.calls[2] != "CommitAndPush" {
		t.Errorf("unexpected call sequence: %v", backend.calls)
	}
}

func TestPushNoChanges(t *testing.T) {
	dir := t.TempDir()
	fakeCheckout(t, dir, false)

	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git", hasChanges: false}

	err := Push(context.Background(), dir, []*Config{cfg}, backend, "")
	if err != nil {
		t.Fatalf("Push error: %v", err)
	}
	if len(backend.calls) != 1 || backend.calls[0] != "HasChanges" {
		t.Errorf("expected only HasChanges call, got %v", backend.calls)
	}
}

func TestPushNotInitialized(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git"}

	err := Push(context.Background(), dir, []*Config{cfg}, backend, "")
	if err == nil {
		t.Error("expected error for not initialized")
	}
}

func TestPushMissingCheckoutIsCleanNoOp(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git"}

	err := Push(context.Background(), dir, []*Config{cfg}, backend, "")
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}
	// The corruption vector was running VCS commands anyway: zero calls.
	if len(backend.calls) != 0 {
		t.Errorf("expected NO backend calls, got %v", backend.calls)
	}
}

func TestPushInvalidCheckoutNeverTouchesBackend(t *testing.T) {
	dir := t.TempDir()
	// The live corruption: CloneDir exists WITHOUT its own repository. git
	// discovers the surrounding project and add/commit land THERE — a
	// post-commit push in a worktree committed 104 RFC files into the
	// consumer repo.
	os.MkdirAll(filepath.Join(dir, CloneDir, "context", "myapp"), 0755)

	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git", hasChanges: true}

	err := Push(context.Background(), dir, []*Config{cfg}, backend, "")
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}
	if len(backend.calls) != 0 {
		t.Errorf("expected NO backend calls on invalid checkout, got %v", backend.calls)
	}
}

func TestSyncInvalidCheckoutNeverTouchesBackend(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)

	cfg := &Config{Project: "myapp", LocalPath: "docs/context"}
	backend := &mockBackend{name: "git"}

	err := Sync(context.Background(), dir, []*Config{cfg}, backend)
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}
	if len(backend.calls) != 0 {
		t.Errorf("expected NO backend calls, got %v", backend.calls)
	}
}

func TestInitRefusesInvalidCheckout(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)

	cfg := &Config{
		Registry:  manifest.RegistryConfig{URL: "github:acme/registry", Auth: "none"},
		Project:   "myapp",
		LocalPath: "docs/context",
	}
	backend := &mockBackend{name: "git"}
	err := Init(context.Background(), dir, []*Config{cfg}, backend)
	if err == nil || errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected a plain error telling to remove the dir, got %v", err)
	}
	if len(backend.calls) != 0 {
		t.Errorf("expected no clone over an invalid checkout, got %v", backend.calls)
	}
	if _, err := os.Lstat(filepath.Join(dir, "docs", "context")); err == nil {
		t.Error("must not symlink into an invalid checkout")
	}
}

func TestStateOf(t *testing.T) {
	dir := t.TempDir()
	if got := StateOf(dir); got != CheckoutMissing {
		t.Errorf("expected missing, got %v", got)
	}
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)
	if got := StateOf(dir); got != CheckoutInvalid {
		t.Errorf("expected invalid, got %v", got)
	}
	os.MkdirAll(filepath.Join(dir, CloneDir, ".git"), 0755)
	if got := StateOf(dir); got != CheckoutValid {
		t.Errorf("expected valid, got %v", got)
	}
}

func twoMountManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/registry", Auth: "none"},
		},
		Context: manifest.ContextMounts{
			{Registry: "main", Project: "rfcs", Path: "docs/rfc"},
			{Registry: "main", Project: "claude-base", Path: "docs/claude-base"},
		},
	}
}

func TestResolveConfigsMultiMount(t *testing.T) {
	t.Run("two mounts resolve in order", func(t *testing.T) {
		cfgs, err := ResolveConfigs(twoMountManifest())
		if err != nil {
			t.Fatal(err)
		}
		if len(cfgs) != 2 || cfgs[0].Project != "rfcs" || cfgs[1].Project != "claude-base" {
			t.Errorf("unexpected cfgs: %+v", cfgs)
		}
	})

	t.Run("duplicate path refused", func(t *testing.T) {
		m := twoMountManifest()
		m.Context[1].Path = "docs/rfc"
		if _, err := ResolveConfigs(m); err == nil || !strings.Contains(err.Error(), "duplicate context path") {
			t.Errorf("expected duplicate path error, got %v", err)
		}
	})

	t.Run("duplicate project refused", func(t *testing.T) {
		m := twoMountManifest()
		m.Context[1].Project = "rfcs"
		if _, err := ResolveConfigs(m); err == nil || !strings.Contains(err.Error(), "duplicate context project") {
			t.Errorf("expected duplicate project error, got %v", err)
		}
	})

	t.Run("mixed registries refused (single shared checkout)", func(t *testing.T) {
		m := twoMountManifest()
		m.Registries["other"] = manifest.RegistryConfig{URL: "github:acme/other", Auth: "none"}
		m.Context[1].Registry = "other"
		if _, err := ResolveConfigs(m); err == nil || !strings.Contains(err.Error(), "share one registry") {
			t.Errorf("expected same-registry error, got %v", err)
		}
	})
}

func TestFilterByProject(t *testing.T) {
	cfgs, _ := ResolveConfigs(twoMountManifest())
	one, err := FilterByProject(cfgs, "claude-base")
	if err != nil || len(one) != 1 || one[0].Project != "claude-base" {
		t.Errorf("expected the claude-base mount, got %+v err=%v", one, err)
	}
	if _, err := FilterByProject(cfgs, "nope"); err == nil {
		t.Error("expected error for unknown project")
	}
	all, err := FilterByProject(cfgs, "")
	if err != nil || len(all) != 2 {
		t.Errorf("empty filter must keep all mounts, got %+v err=%v", all, err)
	}
}

func TestMultiMountInitSyncPush(t *testing.T) {
	dir := t.TempDir()
	cfgs, err := ResolveConfigs(twoMountManifest())
	if err != nil {
		t.Fatal(err)
	}
	backend := &mockBackend{name: "git"}

	// init: one clone, union of sparse paths, one symlink per mount
	if err := Init(context.Background(), dir, cfgs, backend); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(backend.calls) != 1 || backend.calls[0] != "SparseClone" {
		t.Fatalf("expected one SparseClone, got %v", backend.calls)
	}
	for _, mount := range []string{"docs/rfc", "docs/claude-base"} {
		info, err := os.Lstat(filepath.Join(dir, mount))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected symlink at %s (err=%v)", mount, err)
		}
	}

	// o clone do mock não tem .git — fabrica para o guard aceitar
	os.MkdirAll(filepath.Join(dir, CloneDir, ".git"), 0755)

	// sync: widen + pull + symlinks de pé (remove um para provar o reparo)
	os.Remove(filepath.Join(dir, "docs", "claude-base"))
	backend.calls = nil
	if err := Sync(context.Background(), dir, cfgs, backend); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(backend.sparsePaths) == 0 || !strings.Contains(strings.Join(backend.sparsePaths, " "), "claude-base") {
		t.Errorf("expected sparse union to include both projects, got %v", backend.sparsePaths)
	}
	if _, err := os.Lstat(filepath.Join(dir, "docs", "claude-base")); err != nil {
		t.Errorf("expected sync to repair the second mount's symlink: %v", err)
	}

	// push: um stage por mount, UM commit
	os.MkdirAll(filepath.Join(dir, CloneDir, "context", "rfcs"), 0755)
	os.MkdirAll(filepath.Join(dir, CloneDir, "context", "claude-base"), 0755)
	backend.calls = nil
	backend.hasChanges = true
	if err := Push(context.Background(), dir, cfgs, backend, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}
	want := []string{"HasChanges", "Add", "Add", "CommitAndPush"}
	if strings.Join(backend.calls, ",") != strings.Join(want, ",") {
		t.Errorf("expected %v, got %v", want, backend.calls)
	}
}

func TestMultiMountWorktreeGuard(t *testing.T) {
	dir := t.TempDir()
	cfgs, _ := ResolveConfigs(twoMountManifest())
	backend := &mockBackend{name: "git", hasChanges: true}

	// checkout ausente (worktree novo): push e sync são no-ops limpos
	if err := Push(context.Background(), dir, cfgs, backend, ""); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}
	if err := Sync(context.Background(), dir, cfgs, backend); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}
	if len(backend.calls) != 0 {
		t.Errorf("expected NO backend calls, got %v", backend.calls)
	}

	// checkout inválido: idem
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)
	if err := Push(context.Background(), dir, cfgs, backend, ""); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized on invalid, got %v", err)
	}
	if len(backend.calls) != 0 {
		t.Errorf("expected NO backend calls on invalid, got %v", backend.calls)
	}
}
