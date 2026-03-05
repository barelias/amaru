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

var updateSkillset string

var updateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update skills/commands to latest compatible versions",
	Long:  "Update skills/commands to the latest versions compatible with manifest ranges.\nUse --skillset to update all members of a skillset.",
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
	updateCmd.Flags().StringVar(&updateSkillset, "skillset", "", "Update all members of a skillset")
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

	// If --skillset flag is set, update all members of that skillset
	if updateSkillset != "" {
		return runUpdateSkillset(ctx, updateSkillset, m, lock, clients)
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
			fmt.Printf("\n%s is already at the latest compatible version.\n", filterName)
		} else {
			fmt.Println("\nEverything is already up to date.")
		}
		return nil
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}
	fmt.Println("\nLock file updated.")

	return nil
}

func runUpdateSkillset(ctx context.Context, ssName string, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client) error {
	// Look up the locked skillset
	lockedSS, ok := lock.Skillsets[ssName]
	if !ok {
		return fmt.Errorf("skillset %q not found in lock file. Run 'amaru add %s --type=skillset' first", ssName, ssName)
	}

	client, ok := clients[lockedSS.Registry]
	if !ok {
		return fmt.Errorf("no client for registry %q", lockedSS.Registry)
	}

	// Fetch current registry index to get latest member definitions
	idx, err := client.FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("fetching registry index: %w", err)
	}

	// Check if the skillset still exists in the registry
	remoteSS, exists := idx.Skillsets[ssName]
	if !exists {
		return fmt.Errorf("skillset %q no longer exists in registry %q", ssName, lockedSS.Registry)
	}

	// If items aren't inline in the index, fetch from the skillset's manifest.json
	if len(remoteSS.Items) == 0 {
		ssManifest, err := client.FetchSkillsetManifest(ctx, ssName, remoteSS.Latest)
		if err != nil {
			return fmt.Errorf("skillset %q has no inline items and manifest fetch failed: %w", ssName, err)
		}
		remoteSS.Items = ssManifest.ToSkillsetItems()
	}

	fmt.Printf("Updating skillset %q (%d members)...\n", ssName, len(remoteSS.Items))

	updated := 0
	added := 0

	// Update existing members and add new ones
	for _, item := range remoteSS.Items {
		itemType := types.ItemType(item.Type)
		spec, inManifest := m.DepsForType(itemType)[item.Name]

		if !inManifest {
			// New member added to skillset — add it
			entries := idx.EntriesForType(itemType)
			entry, ok := entries[item.Name]
			if !ok {
				ui.Warn("  %s %q not found in registry, skipping", item.Type, item.Name)
				continue
			}

			version := entry.Latest
			newSpec := manifest.DependencySpec{Group: ssName}
			if version != "" {
				newSpec.Version = "^" + version
			} else {
				newSpec.Version = "latest"
			}
			if len(m.Registries) > 1 {
				newSpec.Registry = lockedSS.Registry
			}
			m.SetDep(itemType, item.Name, newSpec)

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
			lock.EntriesForType(itemType)[item.Name] = manifest.NewLockedEntry(lockVersion, lockedSS.Registry, hash)
			displayVersion := version
			if displayVersion == "" {
				displayVersion = "latest"
			}
			ui.Check("  Added %s %s@%s (new member)", item.Type, item.Name, displayVersion)
			added++
			continue
		}

		// Existing member — update it
		did, err := updateItem(ctx, m, lock, clients, item.Type, item.Name, spec, lock.EntriesForType(itemType))
		if err != nil {
			return fmt.Errorf("%s %s: %w", item.Type, item.Name, err)
		}
		if did {
			updated++
		}
	}

	// Recompute skillset digest
	var digestItems []string
	var memberList []string
	for _, item := range remoteSS.Items {
		itemType := types.ItemType(item.Type)
		if le, ok := lock.EntriesForType(itemType)[item.Name]; ok {
			digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s", item.Type, item.Name, le.Version))
		}
		memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))
	}

	lock.Skillsets[ssName] = manifest.LockedSkillset{
		Registry:    lockedSS.Registry,
		Digest:      manifest.SkillsetDigest(digestItems),
		Members:     memberList,
		InstalledAt: lockedSS.InstalledAt,
	}

	if updated == 0 && added == 0 {
		fmt.Printf("\nSkillset %q is up to date.\n", ssName)
		return nil
	}

	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}
	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}

	fmt.Printf("\nSkillset %q: %d updated, %d added.\n", ssName, updated, added)
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
