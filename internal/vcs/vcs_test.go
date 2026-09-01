package vcs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	backend := Detect()
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
	name := backend.Name()
	if name != "git" && name != "sapling" {
		t.Errorf("expected git or sapling, got %s", name)
	}
}

func TestSaplingBackendName(t *testing.T) {
	b := &SaplingBackend{}
	if b.Name() != "sapling" {
		t.Errorf("expected sapling, got %s", b.Name())
	}
}

func TestGitBackendName(t *testing.T) {
	b := &GitBackend{}
	if b.Name() != "git" {
		t.Errorf("expected git, got %s", b.Name())
	}
}

func TestScrubbedEnv(t *testing.T) {
	t.Setenv("GIT_DIR", "/algum/lugar/.git/worktrees/wt")
	t.Setenv("GIT_INDEX_FILE", "/algum/lugar/.git/worktrees/wt/index")
	t.Setenv("GIT_WORK_TREE", "/algum/lugar")
	t.Setenv("GIT_SSH_COMMAND", "ssh -i chave")

	env := scrubbedEnv()
	joined := strings.Join(env, "\n")
	for _, banned := range []string{"GIT_DIR=", "GIT_INDEX_FILE=", "GIT_WORK_TREE="} {
		if strings.Contains(joined, banned) {
			t.Errorf("scrubbedEnv must drop %s", banned)
		}
	}
	if !strings.Contains(joined, "GIT_SSH_COMMAND=ssh -i chave") {
		t.Error("scrubbedEnv must keep non-location git vars (credentials, ssh)")
	}
}

