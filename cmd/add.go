package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/barelias/amaru/internal/installer"
	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/registry"
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
	Short: "Add a skill/command/agent/skillset to the manifest and install",
	Long:  "Add a skill/command/agent to the manifest (amaru.json) and install the files.\nFor skillsets, expands members into individual items.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAdd(cmd.Context(), args[0])
	},
}

func init() {
	addCmd.Flags().StringVar(&addType, "type", "skill", "Item type: skill, command, agent, or skillset")
	addCmd.Flags().BoolVar(&addIsCommand, "command", false, "Shorthand for --type=command")
	addCmd.Flags().StringVar(&addRegistry, "registry", "", "Registry alias (required if multiple registries)")
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

	// Fetch registry index
	clients, err := buildClients(ctx, m, true)
	if err != nil {
		return err
	}

	client := clients[regAlias]
	idx, err := client.FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("fetching registry index: %w", err)
	}

	// Handle skillset type: expand to individual items
	if addType == "skillset" {
		return runAddSkillset(ctx, name, regAlias, m, lock, client, idx)
	}

	// Regular add for skill/command/agent
	entries := idx.EntriesForType(itemType)
	entry, found := entries[name]
	if !found {
		// Check if the name exists as a skillset and suggest it
		if _, isSkillset := idx.Skillsets[name]; isSkillset {
			return fmt.Errorf("%s %q not found in registry %q. Did you mean: amaru add %s --type=skillset", itemType, name, regAlias, name)
		}
		return fmt.Errorf("%s %q not found in registry %q", itemType, name, regAlias)
	}

	// Check if already in manifest
	if deps := m.DepsForType(itemType); deps != nil {
		if _, exists := deps[name]; exists {
			return fmt.Errorf("%s %q already in manifest", itemType, name)
		}
	}

	// Add to manifest with ^latest constraint (or "latest" for unversioned items)
	version := entry.Latest
	spec := manifest.DependencySpec{}
	if version != "" {
		spec.Version = "^" + version
	} else {
		spec.Version = "latest"
	}
	if len(m.Registries) > 1 {
		spec.Registry = regAlias
	}

	m.SetDep(itemType, name, spec)

	// Save manifest
	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	// Download and install (empty version downloads from default branch)
	files, err := client.DownloadFiles(ctx, string(itemType), name, version)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	hash, err := installer.Install(".", string(itemType), name, files)
	if err != nil {
		return fmt.Errorf("installing: %w", err)
	}

	// Update lock (store "latest" for unversioned items)
	lockVersion := version
	if lockVersion == "" {
		lockVersion = "latest"
	}
	lock.EntriesForType(itemType)[name] = manifest.NewLockedEntry(lockVersion, regAlias, hash)

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock: %w", err)
	}

	displayVersion := version
	if displayVersion == "" {
		displayVersion = "latest"
	}
	ui.Check("Added %s %s@%s from [%s]", itemType, name, displayVersion, regAlias)
	fmt.Printf("  %s\n", entry.Description)

	return nil
}

func runAddSkillset(ctx context.Context, name, regAlias string, m *manifest.Manifest, lock *manifest.Lock, client registry.Client, idx *registry.RegistryIndex) error {
	skillset, found := idx.Skillsets[name]
	if !found {
		return fmt.Errorf("skillset %q not found in registry %q", name, regAlias)
	}

	if len(skillset.Items) == 0 {
		return fmt.Errorf("skillset %q has no items", name)
	}

	// Validate all member types (reject nested skillsets)
	for _, item := range skillset.Items {
		if item.Type == "skillset" {
			return fmt.Errorf("skillset %q: nested skillsets are not supported (member %q has type \"skillset\")", name, item.Name)
		}
		itemType := types.ItemType(item.Type)
		if itemType != types.Skill && itemType != types.Command && itemType != types.Agent {
			return fmt.Errorf("skillset %q: member %q has invalid type %q", name, item.Name, item.Type)
		}
	}

	// Pre-validate: check all members exist in the registry
	var missing []string
	for _, item := range skillset.Items {
		itemType := types.ItemType(item.Type)
		entries := idx.EntriesForType(itemType)
		if _, ok := entries[item.Name]; !ok {
			missing = append(missing, fmt.Sprintf("%s %q", item.Type, item.Name))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("skillset %q: members not found in registry: %s", name, strings.Join(missing, ", "))
	}

	fmt.Printf("Installing skillset %q (%d items)...\n", name, len(skillset.Items))

	added := 0
	skipped := 0
	for _, item := range skillset.Items {
		itemType := types.ItemType(item.Type)

		// Skip if already in manifest
		if deps := m.DepsForType(itemType); deps != nil {
			if _, exists := deps[item.Name]; exists {
				ui.Warn("%s %q already in manifest, skipping", item.Type, item.Name)
				skipped++
				continue
			}
		}

		// Look up entry to get version
		entries := idx.EntriesForType(itemType)
		entry := entries[item.Name]

		version := entry.Latest
		spec := manifest.DependencySpec{
			Group: name,
		}
		if version != "" {
			spec.Version = "^" + version
		} else {
			spec.Version = "latest"
		}
		if len(m.Registries) > 1 {
			spec.Registry = regAlias
		}

		m.SetDep(itemType, item.Name, spec)

		// Download and install
		files, err := client.DownloadFiles(ctx, item.Type, item.Name, version)
		if err != nil {
			return fmt.Errorf("downloading %s %q: %w", item.Type, item.Name, err)
		}

		hash, err := installer.Install(".", item.Type, item.Name, files)
		if err != nil {
			return fmt.Errorf("installing %s %q: %w", item.Type, item.Name, err)
		}

		lockVersion := version
		if lockVersion == "" {
			lockVersion = "latest"
		}
		lock.EntriesForType(itemType)[item.Name] = manifest.NewLockedEntry(lockVersion, regAlias, hash)

		displayVersion := version
		if displayVersion == "" {
			displayVersion = "latest"
		}
		ui.Check("  %s %s@%s", item.Type, item.Name, displayVersion)
		added++
	}

	// Compute skillset digest from member versions
	var digestItems []string
	var memberList []string
	for _, item := range skillset.Items {
		itemType := types.ItemType(item.Type)
		lockEntries := lock.EntriesForType(itemType)
		if le, ok := lockEntries[item.Name]; ok {
			digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s", item.Type, item.Name, le.Version))
		}
		memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))
	}

	// Record skillset in lock for change detection
	lock.Skillsets[name] = manifest.LockedSkillset{
		Registry:    regAlias,
		Digest:      manifest.SkillsetDigest(digestItems),
		Members:     memberList,
		InstalledAt: "",
	}

	// Save manifest and lock
	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}
	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock: %w", err)
	}

	fmt.Printf("\nSkillset %q: %d added", name, added)
	if skipped > 0 {
		fmt.Printf(", %d skipped (already installed)", skipped)
	}
	fmt.Println()
	if skillset.Description != "" {
		fmt.Printf("  %s\n", skillset.Description)
	}

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
