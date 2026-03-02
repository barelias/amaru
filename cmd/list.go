package cmd

import (
	"context"
	"fmt"

	"github.com/barelias/amaru/internal/installer"
	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/types"
	"github.com/barelias/amaru/internal/ui"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista skills, commands e agents instalados",
	Long:  "Lista tudo instalado no projeto com status e origem.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	lock, err := loadLock()
	if err != nil {
		return err
	}

	// Try to fetch registry indexes for status
	clients, clientErr := buildClients(ctx, m, true)
	indexes := make(map[string]*registry.RegistryIndex)
	if clientErr == nil {
		for alias, client := range clients {
			idx, err := client.FetchIndex(ctx)
			if err == nil {
				indexes[alias] = idx
			}
		}
	}

	hasItems := false
	for _, itemType := range types.AllInstallableTypes() {
		entries := lock.EntriesForType(itemType)
		if len(entries) > 0 {
			hasItems = true
			ui.Header("%s:", itemType.Plural())
			var rows [][]string
			for name, entry := range entries {
				status := statusForItem(entry, itemType, name, indexes, m)
				rows = append(rows, []string{name, entry.Version, status, fmt.Sprintf("[%s]", entry.Registry)})
			}
			ui.Table(rows)
		}
	}

	if !hasItems {
		fmt.Println("Nenhum item instalado. Rode 'amaru install' primeiro.")
	}

	return nil
}

func statusForItem(entry manifest.LockedEntry, itemType types.ItemType, name string, indexes map[string]*registry.RegistryIndex, m *manifest.Manifest) string {
	if !installer.IsInstalled(".", string(itemType), name) {
		return ui.Error("✗ not installed")
	}

	idx, ok := indexes[entry.Registry]
	if !ok {
		return "?"
	}

	regEntries := idx.EntriesForType(itemType)
	regEntry, ok := regEntries[name]
	if !ok {
		return ui.Success("✓ up-to-date")
	}

	latestV, err := semver.NewVersion(regEntry.Latest)
	if err != nil {
		return "?"
	}
	currentV, err := semver.NewVersion(entry.Version)
	if err != nil {
		return "?"
	}

	if latestV.GreaterThan(currentV) {
		if latestV.Major() > currentV.Major() {
			return ui.Warning(fmt.Sprintf("⚠ %s MAJOR", regEntry.Latest))
		}
		return ui.Warning(fmt.Sprintf("⚠ %s avail", regEntry.Latest))
	}

	return ui.Success("✓ up-to-date")
}
