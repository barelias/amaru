package registry

import (
	"context"

	"github.com/Masterminds/semver/v3"
)

// RegistryIndex is the parsed registry.json from the remote registry.
type RegistryIndex struct {
	UpdatedAt string                    `json:"updated_at"`
	Skills    map[string]RegistryEntry  `json:"skills,omitempty"`
	Commands  map[string]RegistryEntry  `json:"commands,omitempty"`
}

// RegistryEntry is one skill or command in the registry index.
type RegistryEntry struct {
	Latest      string   `json:"latest"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description"`
}

// ItemManifest is the manifest.json inside a skill/command directory in the registry.
type ItemManifest struct {
	Name        string           `json:"name"`
	Type        string           `json:"type"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
	Author      string           `json:"author"`
	Changelog   []ChangelogEntry `json:"changelog,omitempty"`
	Files       []string         `json:"files"`
	Tags        []string         `json:"tags,omitempty"`
}

// ChangelogEntry records a version change.
type ChangelogEntry struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Note    string `json:"note"`
}

// File represents a downloaded file from the registry.
type File struct {
	Path    string // Relative path within the skill/command directory
	Content []byte
}

// Client is the interface for accessing a remote registry.
type Client interface {
	// FetchIndex downloads and parses the registry.json index.
	FetchIndex(ctx context.Context) (*RegistryIndex, error)

	// ListVersions returns all available versions for a skill or command.
	// itemType is "skill" or "command".
	ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error)

	// DownloadFiles downloads all files for a specific version of a skill or command.
	DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error)
}
