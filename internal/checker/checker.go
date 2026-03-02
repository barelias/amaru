package checker

import (
	"context"
	"fmt"

	"github.com/barelias/amaru/internal/installer"
	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/resolver"
	"github.com/barelias/amaru/internal/types"

	"github.com/Masterminds/semver/v3"
)

// UpdateInfo describes an available update.
type UpdateInfo struct {
	Name        string
	ItemType    string // "skill" or "command"
	Registry    string
	Current     string
	Latest      string
	LatestInRange string // latest compatible with the range
	Category    string // "patch", "minor", "major"
}

// DriftInfo describes a locally modified item.
type DriftInfo struct {
	Name     string
	ItemType string
	Registry string
	Version  string
	LocalHash  string
	RemoteHash string
}

// CheckResult is the output of a check operation.
type CheckResult struct {
	Updates    []UpdateInfo
	Drifts     []DriftInfo
	UpToDate   int
}

// Check compares the local lock against the registries.
func Check(ctx context.Context, projectDir string, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client) (*CheckResult, error) {
	result := &CheckResult{}

	for _, itemType := range types.AllInstallableTypes() {
		deps := m.DepsForType(itemType)
		lockEntries := lock.EntriesForType(itemType)

		for name, spec := range deps {
			regAlias, err := m.ResolveRegistry(spec)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", itemType, name, err)
			}

			client, ok := clients[regAlias]
			if !ok {
				return nil, fmt.Errorf("%s %s: no client for registry %q", itemType, name, regAlias)
			}

			locked, hasLock := lockEntries[name]
			if !hasLock {
				continue
			}

			if err := checkItem(ctx, result, string(itemType), name, regAlias, spec.Version, locked, client, projectDir, m.IsIgnored(name)); err != nil {
				return nil, fmt.Errorf("checking %s %s: %w", itemType, name, err)
			}
		}
	}

	return result, nil
}

func checkItem(ctx context.Context, result *CheckResult, itemType, name, regAlias, constraint string, locked manifest.LockedEntry, client registry.Client, projectDir string, ignored bool) error {
	// Check for version updates
	versions, err := client.ListVersions(ctx, itemType, name)
	if err != nil {
		return fmt.Errorf("listing versions: %w", err)
	}

	latestAll := resolver.LatestAvailable(versions)
	latestInRange, _ := resolver.Resolve(constraint, versions)

	currentV, err := semver.NewVersion(locked.Version)
	if err != nil {
		return fmt.Errorf("parsing current version: %w", err)
	}

	hasUpdate := false
	if latestAll != nil && latestAll.GreaterThan(currentV) {
		latest := latestAll.String()
		latestRange := ""
		if latestInRange != nil {
			latestRange = latestInRange.String()
		}
		result.Updates = append(result.Updates, UpdateInfo{
			Name:          name,
			ItemType:      itemType,
			Registry:      regAlias,
			Current:       locked.Version,
			Latest:        latest,
			LatestInRange: latestRange,
			Category:      resolver.ClassifyUpdate(locked.Version, latest),
		})
		hasUpdate = true
	}

	// Check for local drift (hash mismatch)
	if !ignored && installer.IsInstalled(projectDir, itemType, name) {
		localHash, err := installer.ComputeHash(installedPath(projectDir, itemType, name))
		if err == nil && localHash != locked.Hash {
			result.Drifts = append(result.Drifts, DriftInfo{
				Name:       name,
				ItemType:   itemType,
				Registry:   regAlias,
				Version:    locked.Version,
				LocalHash:  localHash,
				RemoteHash: locked.Hash,
			})
		}
	}

	if !hasUpdate {
		result.UpToDate++
	}

	return nil
}

func installedPath(projectDir, itemType, name string) string {
	return projectDir + "/" + installer.DirForType(itemType) + "/" + name
}
