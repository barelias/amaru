package cmd

import (
	"context"
	"fmt"

	"github.com/barelias/amaru/internal/installer"
	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/ui"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista skills e commands instalados",
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

	// Print skills
	if len(lock.Skills) > 0 {
		ui.Header("Skills:")
		var rows [][]string
		for name, entry := range lock.Skills {
			status := statusForItem(entry, "skill", name, indexes, m)
			rows = append(rows, []string{name, entry.Version, status, fmt.Sprintf("[%s]", entry.Registry)})
		}
		ui.Table(rows)
	}

	// Print commands
	if len(lock.Commands) > 0 {
		ui.Header("Commands:")
		var rows [][]string
		for name, entry := range lock.Commands {
			status := statusForItem(entry, "command", name, indexes, m)
			rows = append(rows, []string{name, entry.Version, status, fmt.Sprintf("[%s]", entry.Registry)})
		}
		ui.Table(rows)
	}

	if len(lock.Skills) == 0 && len(lock.Commands) == 0 {
		fmt.Println("Nenhuma skill ou command instalado. Rode 'amaru install' primeiro.")
	}

	return nil
}

func statusForItem(entry manifest.LockedEntry, itemType, name string, indexes map[string]*registry.RegistryIndex, m *manifest.Manifest) string {
	// Check if files are present locally
	if !installer.IsInstalled(".", itemType, name) {
		return ui.Error("✗ not installed")
	}

	idx, ok := indexes[entry.Registry]
	if !ok {
		return "?"
	}

	var regEntry registry.RegistryEntry
	if itemType == "skill" {
		regEntry, ok = idx.Skills[name]
	} else {
		regEntry, ok = idx.Commands[name]
	}
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
