package cmd

import (
	"context"
	"fmt"

	"github.com/barelias/amaru/internal/installer"
	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/types"
	"github.com/barelias/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var (
	addIsCommand bool
	addType      string
	addRegistry  string
)

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Adiciona uma skill/command/agent ao manifesto e instala",
	Long:  "Adiciona uma skill/command/agent ao manifesto (amaru.json) e instala os arquivos.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAdd(cmd.Context(), args[0])
	},
}

func init() {
	addCmd.Flags().StringVar(&addType, "type", "skill", "Item type: skill, command, or agent")
	addCmd.Flags().BoolVar(&addIsCommand, "command", false, "Shorthand for --type=command")
	addCmd.Flags().StringVar(&addRegistry, "registry", "", "Registry alias (obrigatório se múltiplos registries)")
	rootCmd.AddCommand(addCmd)
}

func runAdd(ctx context.Context, name string) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	lock, err := loadLock()
	if err != nil {
		return err
	}

	// Resolve effective item type
	itemType := types.ItemType(addType)
	if addIsCommand {
		itemType = types.Command
	}

	// Determine which registry to use
	regAlias := addRegistry
	if regAlias == "" {
		regAlias = m.DefaultRegistry()
		if regAlias == "" {
			regAlias, err = findInRegistries(ctx, m, itemType, name)
			if err != nil {
				return err
			}
		}
	}

	if _, ok := m.Registries[regAlias]; !ok {
		return fmt.Errorf("registry %q not found in manifest", regAlias)
	}

	// Check if already in manifest
	if deps := m.DepsForType(itemType); deps != nil {
		if _, exists := deps[name]; exists {
			return fmt.Errorf("%s %q already in manifest", itemType, name)
		}
	}

	// Fetch registry to get latest version
	clients, err := buildClients(ctx, m, true)
	if err != nil {
		return err
	}

	client := clients[regAlias]
	idx, err := client.FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("fetching registry index: %w", err)
	}

	entries := idx.EntriesForType(itemType)
	entry, found := entries[name]
	if !found {
		return fmt.Errorf("%s %q not found in registry %q", itemType, name, regAlias)
	}

	// Add to manifest with ^latest constraint
	spec := manifest.DependencySpec{
		Version: "^" + entry.Latest,
	}
	if len(m.Registries) > 1 {
		spec.Registry = regAlias
	}

	m.SetDep(itemType, name, spec)

	// Save manifest
	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	// Download and install
	files, err := client.DownloadFiles(ctx, string(itemType), name, entry.Latest)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	hash, err := installer.Install(".", string(itemType), name, files)
	if err != nil {
		return fmt.Errorf("installing: %w", err)
	}

	// Update lock
	lock.EntriesForType(itemType)[name] = manifest.NewLockedEntry(entry.Latest, regAlias, hash)

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock: %w", err)
	}

	ui.Check("Added %s %s@%s from [%s]", itemType, name, entry.Latest, regAlias)
	fmt.Printf("  %s\n", entry.Description)

	return nil
}

func findInRegistries(ctx context.Context, m *manifest.Manifest, itemType types.ItemType, name string) (string, error) {
	clients, err := buildClients(ctx, m, true)
	if err != nil {
		return "", err
	}

	var foundIn []string
	for alias, client := range clients {
		idx, err := client.FetchIndex(ctx)
		if err != nil {
			continue
		}
		if entries := idx.EntriesForType(itemType); entries != nil {
			if _, ok := entries[name]; ok {
				foundIn = append(foundIn, alias)
			}
		}
	}

	switch len(foundIn) {
	case 0:
		return "", fmt.Errorf("%s %q not found in any configured registry. Use 'amaru browse' to see available items", itemType, name)
	case 1:
		return foundIn[0], nil
	default:
		return "", fmt.Errorf("%s %q found in multiple registries: %v. Use --registry to specify", itemType, name, foundIn)
	}
}
