package cmd

import (
	"context"
	"fmt"

	"github.com/useamaru/amaru/internal/installer"
	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/resolver"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var installForce bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install skills and commands from manifest",
	Long:  "Reads amaru.json, authenticates with registries, resolves versions, copies files, and generates amaru.lock.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInstall(cmd.Context())
	},
}

func init() {
	installCmd.Flags().BoolVar(&installForce, "force", false, "Reinstall even if lock exists and versions are compatible")
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

	for _, itemType := range types.AllInstallableTypes() {
		deps := m.DepsForType(itemType)
		if len(deps) > 0 {
			ui.Header("Installing %s...", itemType.Plural())
			lockEntries := lock.EntriesForType(itemType)
			for name, spec := range deps {
				if err := installItem(ctx, m, lock, clients, string(itemType), name, spec, lockEntries); err != nil {
					return fmt.Errorf("%s %s: %w", itemType, name, err)
				}
			}
		}
	}

	// Install skillsets
	if len(m.Skillsets) > 0 {
		ui.Header("Installing skillsets...")
		for name, spec := range m.Skillsets {
			if err := installSkillset(ctx, name, spec, m, lock, clients); err != nil {
				return fmt.Errorf("skillset %s: %w", name, err)
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

	// Check if already installed and up to date. Presence alone doesn't cut
	// it: the lock may have advanced on another machine (the skills dir is
	// gitignored, the lock is versioned) — converge unless the drift was
	// explicitly accepted with 'amaru ignore'.
	if !installForce {
		if locked, ok := lockEntries[name]; ok {
			if installer.IsInstalled(".", itemType, name) &&
				(m.IsIgnored(name) || installer.LocalHash(".", itemType, name) == locked.Hash) {
				displayVersion := locked.Version
				if displayVersion == "" {
					displayVersion = "latest"
				}
				ui.Check("%s@%s (%s) — already installed", name, displayVersion, regAlias)
				return nil
			}
		}
	}

	// Resolve version (returns "" for "latest" constraint)
	resolved, err := resolveVersion(ctx, client, itemType, name, spec.Version)
	if err != nil {
		return err
	}

	// Download files (empty version downloads from default branch)
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
	lockVersion := resolved
	if lockVersion == "" {
		lockVersion = "latest"
	}
	lockEntries[name] = manifest.NewLockedEntry(lockVersion, regAlias, hash)

	displayVersion := resolved
	if displayVersion == "" {
		displayVersion = "latest"
	}
	ui.Check("%s@%s (%s)", name, displayVersion, regAlias)
	return nil
}

func installSkillset(ctx context.Context, name string, spec manifest.SkillsetSpec, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client) error {
	regAlias, err := m.ResolveSkillsetRegistry(spec)
	if err != nil {
		return err
	}

	client, ok := clients[regAlias]
	if !ok {
		return fmt.Errorf("no client for registry %q", regAlias)
	}

	// No presence-only short-circuit here: it used to hide member advancement
	// entirely (a content bump with an unchanged member list never installed,
	// and the lock digest went stale). The per-member loop below does the
	// skipping — cheap when everything is current, correct when it isn't.

	idx, err := client.FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("fetching registry index: %w", err)
	}

	skillset, found := idx.Skillsets[name]
	if !found {
		return fmt.Errorf("skillset %q not found in registry %q", name, regAlias)
	}

	// Resolve items from manifest if not inline
	if len(skillset.Items) == 0 {
		ssManifest, err := client.FetchSkillsetManifest(ctx, name, skillset.Latest)
		if err != nil {
			return fmt.Errorf("fetching skillset manifest: %w", err)
		}
		skillset.Items = ssManifest.ToSkillsetItems()
	}

	// Cache per-registry indexes so cross-registry skillsets only fetch
	// each alias' index once, not once per member.
	indexCache := map[string]*registry.RegistryIndex{regAlias: idx}
	resolveItemRegistry := func(item registry.SkillsetItem) (string, registry.Client, *registry.RegistryIndex, error) {
		alias := item.Registry
		if alias == "" {
			alias = regAlias
		}
		if _, configured := m.Registries[alias]; !configured {
			return "", nil, nil, fmt.Errorf(
				"skillset %q references registry %q which is not configured in amaru.json (configure it under \"registries\" or remove the cross-registry reference)",
				name, alias)
		}
		c, ok := clients[alias]
		if !ok {
			return "", nil, nil, fmt.Errorf("no client built for registry %q", alias)
		}
		if cached, ok := indexCache[alias]; ok {
			return alias, c, cached, nil
		}
		fetched, err := c.FetchIndex(ctx)
		if err != nil {
			return "", nil, nil, fmt.Errorf("fetching index for registry %q: %w", alias, err)
		}
		indexCache[alias] = fetched
		return alias, c, fetched, nil
	}

	var digestItems []string
	var memberList []string
	upToDate := 0
	for _, item := range skillset.Items {
		itemType := types.ItemType(item.Type)

		itemAlias, itemClient, itemIdx, err := resolveItemRegistry(item)
		if err != nil {
			return err
		}
		entries := itemIdx.EntriesForType(itemType)
		entry, ok := entries[item.Name]
		if !ok {
			ui.Warn("  %s %q not found in registry %q, skipping", item.Type, item.Name, itemAlias)
			continue
		}

		version := entry.Latest
		lockEntries := lock.EntriesForType(itemType)

		// Skip only when the member is genuinely current: locked at the
		// registry's latest version AND the disk matches the lock (or the
		// drift was accepted with 'amaru ignore'). Presence alone used to
		// skip here, so a member content bump never reached the disk.
		if !installForce {
			if locked, hasLock := lockEntries[item.Name]; hasLock {
				lockVersion := version
				if lockVersion == "" {
					lockVersion = "latest"
				}
				current := locked.Version == lockVersion &&
					installer.IsInstalled(".", item.Type, item.Name) &&
					(m.IsIgnored(item.Name) || installer.LocalHash(".", item.Type, item.Name) == locked.Hash)
				if current {
					digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s@%s", item.Type, item.Name, lockVersion, itemAlias))
					memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))
					upToDate++
					continue
				}
			}
		}

		files, err := itemClient.DownloadFiles(ctx, item.Type, item.Name, version)
		if err != nil {
			return fmt.Errorf("downloading %s %q from registry %q: %w", item.Type, item.Name, itemAlias, err)
		}

		hash, err := installer.Install(".", item.Type, item.Name, files)
		if err != nil {
			return fmt.Errorf("installing %s %q: %w", item.Type, item.Name, err)
		}

		lockVersion := version
		if lockVersion == "" {
			lockVersion = "latest"
		}
		lockEntries[item.Name] = manifest.NewLockedEntry(lockVersion, itemAlias, hash)

		// Encode the source alias into the digest so cross-registry skillsets
		// invalidate correctly when an item moves to a different registry.
		digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s@%s", item.Type, item.Name, lockVersion, itemAlias))
		memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))

		displayVersion := version
		if displayVersion == "" {
			displayVersion = "latest"
		}
		if itemAlias != regAlias {
			ui.Check("  %s %s@%s [%s]", item.Type, item.Name, displayVersion, itemAlias)
		} else {
			ui.Check("  %s %s@%s", item.Type, item.Name, displayVersion)
		}
	}

	lock.Skillsets[name] = manifest.LockedSkillset{
		Registry:    regAlias,
		Digest:      manifest.SkillsetDigest(digestItems),
		Members:     memberList,
		InstalledAt: "",
	}

	if upToDate == len(memberList) {
		ui.Check("skillset %s (%d members) — already installed", name, len(memberList))
	}

	return nil
}

func resolveVersion(ctx context.Context, client registry.Client, itemType, name, constraint string) (string, error) {
	// "latest" means unversioned — download from default branch
	if constraint == "latest" {
		return "", nil
	}

	versions, err := client.ListVersions(ctx, itemType, name)
	if err != nil {
		return "", fmt.Errorf("listing versions: %w", err)
	}

	// No tags found — registry doesn't use per-item version tags.
	// Return empty so DownloadFiles fetches from default branch.
	if len(versions) == 0 {
		return "", nil
	}

	best, err := resolver.Resolve(constraint, versions)
	if err != nil {
		return "", err
	}

	return best.String(), nil
}
