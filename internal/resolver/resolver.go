package resolver

import (
	"fmt"
	"sort"

	"github.com/Masterminds/semver/v3"
)

// Resolve finds the best version matching the given constraint from the available versions.
// Returns the highest version that satisfies the constraint.
func Resolve(constraint string, available []*semver.Version) (*semver.Version, error) {
	if len(available) == 0 {
		return nil, fmt.Errorf("no versions available")
	}

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, fmt.Errorf("invalid version constraint %q: %w", constraint, err)
	}

	// Sort descending to find the highest match first
	sorted := make([]*semver.Version, len(available))
	copy(sorted, available)
	sort.Sort(sort.Reverse(semver.Collection(sorted)))

	for _, v := range sorted {
		if c.Check(v) {
			return v, nil
		}
	}

	return nil, fmt.Errorf("no version satisfies constraint %q (available: %v)", constraint, available)
}

// IsUpgradable checks if there's a newer version available within the constraint.
func IsUpgradable(current, constraint string, available []*semver.Version) (bool, *semver.Version, error) {
	currentV, err := semver.NewVersion(current)
	if err != nil {
		return false, nil, fmt.Errorf("invalid current version %q: %w", current, err)
	}

	best, err := Resolve(constraint, available)
	if err != nil {
		return false, nil, err
	}

	if best.GreaterThan(currentV) {
		return true, best, nil
	}
	return false, nil, nil
}

// LatestAvailable returns the highest version from the list, regardless of constraints.
func LatestAvailable(available []*semver.Version) *semver.Version {
	if len(available) == 0 {
		return nil
	}
	sorted := make([]*semver.Version, len(available))
	copy(sorted, available)
	sort.Sort(semver.Collection(sorted))
	return sorted[len(sorted)-1]
}

// ClassifyUpdate categorizes the version change as patch, minor, or major.
func ClassifyUpdate(from, to string) string {
	fromV, err := semver.NewVersion(from)
	if err != nil {
		return "unknown"
	}
	toV, err := semver.NewVersion(to)
	if err != nil {
		return "unknown"
	}

	if toV.Major() > fromV.Major() {
		return "major"
	}
	if toV.Minor() > fromV.Minor() {
		return "minor"
	}
	if toV.Patch() > fromV.Patch() {
		return "patch"
	}
	return "none"
}
