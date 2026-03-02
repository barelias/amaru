package vcs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Backend represents a VCS backend (Sapling or Git).
type Backend interface {
	// Name returns "sapling" or "git".
	Name() string
	// SparseClone clones a repo with a sparse profile, checking out only the specified paths.
	SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error
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

// SaplingBackend implements Backend using Sapling (sl).
type SaplingBackend struct{}

func (s *SaplingBackend) Name() string { return "sapling" }

func (s *SaplingBackend) SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error {
	args := []string{"clone"}
	if len(paths) > 0 {
		args = append(args, "--enable-profile", paths[0])
	}
	args = append(args, repoURL, targetDir)
	cmd := exec.CommandContext(ctx, "sl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *SaplingBackend) Pull(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "sl", "pull", "--update")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *SaplingBackend) HasChanges(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "sl", "status")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}

func (s *SaplingBackend) Add(ctx context.Context, dir string, paths []string) error {
	args := append([]string{"add"}, paths...)
	cmd := exec.CommandContext(ctx, "sl", args...)
	cmd.Dir = dir
	return cmd.Run()
}

func (s *SaplingBackend) CommitAndPush(ctx context.Context, dir, message string) error {
	commit := exec.CommandContext(ctx, "sl", "commit", "-m", message)
	commit.Dir = dir
	if err := commit.Run(); err != nil {
		return fmt.Errorf("sl commit: %w", err)
	}
	push := exec.CommandContext(ctx, "sl", "push")
	push.Dir = dir
	return push.Run()
}

// GitBackend implements Backend using Git sparse-checkout.
type GitBackend struct{}

func (g *GitBackend) Name() string { return "git" }

func (g *GitBackend) SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error {
	clone := exec.CommandContext(ctx, "git", "clone", "--filter=blob:none", "--no-checkout", repoURL, targetDir)
	clone.Stdout = os.Stdout
	clone.Stderr = os.Stderr
	if err := clone.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	init := exec.CommandContext(ctx, "git", "sparse-checkout", "init", "--cone")
	init.Dir = targetDir
	if err := init.Run(); err != nil {
		return fmt.Errorf("git sparse-checkout init: %w", err)
	}

	args := append([]string{"sparse-checkout", "set"}, paths...)
	set := exec.CommandContext(ctx, "git", args...)
	set.Dir = targetDir
	if err := set.Run(); err != nil {
		return fmt.Errorf("git sparse-checkout set: %w", err)
	}

	checkout := exec.CommandContext(ctx, "git", "checkout")
	checkout.Dir = targetDir
	return checkout.Run()
}

func (g *GitBackend) Pull(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *GitBackend) HasChanges(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}

func (g *GitBackend) Add(ctx context.Context, dir string, paths []string) error {
	args := append([]string{"add"}, paths...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd.Run()
}

func (g *GitBackend) CommitAndPush(ctx context.Context, dir, message string) error {
	commit := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commit.Dir = dir
	if err := commit.Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	push := exec.CommandContext(ctx, "git", "push")
	push.Dir = dir
	return push.Run()
}
