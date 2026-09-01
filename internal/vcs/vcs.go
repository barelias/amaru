package vcs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Backend represents a VCS backend (Sapling or Git).
type Backend interface {
	// Name returns "sapling" or "git".
	Name() string
	// SparseClone clones a repo with a sparse profile, checking out only the specified paths.
	SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error
	// EnsureSparsePaths widens/sets the sparse profile of an existing checkout.
	EnsureSparsePaths(ctx context.Context, dir string, paths []string) error
	// Pull fetches and updates the sparse working copy.
	Pull(ctx context.Context, dir string) error
	// HasChanges returns true if there are uncommitted changes.
	HasChanges(ctx context.Context, dir string) bool
	// Add stages changes in the given paths.
	Add(ctx context.Context, dir string, paths []string) error
	// CommitAndPush commits staged changes and pushes.
	CommitAndPush(ctx context.Context, dir, message string) error
}

// Detect returns the best available VCS backend.
// Prefers Sapling if available, falls back to Git.
func Detect() Backend {
	if _, err := exec.LookPath("sl"); err == nil {
		return &SaplingBackend{}
	}
	return &GitBackend{}
}

// repoLocationEnvVars são as variáveis de ambiente que fixam a LOCALIZAÇÃO do
// repositório/worktree/índice do git. O git as exporta para o processo dos
// hooks — num worktree linkado, GIT_DIR e GIT_INDEX_FILE saem ABSOLUTAS,
// apontando para .git/worktrees/<nome> do repo consumidor (na árvore principal
// GIT_INDEX_FILE sai relativa e resolve por acidente dentro do checkout, o que
// escondia o vazamento). Herdada por um git filho, GIT_DIR pula a descoberta
// por diretório: todo comando destinado ao checkout do registry passa a operar
// no repo CONSUMIDOR, mesmo com o diretório certo. Toda invocação de backend
// limpa essas variáveis.
var repoLocationEnvVars = map[string]bool{
	"GIT_DIR":                          true,
	"GIT_WORK_TREE":                    true,
	"GIT_IMPLICIT_WORK_TREE":           true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"GIT_COMMON_DIR":                   true,
	"GIT_NAMESPACE":                    true,
	"GIT_PREFIX":                       true,
	"GIT_GRAFT_FILE":                   true,
	"GIT_SHALLOW_FILE":                 true,
	"GIT_INTERNAL_SUPER_PREFIX":        true,
	"GIT_CEILING_DIRECTORIES":          true,
}

// scrubbedEnv devolve o ambiente do processo sem as variáveis de localização
// de repositório, preservando o resto (credenciais, GIT_SSH_COMMAND, autor).
func scrubbedEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !repoLocationEnvVars[name] {
			out = append(out, kv)
		}
	}
	return out
}

// gitCmd monta um comando git ancorado explicitamente em dir (via -C) e com o
// ambiente limpo — nunca herda cwd nem GIT_DIR/GIT_WORK_TREE do chamador.
func gitCmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = scrubbedEnv()
	return cmd
}

// samePath compara dois caminhos após normalização absoluta e resolução de
// symlinks (t.TempDir no macOS vive atrás de /private).
func samePath(a, b string) bool {
	norm := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		return filepath.Clean(p)
	}
	return norm(a) == norm(b)
}

// SaplingBackend implements Backend using Sapling (sl).
type SaplingBackend struct{}

func (s *SaplingBackend) Name() string { return "sapling" }

// slCmd monta um comando sl com cwd explícito e o mesmo ambiente limpo do git
// — o modo de interoperabilidade dotgit do Sapling também lê GIT_DIR.
func slCmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sl", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedEnv()
	return cmd
}

func (s *SaplingBackend) SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error {
	args := []string{"clone"}
	if len(paths) > 0 {
		args = append(args, "--enable-profile", paths[0])
	}
	args = append(args, repoURL, targetDir)
	cmd := exec.CommandContext(ctx, "sl", args...)
	cmd.Env = scrubbedEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *SaplingBackend) EnsureSparsePaths(ctx context.Context, dir string, paths []string) error {
	for _, p := range paths {
		cmd := slCmd(ctx, dir, "sparse", "include", p)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sl sparse include %s: %w", p, err)
		}
	}
	return nil
}