// requireGit pula o teste quando não há git no PATH.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// mustGit roda git com identidade fixa e ambiente limpo (o processo de teste
// pode estar com GIT_DIR envenenado via t.Setenv) e falha o teste em erro.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=amaru-test", "-c", "user.email=amaru@test", "-c", "protocol.file.allow=always"}, args...)
	cmd := gitCmd(context.Background(), dir, full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v em %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// tryGit roda git com ambiente limpo e devolve saída + erro (para asserções
// de estado que podem legitimamente falhar).
func tryGit(dir string, args ...string) (string, error) {
	out, err := gitCmd(context.Background(), dir, args...).CombinedOutput()
	return string(out), err
}

// setupConsumerWithWorktree cria o repo consumidor com um commit e um
// worktree linkado, devolvendo (consumer, worktree).
func setupConsumerWithWorktree(t *testing.T, base string) (string, string) {
	t.Helper()
	consumer := filepath.Join(base, "consumer")
	if err := os.MkdirAll(consumer, 0755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, consumer, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(consumer, "app.txt"), []byte("app\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, consumer, "add", "-A")
	mustGit(t, consumer, "commit", "-q", "-m", "consumer inicial")

	worktree := filepath.Join(base, "wt")
	mustGit(t, consumer, "worktree", "add", "-q", worktree)
	return consumer, worktree
}

// setupRegistryClone cria um registry remoto (bare) com conteúdo de contexto
// e o clona em <worktree>/.claude/.amaru-context — o que o context init faz.
// Devolve (remote, cloneDir).
func setupRegistryClone(t *testing.T, base, worktree string) (string, string) {
	t.Helper()
	seed := filepath.Join(base, "seed")
	if err := os.MkdirAll(filepath.Join(seed, "context", "proj"), 0755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, seed, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "context", "proj", "rfc.md"), []byte("rfc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "AGENTS.md"), []byte("agents\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, seed, "add", "-A")
	mustGit(t, seed, "commit", "-q", "-m", "registry seed")

	remote := filepath.Join(base, "registry.git")
	mustGit(t, base, "clone", "-q", "--bare", seed, remote)

	cloneDir := filepath.Join(worktree, ".claude", ".amaru-context")
	mustGit(t, base, "clone", "-q", remote, cloneDir)
	mustGit(t, cloneDir, "config", "user.name", "amaru-test")
	mustGit(t, cloneDir, "config", "user.email", "amaru@test")
	return remote, cloneDir
}

// poisonHookEnv reproduz o ambiente que o git exporta a um hook rodando num
// WORKTREE linkado do consumidor (medido ao vivo): GIT_DIR e GIT_INDEX_FILE
// absolutos apontando para .git/worktrees/<nome>.
func poisonHookEnv(t *testing.T, consumer, worktreeName string) {
	t.Helper()
	gitDir := filepath.Join(consumer, ".git", "worktrees", worktreeName)
	if _, err := os.Stat(gitDir); err != nil {
		t.Fatalf("gitdir do worktree não existe: %v", err)
	}
	t.Setenv("GIT_DIR", gitDir)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(gitDir, "index"))
	t.Setenv("GIT_PREFIX", "")
}

// assertConsumerUntouched garante que o worktree do consumidor não ganhou
// perfil esparso nem commit/stage vindos do registry — a contaminação medida
// em produção.
func assertConsumerUntouched(t *testing.T, worktree string) {
	t.Helper()
	if out, err := tryGit(worktree, "sparse-checkout", "list"); err == nil {
		t.Errorf("o worktree do consumidor virou sparse-checkout:\n%s", out)
	}
	if out, _ := tryGit(worktree, "config", "--get", "core.sparseCheckout"); strings.TrimSpace(out) == "true" {
		t.Error("core.sparseCheckout foi setado no worktree do consumidor")
	}
	log, err := tryGit(worktree, "log", "--oneline")
	if err != nil {
		t.Fatalf("git log no consumidor: %v", err)
	}
	if strings.Contains(log, "amaru") {
		t.Errorf("commit do registry vazou para o consumidor:\n%s", log)
	}
	if out, _ := tryGit(worktree, "diff", "--cached", "--name-only"); strings.TrimSpace(out) != "" {
		t.Errorf("arquivos do registry foram staged no consumidor:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(worktree, "context")); err == nil {
		t.Error("um diretório context/ foi materializado na raiz do consumidor")
	}
}

// O bug de produção: hooks do consumidor (post-checkout/post-commit) rodando
// num worktree exportam GIT_DIR/GIT_INDEX_FILE absolutos; herdados pelos git
// filhos do amaru, todo comando destinado ao checkout do registry operava no
// repo CONSUMIDOR — sparse-checkout aplicado ao worktree e um commit com 28
// arquivos de contexto dentro do projeto. O backend precisa ser imune a esse
// ambiente.
func TestGitBackendImmuneToWorktreeHookEnv(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	base := t.TempDir()

	consumer, worktree := setupConsumerWithWorktree(t, base)
	remote, cloneDir := setupRegistryClone(t, base, worktree)
	poisonHookEnv(t, consumer, "wt")

	b := &GitBackend{}

	// sync: perfil esparso + pull, ancorados no checkout do registry
	if err := b.EnsureSparsePaths(ctx, cloneDir, []string{"context/proj", "AGENTS.md"}); err != nil {
		t.Fatalf("EnsureSparsePaths: %v", err)
	}
	if err := b.Pull(ctx, cloneDir); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if out, err := tryGit(cloneDir, "sparse-checkout", "list"); err != nil || !strings.Contains(out, "context/proj") {
		t.Errorf("o perfil esparso deveria estar no clone do registry, got %q err=%v", out, err)
	}

	// consumidor sujo + registry limpo: o gatilho do push era o status do
	// consumidor vazando — HasChanges tem que responder pelo REGISTRY
	if err := os.WriteFile(filepath.Join(worktree, "app.txt"), []byte("sujo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if b.HasChanges(ctx, cloneDir) {
		t.Error("HasChanges leu o estado do consumidor, não do registry")
	}

	// push: add + commit + push aterrissam no registry, nunca no consumidor
	if err := os.WriteFile(filepath.Join(cloneDir, "context", "proj", "novo.md"), []byte("novo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !b.HasChanges(ctx, cloneDir) {
		t.Fatal("expected changes in the registry checkout")
	}
	if err := b.Add(ctx, cloneDir, []string{"context/proj"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := b.CommitAndPush(ctx, cloneDir, "amaru: update context for proj"); err != nil {
		t.Fatalf("CommitAndPush: %v", err)
	}

	remoteLog, err := tryGit(remote, "log", "--oneline", "-1")
	if err != nil || !strings.Contains(remoteLog, "amaru: update context") {
		t.Errorf("o commit deveria chegar ao remoto do registry, got %q err=%v", remoteLog, err)
	}
	assertConsumerUntouched(t, worktree)
}

// SparseClone sob o mesmo ambiente de hook: o clone e seus passos de
// sparse-checkout/checkout têm que agir só no diretório alvo.
func TestGitBackendSparseCloneImmuneToWorktreeHookEnv(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	base := t.TempDir()

	consumer, worktree := setupConsumerWithWorktree(t, base)
	remote, _ := setupRegistryClone(t, base, worktree)
	// descarta o clone pré-feito: este teste exercita o SparseClone real
	if err := os.RemoveAll(filepath.Join(worktree, ".claude")); err != nil {
		t.Fatal(err)
	}
	poisonHookEnv(t, consumer, "wt")

	b := &GitBackend{}
	target := filepath.Join(worktree, ".claude", ".amaru-context")
	if err := b.SparseClone(ctx, remote, target, []string{"context/proj", "AGENTS.md"}); err != nil {
		t.Fatalf("SparseClone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "context", "proj", "rfc.md")); err != nil {
		t.Errorf("o conteúdo do registry deveria estar no checkout: %v", err)
	}
	assertConsumerUntouched(t, worktree)
}

// Diretório de checkout SEM repositório próprio dentro de um projeto git: a
// descoberta subiria até o consumidor. Toda operação recusa (guard do #9,
// agora também no nível do backend — cobre sync e push).
func TestGitBackendRefusesCheckoutInsideSurroundingRepo(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	base := t.TempDir()

	_, worktree := setupConsumerWithWorktree(t, base)
	dir := filepath.Join(worktree, ".claude", ".amaru-context")
	if err := os.MkdirAll(filepath.Join(dir, "context", "proj"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "context", "proj", "rfc.md"), []byte("rfc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	b := &GitBackend{}
	if err := b.EnsureSparsePaths(ctx, dir, []string{"context/proj"}); err == nil {
		t.Error("EnsureSparsePaths deveria recusar um checkout sem repositório próprio")
	}
	if err := b.Pull(ctx, dir); err == nil {
		t.Error("Pull deveria recusar um checkout sem repositório próprio")
	}
	if err := b.Add(ctx, dir, []string{"context/proj"}); err == nil {
		t.Error("Add deveria recusar um checkout sem repositório próprio")
	}
	if err := b.CommitAndPush(ctx, dir, "amaru: nunca"); err == nil {
		t.Error("CommitAndPush deveria recusar um checkout sem repositório próprio")
	}
	if b.HasChanges(ctx, dir) {
		t.Error("HasChanges deveria responder false para um checkout sem repositório próprio")
	}
	assertConsumerUntouched(t, worktree)
}
