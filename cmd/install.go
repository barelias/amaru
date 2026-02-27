package cmd

import (
	"context"
	"fmt"

	"github.com/barelias/amaru/internal/installer"
	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/resolver"
	"github.com/barelias/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var installForce bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Instala skills e commands do manifesto",
	Long:  "Lê amaru.json, autentica nos registries, resolve versões, copia arquivos e gera amaru.lock.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInstall(cmd.Context())
	},
}

func init() {
	installCmd.Flags().BoolVar(&installForce, "force", false, "Reinstala mesmo se lock existe e versões são compatíveis")
	rootCmd.AddCommand(installCmd)
}

func runInstall(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	lock, err := loadLock()
	if err != nil {
		return err
	}

	clients, err := buildClients(ctx, m, false)
	if err != nil {
		return err
	}

	// Install skills
	if len(m.Skills) > 0 {
		ui.Header("Installing skills...")
		for name, spec := range m.Skills {
			if err := installItem(ctx, m, lock, clients, "skill", name, spec, lock.Skills); err != nil {
				return fmt.Errorf("skill %s: %w", name, err)
			}
		}
	}

	// Install commands
	if len(m.Commands) > 0 {
		ui.Header("Installing commands...")
		for name, spec := range m.Commands {
			if err := installItem(ctx, m, lock, clients, "command", name, spec, lock.Commands); err != nil {
				return fmt.Errorf("command %s: %w", name, err)
			}
		}
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}
	fmt.Println("\nLock file updated.")

	return nil
}

func installItem(ctx context.Context, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client, itemType, name string, spec manifest.DependencySpec, lockEntries map[string]manifest.LockedEntry) error {
	regAlias, err := m.ResolveRegistry(spec)
	if err != nil {
		return err
	}

	client, ok := clients[regAlias]
	if !ok {
		return fmt.Errorf("no client for registry %q", regAlias)
	}

	// Check if already installed and up to date
	if !installForce {
		if locked, ok := lockEntries[name]; ok {
			if installer.IsInstalled(".", itemType, name) {
				ui.Check("%s@%s (%s) — already installed", name, locked.Version, regAlias)
				return nil
			}
		}
	}

	// Resolve version
	resolved, err := resolveVersion(ctx, client, itemType, name, spec.Version)
	if err != nil {
		return err
	}

	// Download files
	files, err := client.DownloadFiles(ctx, itemType, name, resolved)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	// Install to local project
	hash, err := installer.Install(".", itemType, name, files)
	if err != nil {
		return fmt.Errorf("installing: %w", err)
	}

	// Update lock
	lockEntries[name] = manifest.NewLockedEntry(resolved, regAlias, hash)

	ui.Check("%s@%s (%s)", name, resolved, regAlias)
	return nil
}

func resolveVersion(ctx context.Context, client registry.Client, itemType, name, constraint string) (string, error) {
	versions, err := client.ListVersions(ctx, itemType, name)
	if err != nil {
		return "", fmt.Errorf("listing versions: %w", err)
	}

	best, err := resolver.Resolve(constraint, versions)
	if err != nil {
		return "", err
	}

	return best.String(), nil
}
