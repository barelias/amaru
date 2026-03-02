package types

// ItemType represents an installable item type.
type ItemType string

const (
	Skill   ItemType = "skill"
	Command ItemType = "command"
	Agent   ItemType = "agent"
)

// AllInstallableTypes returns all item types in canonical order.
func AllInstallableTypes() []ItemType {
	return []ItemType{Skill, Command, Agent}
}

// DirName returns the .claude subdirectory name for this type (e.g. "skills").
func (t ItemType) DirName() string {
	switch t {
	case Skill:
		return "skills"
	case Command:
		return "commands"
	case Agent:
		return "agents"
	default:
		return string(t) + "s"
	}
}

// Singular returns the display name (e.g. "skill").
func (t ItemType) Singular() string {
	return string(t)
}

// Plural returns the plural display name (e.g. "skills").
func (t ItemType) Plural() string {
	return t.DirName()
}
