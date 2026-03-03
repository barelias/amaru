package cmd

import (
	"context"
	"fmt"

	"github.com/barelias/amaru/internal/installer"
	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/resolver"
	"github.com/barelias/amaru/internal/types"
	"github.com/barelias/amaru/internal/ui"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Atualiza skills/commands para versões compatíveis mais recentes",
	Long:  "Atualiza skills/commands para versões mais recentes compatíveis com os ranges do manifesto.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var name string
		if len(args) > 0 {
			name = args[0]
		}
		return runUpdate(cmd.Context(), name)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(ctx context.Context, filterName string) error {
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

	updated := 0

	for _, itemType := range types.AllInstallableTypes() {
		for name, spec := range m.DepsForType(itemType) {
			if filterName != "" && name != filterName {
				continue
			}
			did, err := updateItem(ctx, m, lock, clients, string(itemType), name, spec, lock.EntriesForType(itemType))
			if err != nil {
				return fmt.Errorf("%s %s: %w", itemType, name, err)
			}
			if did {
				updated++
			}
		}
	}

	if updated == 0 {
		if filterName != "" {
			fmt.Printf("\n%s já está na versão mais recente compatível.\n", filterName)
		} else {
			fmt.Println("\nTudo já está atualizado.")
		}
		return nil
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}
	fmt.Println("\nLock file updated.")

	return nil
}

func updateItem(ctx context.Context, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client, itemType, name string, spec manifest.DependencySpec, lockEntries map[string]manifest.LockedEntry) (bool, error) {
	regAlias, err := m.ResolveRegistry(spec)
	if err != nil {
		return false, err
	}

	client, ok := clients[regAlias]
	if !ok {
		return false, fmt.Errorf("no client for registry %q", regAlias)
	}

	locked, hasLock := lockEntries[name]
	if !hasLock {
		return false, nil // Not installed
	}

	// For "latest" items, re-download from default branch and compare hash
	if spec.Version == "latest" {
		files, err := client.DownloadFiles(ctx, itemType, name, "")
		if err != nil {
			return false, fmt.Errorf("downloading: %w", err)
		}

		hash, err := installer.Install(".", itemType, name, files)
		if err != nil {
			return false, fmt.Errorf("installing: %w", err)
		}

		if hash != locked.Hash {
			lockEntries[name] = manifest.NewLockedEntry("latest", regAlias, hash)
			ui.Check("Updating %s@latest — content changed [%s]", name, regAlias)
			return true, nil
		}
		return false, nil
	}

	versions, err := client.ListVersions(ctx, itemType, name)
	if err != nil {
		return false, fmt.Errorf("listing versions: %w", err)
	}

	currentV, err := semver.NewVersion(locked.Version)
	if err != nil {
		return false, err
	}

	best, err := resolver.Resolve(spec.Version, versions)
	if err != nil {
		return false, err
	}

	if !best.GreaterThan(currentV) {
		return false, nil
	}

	// Download and install new version
	files, err := client.DownloadFiles(ctx, itemType, name, best.String())
	if err != nil {
		return false, fmt.Errorf("downloading: %w", err)
	}

	hash, err := installer.Install(".", itemType, name, files)
	if err != nil {
		return false, fmt.Errorf("installing: %w", err)
	}

	lockEntries[name] = manifest.NewLockedEntry(best.String(), regAlias, hash)
	category := resolver.ClassifyUpdate(locked.Version, best.String())
	ui.Check("Updating %s: %s → %s (%s) [%s]", name, locked.Version, best.String(), category, regAlias)

	return true, nil
}