func (s *SaplingBackend) Pull(ctx context.Context, dir string) error {
	cmd := slCmd(ctx, dir, "pull", "--update")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *SaplingBackend) HasChanges(ctx context.Context, dir string) bool {
	out, err := slCmd(ctx, dir, "status").Output()
	return err == nil && len(out) > 0
}

func (s *SaplingBackend) Add(ctx context.Context, dir string, paths []string) error {
	return slCmd(ctx, dir, append([]string{"add"}, paths...)...).Run()
}

func (s *SaplingBackend) CommitAndPush(ctx context.Context, dir, message string) error {
	if err := slCmd(ctx, dir, "commit", "-m", message).Run(); err != nil {
		return fmt.Errorf("sl commit: %w", err)
	}
	return slCmd(ctx, dir, "push").Run()
}

// GitBackend implements Backend using Git sparse-checkout.
type GitBackend struct{}

func (g *GitBackend) Name() string { return "git" }

// requireOwnRepo garante que dir é a raiz do seu PRÓPRIO repositório antes de
// qualquer operação. Sem o .git próprio, a descoberta do git sobe até o
// projeto que envolve o checkout — e um sparse-checkout/add/commit destinado
// ao registry aterrissa no repo do consumidor.
func (g *GitBackend) requireOwnRepo(ctx context.Context, dir string) error {
	out, err := gitCmd(ctx, dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("%s is not a git checkout: %w", dir, err)
	}
	top := strings.TrimSpace(string(out))
	if !samePath(top, dir) {
		return fmt.Errorf(
			"refusing to run git in %s: it resolves to the repository at %s (the surrounding project), not to its own checkout",
			dir, top)
	}
	return nil
}

func (g *GitBackend) SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error {
	clone := exec.CommandContext(ctx, "git", "clone", "--filter=blob:none", "--no-checkout", repoURL, targetDir)
	clone.Env = scrubbedEnv()
	clone.Stdout = os.Stdout
	clone.Stderr = os.Stderr
	if err := clone.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	if err := gitCmd(ctx, targetDir, "sparse-checkout", "init", "--cone").Run(); err != nil {
		return fmt.Errorf("git sparse-checkout init: %w", err)
	}

	if err := gitCmd(ctx, targetDir, append([]string{"sparse-checkout", "set"}, paths...)...).Run(); err != nil {
		return fmt.Errorf("git sparse-checkout set: %w", err)
	}

	return gitCmd(ctx, targetDir, "checkout").Run()
}

func (g *GitBackend) EnsureSparsePaths(ctx context.Context, dir string, paths []string) error {
	if err := g.requireOwnRepo(ctx, dir); err != nil {
		return err
	}
	// `set` is idempotent and replaces the profile with the given union — the
	// working tree updates in place. --skip-checks: the profile includes
	// AGENTS.md, a FILE, which cone mode otherwise refuses once it exists in
	// the checkout (measured live against a real registry).
	cmd := gitCmd(ctx, dir, append([]string{"sparse-checkout", "set", "--skip-checks"}, paths...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git sparse-checkout set: %w", err)
	}
	return nil
}

func (g *GitBackend) Pull(ctx context.Context, dir string) error {
	if err := g.requireOwnRepo(ctx, dir); err != nil {
		return err
	}
	cmd := gitCmd(ctx, dir, "pull")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *GitBackend) HasChanges(ctx context.Context, dir string) bool {
	if err := g.requireOwnRepo(ctx, dir); err != nil {
		return false
	}
	out, err := gitCmd(ctx, dir, "status", "--porcelain").Output()
	return err == nil && len(out) > 0
}

func (g *GitBackend) Add(ctx context.Context, dir string, paths []string) error {
	if err := g.requireOwnRepo(ctx, dir); err != nil {
		return err
	}
	return gitCmd(ctx, dir, append([]string{"add"}, paths...)...).Run()
}

func (g *GitBackend) CommitAndPush(ctx context.Context, dir, message string) error {
	if err := g.requireOwnRepo(ctx, dir); err != nil {
		return err
	}
	if err := gitCmd(ctx, dir, "commit", "-m", message).Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return gitCmd(ctx, dir, "push").Run()
}
