# Amaru — Skills, Commands & Agents Manager

Use the `amaru` CLI to manage Claude Code skills, commands, and agents from centralized GitHub registries.

## Core Workflow

```bash
amaru init                              # Set up amaru.json with registry URLs
amaru browse                            # Discover available items
amaru add <name>                        # Add a skill (default type)
amaru add <name> --type command         # Add a command
amaru add <name> --type agent           # Add an agent
amaru add <name> --type skillset        # Add a skillset (expands to individual items)
amaru install                           # Install/sync everything from manifest
amaru check                             # Check for updates and local drift
amaru update [name]                     # Update to latest compatible versions
amaru update --skillset <name>          # Update all members of a skillset
amaru list                              # Show installed items with status
```

## Key Concepts

- **Manifest** (`amaru.json`): Declares registries, version constraints, and dependencies. Committed to the repo.
- **Lock file** (`amaru.lock`): Resolved versions, content hashes, skillset digests. Committed for reproducibility.
- **Version constraints**: `^1.0.0` (minor+patch), `~1.0.0` (patch only), `1.0.0` (exact), `latest` (unversioned, hash-tracked).
- **Skillsets**: Registry-defined groups that expand to individual items on install. Use `--type skillset` to add.
- **Registries**: GitHub repos with `amaru_registry.json` at root and items in `.amaru_registry/skills/`, `.amaru_registry/commands/`, `.amaru_registry/agents/`.

## When to Use Each Command

| Situation | Command |
|-----------|---------|
| Starting a new project | `amaru init` then `amaru browse` |
| Adding a specific skill | `amaru add <name>` |
| New team member onboarding | `amaru install` (reads existing manifest) |
| Checking if anything is outdated | `amaru check` or `amaru check --quiet` |
| Updating everything | `amaru update` |
| Updating one skillset | `amaru update --skillset <name>` |
| Accepting local edits to a skill | `amaru ignore <name>` |
| Setting up shared docs | `amaru context init` |
| Creating a new registry | `amaru repo init` |

## Registry URL Formats

`amaru init` accepts any GitHub URL format:
- `github:org/repo` (canonical)
- `git@github.com:org/repo.git` (SSH)
- `https://github.com/org/repo`
- `github.com/org/repo` (bare domain)

All formats are normalized automatically.

## Files Managed

| Path | Purpose |
|------|---------|
| `amaru.json` | Manifest (version constraints, registries) |
| `amaru.lock` | Lock file (resolved versions, hashes) |
| `.claude/skills/<name>/` | Installed skills |
| `.claude/commands/<name>/` | Installed commands |
| `.claude/agents/<name>/` | Installed agents |

## Context Documentation

```bash
amaru context init    # Sparse-clone context docs from registry
amaru context sync    # Pull latest context
amaru context push    # Push local changes back
```

Context docs are synced to `docs/context/` (configurable) and auto-sync via git hooks.
