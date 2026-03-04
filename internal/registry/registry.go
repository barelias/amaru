package registry

import (
	"context"

	"github.com/Masterminds/semver/v3"
	"github.com/barelias/amaru/internal/types"
)

// RegistryIndex is the parsed amaru_registry.json from the remote registry.
type RegistryIndex struct {
	AmaruVersion string                      `json:"amaru_version"`
	UpdatedAt    string                      `json:"updated_at"`
	Skills       map[string]RegistryEntry    `json:"skills,omitempty"`
	Commands     map[string]RegistryEntry    `json:"commands,omitempty"`
	Agents       map[string]RegistryEntry    `json:"agents,omitempty"`
	Skillsets    map[string]SkillsetEntry    `json:"skillsets,omitempty"`
}

// EntriesForType returns the registry entries for a given item type.
func (idx *RegistryIndex) EntriesForType(t types.ItemType) map[string]RegistryEntry {
	switch t {
	case types.Skill:
		return idx.Skills
	case types.Command:
		return idx.Commands
	case types.Agent:
		return idx.Agents
	default:
		return nil
	}
}

// RegistryEntry is one skill or command in the registry index.
type RegistryEntry struct {
	Latest      string   `json:"latest"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description"`
}

// SkillsetEntry is a named group of skills/commands/agents in the registry index.
// Skillsets expand to individual items on install (VS Code Extension Pack pattern).
type SkillsetEntry struct {
	Description string         `json:"description"`
	Tags        []string       `json:"tags,omitempty"`
	Items       []SkillsetItem `json:"items"`
}

// SkillsetItem is one member of a skillset.
type SkillsetItem struct {
	Type string `json:"type"` // "skill", "command", or "agent"
	Name string `json:"name"`
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

	// ListVersions returns all available versions for an item.
	// itemType is "skill", "command", or "agent".
	ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error)

	// DownloadFiles downloads all files for a specific version of an item.
	DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error)
}
