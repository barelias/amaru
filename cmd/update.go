package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/useamaru/amaru/internal/installer"
	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/resolver"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
)

var (
	updateSkillset  string
	updateNoContext bool
)

var updateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update skills/commands to latest compatible versions",
	Long:  "Update skills/commands to the latest versions compatible with manifest ranges.\nContext mounts declared in amaru.json are synced too (--no-context skips them).\nUse --skillset to update all members of a skillset.",
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
	updateCmd.Flags().BoolVar(&updateNoContext, "no-context", false, "Skip syncing the context mounts declared in amaru.json")
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

	// If filterName matches a skillset, delegate to skillset update
	if filterName != "" {
		if _, isSkillset := m.Skillsets[filterName]; isSkillset {
			return runUpdateSkillset(ctx, filterName, m, lock, clients)
		}
	}

	// Context mounts ride along with a bare update: they are declared in the
	// same manifest, and leaving them to an explicit 'amaru context sync' meant
	// a green update over stale docs.
	syncContext := func() error {
		if updateNoContext || filterName != "" {
			return nil
		}
		return syncManifestContext(ctx, m)
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

	// A bare update covers skillsets too — they used to be reachable only via
	// --skillset/<name>, so a manifest with nothing but skillsets always
	// reported "Everything is already up to date" while members advanced.
	if filterName == "" {
		ssNames := make([]string, 0, len(m.Skillsets))
		for name := range m.Skillsets {
			ssNames = append(ssNames, name)
		}
		sort.Strings(ssNames)
		for _, ssName := range ssNames {
			changed, err := updateSkillsetMembers(ctx, ssName, m, lock, clients)
			if err != nil {
				return fmt.Errorf("skillset %s: %w", ssName, err)
			}
			updated += changed
		}
	}

	if updated == 0 {
		if filterName != "" {
			fmt.Printf("\n%s is already at the latest compatible version.\n", filterName)
		} else {
			fmt.Println("\nEverything is already up to date.")
		}
		return syncContext()
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}
	fmt.Println("\nLock file updated.")

	return syncContext()
}

func runUpdateSkillset(ctx context.Context, ssName string, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client) error {
	changed, err := updateSkillsetMembers(ctx, ssName, m, lock, clients)
	if err != nil {
		return err
	}

	if changed == 0 {
		fmt.Printf("\nSkillset %q is up to date.\n", ssName)
		return nil
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}
	return nil
}

