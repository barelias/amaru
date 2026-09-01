package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/useamaru/amaru/internal/ctxdocs"
	"github.com/useamaru/amaru/internal/hooks"
	"github.com/useamaru/amaru/internal/ui"
	"github.com/useamaru/amaru/internal/vcs"

	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage context documentation from the centralized registry",
	Long:  "Sync compound engineering context docs (brainstorms, plans, solutions) from the centralized registry.",
}

var contextInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up context sync for the current project",
	Long:  "Clone the context section of the registry using sparse checkout (Sapling or Git).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runContextInit(cmd.Context())
	},
}

var contextSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull latest context from the centralized repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runContextSync(cmd.Context())
	},
}

var (
	contextPushMessage string
	contextProject     string
)

var contextPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push local context changes back to the centralized repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runContextPush(cmd.Context())
	},
}

var contextPathCmd = &cobra.Command{
	Use:    "path",
	Short:  "Print the local context path",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadManifest()
		if err != nil {
			return err
		}
		paths := ctxdocs.LocalPaths(m)
		if len(paths) == 0 {
			return fmt.Errorf("no context configured")
		}
		// One per line; the post-commit hook greps them as alternatives.
		fmt.Print(strings.Join(paths, "\n"))
		return nil
	},
}

func init() {
	contextPushCmd.Flags().StringVarP(&contextPushMessage, "message", "m", "", "Commit message")
	for _, c := range []*cobra.Command{contextInitCmd, contextSyncCmd, contextPushCmd} {
		c.Flags().StringVar(&contextProject, "project", "", "Operate on a single context mount (project name)")
	}
	contextCmd.AddCommand(contextInitCmd)
	contextCmd.AddCommand(contextSyncCmd)
	contextCmd.AddCommand(contextPushCmd)
	contextCmd.AddCommand(contextPathCmd)
	rootCmd.AddCommand(contextCmd)
}

func runContextInit(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	cfgs, err := ctxdocs.ResolveConfigs(m)
	if err != nil {
		return err
	}
	cfgs, err = ctxdocs.FilterByProject(cfgs, contextProject)
	if err != nil {
		return err
	}

	backend := vcs.Detect()
	fmt.Printf("Using %s for sparse checkout...\n", backend.Name())

	if err := ctxdocs.Init(ctx, ".", cfgs, backend); err != nil {
		return err
	}

	// Add clone dir to .gitignore
	if err := ctxdocs.EnsureGitIgnore("."); err != nil {
		ui.Err("Could not update .gitignore: %v", err)
	}

	// Install git hooks
	if err := hooks.InstallHook(".", "post-checkout", hooks.PostCheckoutScript()); err != nil {
		ui.Err("Could not install post-checkout hook: %v", err)
	} else {
		ui.Check("Installed post-checkout hook for auto-sync")
	}

	if err := hooks.InstallHook(".", "post-commit", hooks.PostCommitScript()); err != nil {
		ui.Err("Could not install post-commit hook: %v", err)
	} else {
		ui.Check("Installed post-commit hook for auto-push")
	}

	for _, cfg := range cfgs {
		ui.Check("Context initialized for project %q", cfg.Project)
		fmt.Printf("  Docs available at: %s/\n", cfg.LocalPath)
	}
	return nil
}

func runContextSync(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	cfgs, err := ctxdocs.ResolveConfigs(m)
	if err != nil {
		return err
	}
	cfgs, err = ctxdocs.FilterByProject(cfgs, contextProject)
	if err != nil {
		return err
	}

	backend := vcs.Detect()
	if err := ctxdocs.Sync(ctx, ".", cfgs, backend); err != nil {
		// Hook position (post-merge/post-checkout): a fresh git worktree never
		// carries the gitignored checkout — warn and exit clean, don't fail.
		if errors.Is(err, ctxdocs.ErrNotInitialized) {
			ui.Warn("%v", err)
			ui.Warn("Nothing to sync here (fresh clone or git worktree?). Run 'amaru context init' to set it up.")
			return nil
		}
		return err
	}

	for _, cfg := range cfgs {
		ui.Check("Context synced for project %q", cfg.Project)
	}
	return nil
}

func runContextPush(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	cfgs, err := ctxdocs.ResolveConfigs(m)
	if err != nil {
		return err
	}
	cfgs, err = ctxdocs.FilterByProject(cfgs, contextProject)
	if err != nil {
		return err
	}

	backend := vcs.Detect()
	if err := ctxdocs.Push(ctx, ".", cfgs, backend, contextPushMessage); err != nil {
		// Hook position (post-commit): same worktree tolerance as sync.
		if errors.Is(err, ctxdocs.ErrNotInitialized) {
			ui.Warn("%v", err)
			ui.Warn("Nothing to push here (fresh clone or git worktree?). Run 'amaru context init' to set it up.")
			return nil
		}
		return err
	}

	for _, cfg := range cfgs {
		ui.Check("Context pushed for project %q", cfg.Project)
	}
	return nil
}
