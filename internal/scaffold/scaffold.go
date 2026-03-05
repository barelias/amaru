package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RepoConfig holds the parameters for scaffolding a new registry repo.
type RepoConfig struct {
	Dir     string
	Project string // Initial project name (optional)
}

// ScaffoldRepo creates the full registry repo structure.
func ScaffoldRepo(cfg RepoConfig) error {
	dirs := []string{
		".amaru_registry/skills",
		".amaru_registry/commands",
		".amaru_registry/agents",
		".amaru_registry/context",
		".amaru_registry/.sparse-profiles",
	}

	if cfg.Project != "" {
		dirs = append(dirs,
			filepath.Join(".amaru_registry", "context", cfg.Project, "brainstorms"),
			filepath.Join(".amaru_registry", "context", cfg.Project, "plans"),
			filepath.Join(".amaru_registry", "context", cfg.Project, "solutions"),
		)
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, d), 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	// Write amaru_registry.json
	registryJSON := map[string]interface{}{
		"amaru_version": "1",
		"updated_at":    "",
		"skills":        map[string]interface{}{},
		"commands":      map[string]interface{}{},
		"agents":        map[string]interface{}{},
		"skillsets":     map[string]interface{}{},
	}
	if err := writeJSON(filepath.Join(cfg.Dir, "amaru_registry.json"), registryJSON); err != nil {
		return err
	}

	// Write root AGENTS.md
	if err := os.WriteFile(filepath.Join(cfg.Dir, "AGENTS.md"), []byte(RootAgentsMD()), 0644); err != nil {
		return err
	}

	// Write .gitkeep files in empty directories
	for _, d := range []string{".amaru_registry/skills", ".amaru_registry/commands", ".amaru_registry/agents"} {
		gitkeep := filepath.Join(cfg.Dir, d, ".gitkeep")
		if err := os.WriteFile(gitkeep, []byte(""), 0644); err != nil {
			return err
		}
	}

	// Write per-project files if project specified
	if cfg.Project != "" {
		agentsContent := ProjectAgentsMD(cfg.Project)
		if err := os.WriteFile(filepath.Join(cfg.Dir, ".amaru_registry", "context", cfg.Project, "AGENTS.md"), []byte(agentsContent), 0644); err != nil {
			return err
		}

		profileContent := SparseProfile(cfg.Project)
		if err := os.WriteFile(filepath.Join(cfg.Dir, ".amaru_registry", ".sparse-profiles", cfg.Project), []byte(profileContent), 0644); err != nil {
			return err
		}
	}

	return nil
}

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// RootAgentsMD returns the template for the root AGENTS.md.
func RootAgentsMD() string {
	return `# Registry Structure

This repository is an amaru registry that provides skills, commands, agents,
and context documentation for Claude Code projects.

## Layout

` + "```" + `
registry/
├── amaru_registry.json        # Package index (auto-updated by CI)
├── AGENTS.md                  # This file — top-level navigation
└── .amaru_registry/           # All registry content
    ├── .sparse-profiles/      # Sapling sparse profiles for selective cloning
    │   └── <project-name>     # One profile per consuming project
    ├── skills/                # Claude Code skills (versioned packages)
    │   └── <skill-name>/
    │       ├── manifest.json
    │       └── skill.md
    ├── commands/              # Claude Code commands (versioned packages)
    │   └── <command-name>/
    │       ├── manifest.json
    │       └── command.md
    ├── agents/                # Claude Code agent definitions (versioned packages)
    │   └── <agent-name>/
    │       ├── manifest.json
    │       └── agent.md
    └── context/               # Project context documentation (NOT versioned)
        └── <project-name>/
            ├── AGENTS.md      # Per-project navigation + repo info
            ├── brainstorms/   # Early-stage ideas and explorations
            ├── plans/         # Concrete implementation plans
            └── solutions/     # Finalized designs and decisions
` + "```" + `

## Versioning

Skills, commands, and agents are versioned via git tags:
- ` + "`skill/<name>/<semver>`" + `
- ` + "`command/<name>/<semver>`" + `
- ` + "`agent/<name>/<semver>`" + `

Context documentation is NOT versioned — it is synced via sparse checkout.

## Consuming This Registry

` + "```bash" + `
# In your project:
amaru init                    # Point to this registry
amaru install                 # Install skills/commands/agents
amaru context init            # Set up context sync
amaru context sync            # Pull latest context
` + "```" + `

## Sparse Profiles

The ` + "`.amaru_registry/.sparse-profiles/`" + ` directory contains Sapling sparse profiles.
Each profile is named after a project and defines which paths that project
needs from this repository. If Sapling is not available, amaru falls back
to git sparse-checkout.
`
}

// ProjectAgentsMD returns the template for a per-project AGENTS.md.
func ProjectAgentsMD(project string) string {
	return fmt.Sprintf(`# %s — Context Documentation

This directory contains context documentation for the **%s** project,
following the compound engineering docs pattern.

## Structure

- **brainstorms/** — Early-stage ideas, explorations, and rough thinking.
  Files follow the naming convention: `+"`YYYY-MM-DD-<topic>-brainstorm.md`"+`

- **plans/** — Concrete implementation plans with specific steps,
  dependencies, and success criteria.
  Files follow: `+"`YYYY-MM-DD-<type>-<title>-plan.md`"+`
  Types: feat, fix, refactor, guide

- **solutions/** — Finalized designs, architectural decisions, and
  completed implementation notes organized by category:
  - build-errors/
  - feature-implementations/
  - integration-issues/
  - runtime-errors/
  - ui-bugs/
  - ui-patterns/

## Workflow

1. Start in `+"`brainstorms/`"+` with open-ended exploration
2. Promote promising ideas to `+"`plans/`"+` with concrete details
3. Move completed work to `+"`solutions/`"+` as reference documentation

## Frontmatter

All documents use YAML frontmatter with at minimum:
`+"```yaml"+`
---
title: "Document Title"
date: YYYY-MM-DD
status: active|done|pending
---
`+"```"+`

## Sync

This context is managed by amaru:
`+"```bash"+`
amaru context sync    # Pull latest from centralized repo
amaru context push    # Push local changes back
`+"```"+`
`, project, project)
}

// SparseProfile returns the content of a Sapling sparse profile for a project.
func SparseProfile(project string) string {
	return fmt.Sprintf(`# Sapling sparse profile for %s

[include]
.amaru_registry/context/%s/**
AGENTS.md
amaru_registry.json

[exclude]
*
`, project, project)
}