// updateSkillsetMembers brings every member of a skillset to the registry's
// latest, mutating the lock in place. Returns how many members changed
// (updated + added + removed); the caller decides when to save the lock.
func updateSkillsetMembers(ctx context.Context, ssName string, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client) (int, error) {
	// Source of truth is now the manifest
	ssSpec, inManifest := m.Skillsets[ssName]
	if !inManifest {
		return 0, fmt.Errorf("skillset %q not found in manifest. Run 'amaru add %s --type=skillset' first", ssName, ssName)
	}

	regAlias, err := m.ResolveSkillsetRegistry(ssSpec)
	if err != nil {
		return 0, err
	}

	client, ok := clients[regAlias]
	if !ok {
		return 0, fmt.Errorf("no client for registry %q", regAlias)
	}

	// Fetch current registry index
	idx, err := client.FetchIndex(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetching registry index: %w", err)
	}

	remoteSS, exists := idx.Skillsets[ssName]
	if !exists {
		return 0, fmt.Errorf("skillset %q no longer exists in registry %q", ssName, regAlias)
	}

	// Resolve items from manifest if not inline
	if len(remoteSS.Items) == 0 {
		ssManifest, err := client.FetchSkillsetManifest(ctx, ssName, remoteSS.Latest)
		if err != nil {
			return 0, fmt.Errorf("fetching skillset manifest: %w", err)
		}
		remoteSS.Items = ssManifest.ToSkillsetItems()
	}

	// Cross-registry members download from THEIR registry, same as install.
	indexCache := map[string]*registry.RegistryIndex{regAlias: idx}
	resolveItemRegistry := func(item registry.SkillsetItem) (string, registry.Client, *registry.RegistryIndex, error) {
		alias := item.Registry
		if alias == "" {
			alias = regAlias
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

	// Build set of remote members for diffing
	remoteMembers := make(map[string]bool)
	for _, item := range remoteSS.Items {
		remoteMembers[fmt.Sprintf("%s/%s", item.Type, item.Name)] = true
	}

	// Build set of current locked members
	lockedSS := lock.Skillsets[ssName]
	localMembers := make(map[string]bool)
	for _, member := range lockedSS.Members {
		localMembers[member] = true
	}

	fmt.Printf("Updating skillset %q...\n", ssName)

	updated := 0
	added := 0
	removed := 0

	// Resolve every member first, then fetch them in one parallel batch: the
	// per-member download used to run in series, so a skillset paid one full
	// round trip per skill even when nothing had changed.
	type memberPlan struct {
		item    registry.SkillsetItem
		alias   string
		version string
		isNew   bool
		locked  manifest.LockedEntry
	}

	var plans []memberPlan
	for _, item := range remoteSS.Items {
		memberKey := fmt.Sprintf("%s/%s", item.Type, item.Name)

		itemAlias, _, itemIdx, err := resolveItemRegistry(item)
		if err != nil {
			return 0, err
		}
		entries := itemIdx.EntriesForType(types.ItemType(item.Type))
		entry, ok := entries[item.Name]
		if !ok {
			ui.Warn("  %s %q not found in registry %q, skipping", item.Type, item.Name, itemAlias)
			continue
		}

		if !localMembers[memberKey] {
			plans = append(plans, memberPlan{item: item, alias: itemAlias, version: entry.Latest, isNew: true})
			continue
		}

		// Accepted drift stays put: overwriting an ignored member on every
		// update would defeat 'amaru ignore'.
		if m.IsIgnored(item.Name) {
			continue
		}

		locked, hasLock := lock.EntriesForType(types.ItemType(item.Type))[item.Name]
		if !hasLock {
			continue
		}
		plans = append(plans, memberPlan{item: item, alias: itemAlias, version: entry.Latest, locked: locked})
	}

	jobs := make([]downloadJob, len(plans))
	for i, p := range plans {
		_, itemClient, _, err := resolveItemRegistry(p.item)
		if err != nil {
			return 0, err
		}
		jobs[i] = downloadJob{Client: itemClient, ItemType: p.item.Type, Name: p.item.Name, Version: p.version}
	}
	downloadItems(ctx, jobs)

	for i, p := range plans {
		if jobs[i].Err != nil {
			return updated + added + removed, fmt.Errorf("downloading %s %q: %w", p.item.Type, p.item.Name, jobs[i].Err)
		}

		hash, err := installer.Install(".", p.item.Type, p.item.Name, jobs[i].Files)
		if err != nil {
			return updated + added + removed, fmt.Errorf("installing %s %q: %w", p.item.Type, p.item.Name, err)
		}

		lockVersion := p.version
		if lockVersion == "" {
			lockVersion = "latest"
		}
		lockEntries := lock.EntriesForType(types.ItemType(p.item.Type))

		if p.isNew {
			lockEntries[p.item.Name] = manifest.NewLockedEntry(lockVersion, p.alias, hash)
			ui.Check("  Added %s %s@%s (new member)", p.item.Type, p.item.Name, lockVersion)
			added++
			continue
		}

		if hash != p.locked.Hash {
			lockEntries[p.item.Name] = manifest.NewLockedEntry(lockVersion, p.alias, hash)
			ui.Check("  Updated %s %s — content changed", p.item.Type, p.item.Name)
			updated++
		}
	}

	// Remove members that are no longer in the remote skillset
	for _, member := range lockedSS.Members {
		if remoteMembers[member] {
			continue
		}
		parts := strings.SplitN(member, "/", 2)
		if len(parts) != 2 {
			continue
		}
		itemType, itemName := parts[0], parts[1]

		if err := installer.Uninstall(".", itemType, itemName); err != nil {
			ui.Warn("  Failed to remove %s %s: %v", itemType, itemName, err)
			continue
		}
		delete(lock.EntriesForType(types.ItemType(itemType)), itemName)
		ui.Check("  Removed %s %s (no longer in skillset)", itemType, itemName)
		removed++
	}

	// Recompute skillset digest — same shape install writes (alias included),
	// so install and update never ping-pong the digest between formats.
	var digestItems []string
	var memberList []string
	for _, item := range remoteSS.Items {
		itemType := types.ItemType(item.Type)
		if le, ok := lock.EntriesForType(itemType)[item.Name]; ok {
			digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s@%s", item.Type, item.Name, le.Version, le.Registry))
		}
		memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))
	}

	lock.Skillsets[ssName] = manifest.LockedSkillset{
		Registry:    regAlias,
		Digest:      manifest.SkillsetDigest(digestItems),
		Members:     memberList,
		InstalledAt: lockedSS.InstalledAt,
	}

	if updated+added+removed > 0 {
		fmt.Printf("\nSkillset %q: %d updated, %d added, %d removed.\n", ssName, updated, added, removed)
	}
	return updated + added + removed, nil
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

	// No tags found — registry doesn't use per-item version tags.
	// Re-download from default branch and compare hash (same as "latest" path).
	if len(versions) == 0 {
		files, err := client.DownloadFiles(ctx, itemType, name, "")
		if err != nil {
			return false, fmt.Errorf("downloading: %w", err)
		}
		hash, err := installer.Install(".", itemType, name, files)
		if err != nil {
			return false, fmt.Errorf("installing: %w", err)
		}
		if hash != locked.Hash {
			lockEntries[name] = manifest.NewLockedEntry(locked.Version, regAlias, hash)
			ui.Check("Updating %s — content changed [%s]", name, regAlias)
			return true, nil
		}
		return false, nil
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
