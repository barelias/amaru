---
title: "feat: Add repo management commands for registry authoring"
type: feat
status: completed
date: 2026-03-05
---

# feat: Add repo management commands for registry authoring

## Overview

Currently `amaru repo init` scaffolds an empty registry, but there are no commands to **manage** it afterward. Registry maintainers must hand-edit `amaru_registry.json` and manually create directories — error-prone and tedious.

This plan adds a full suite of `amaru repo` subcommands so maintainers can add items, define skillsets, remove items, validate consistency, tag versions, and list contents — all from the CLI. It also updates the self-hosted `amaru-usage` skill, README, CLAUDE.md, and AGENTS.md to document these new commands.

## Problem Statement

A registry repo without management commands is like a package manager without `publish`. Today, creating a skill in a registry requires:

1. Manually create `.amaru_registry/skills/<name>/` directory
2. Manually write `manifest.json` with the correct schema
3. Manually write `skill.md` with proper frontmatter
4. Manually edit `amaru_registry.json` to add the entry
5. Manually create a git tag in the correct `skill/<name>/<semver>` format

Every step can go wrong silently. There's no validation, no templates, no guardrails.

## Proposed Solution

Seven new subcommands under `amaru repo`:

| Command | Purpose |
|---------|---------|
| `amaru repo add <name>` | Create a new item (skill/command/agent) with template files + index entry |
| `amaru repo add <name> --type skillset` | Define a skillset in the registry index |
| `amaru repo remove <name>` | Remove an item or skillset from the registry |
| `amaru repo list` | List all items in the local registry |
| `amaru repo validate` | Check index-to-filesystem consistency |
| `amaru repo tag <name> <version>` | Tag a new version (git tag + update index/manifest) |
| `amaru repo info <name>` | Show details about a specific item |

Plus documentation updates to README.md, CLAUDE.md, AGENTS.md, and the `amaru-usage` skill.

## Technical Approach

### Shared Infrastructure

Before implementing commands, build shared utilities that all `repo` commands need:

#### Registry Root Detection — `internal/scaffold/detect.go`

```go
// FindRegistryRoot looks for amaru_registry.json in the given directory.
// Does NOT walk up — registry commands must run at the root.
func FindRegistryRoot(dir string) (string, error)
```

#### Local Index Load/Save — `internal/scaffold/index.go`

```go
// LoadLocalIndex reads and parses amaru_registry.json from disk.
func LoadLocalIndex(dir string) (*registry.RegistryIndex, error)

// SaveLocalIndex writes amaru_registry.json atomically (temp file + rename).
func SaveLocalIndex(dir string, idx *registry.RegistryIndex) error
```

Atomic writes via temp file + `os.Rename` prevent partial writes on crash.

#### Name Validation — `internal/types/validate.go`

```go
// ValidateItemName checks that a name is safe for directories, git tags, and JSON keys.
// Rules: lowercase alphanumeric + hyphens, starts with letter, 2-64 chars.
// Pattern: ^[a-z][a-z0-9-]{1,63}$
func ValidateItemName(name string) error
```

#### Content Templates — `internal/scaffold/templates.go`

Templates for each item type that generate valid `manifest.json` + content files:

```go
func SkillManifest(name, description, author string) ItemManifest
func SkillTemplate(name, description string) string  // skill.md with frontmatter

func CommandManifest(name, description, author string) ItemManifest
func CommandTemplate(name, description string) string  // command.md with frontmatter

func AgentManifest(name, description, author string) ItemManifest
func AgentTemplate(name, description string) string  // agent.md with frontmatter
```

### Phase 1: Core CRUD Commands

#### `amaru repo add <name>` — `cmd/repo_add.go`

**Flags:**
- `--type` / `-t`: item type (skill|command|agent|skillset), default "skill"
- `--description` / `-d`: item description (optional, placeholder if omitted)
- `--author` / `-a`: author name (optional, defaults to git `user.name`)
- `--tags`: comma-separated tags (optional)

**Flow for skill/command/agent:**
1. Detect registry root (look for `amaru_registry.json` in CWD)
2. Validate name (`ValidateItemName`)
3. Load local index → check for name collision (across all types)
4. Create directory: `.amaru_registry/<type>s/<name>/`
5. Generate and write `manifest.json` from template
6. Generate and write content file (`skill.md`, `command.md`, or `agent.md`) from template
7. Add entry to index (latest: "", description, tags)
8. Update `updated_at` timestamp
9. Save index atomically
10. Print success + next steps ("Edit the content file, then `amaru repo tag <name> 1.0.0`")

**Flow for skillset:**
1. Detect registry root, validate name
2. Parse `--items` flag: comma-separated `type/name` pairs (e.g., `skill/research,command/bootstrap`)
3. Validate all referenced items exist in the index
4. Add skillset entry to index
5. Save index

**Idempotency:** Error if name already exists. No `--force` overwrite.

**Skillset-specific flag:**
- `--items`: comma-separated `type/name` members (required for skillsets)

#### `amaru repo remove <name>` — `cmd/repo_remove.go`

**Flags:**
- `--type` / `-t`: item type (skill|command|agent|skillset), default "skill"
- `--force` / `-f`: skip skillset dependency check

**Flow for skill/command/agent:**
1. Detect registry root, load index
2. Check if item exists in index → error if not
3. Check if item is referenced by any skillset → block unless `--force`
4. Remove entry from index
5. Remove directory `.amaru_registry/<type>s/<name>/` (if it exists)
6. Update `updated_at`, save index
7. Print success + warn about orphaned git tags (informational, don't delete them)

**Flow for skillset:**
1. Remove skillset entry from index (no filesystem changes needed)

#### `amaru repo list` — `cmd/repo_list.go`

**Flags:**
- `--type` / `-t`: filter by type (optional)
- `--json`: machine-readable JSON output

**Output (human-readable):**
```
Skills (3)
  research        v1.2.0   Tools for deep codebase research
  plan            v2.0.1   Implementation planning workflow
  amaru-usage     latest   How to use amaru CLI

Commands (1)
  dev/bootstrap   v1.0.0   Project bootstrapping script

Agents (0)

Skillsets (1)
  starter-pack    3 items  Essential skills for new projects
```

**Output (JSON):** Full `amaru_registry.json` content, pretty-printed.

#### `amaru repo info <name>` — `cmd/repo_info.go`

**Flags:**
- `--type` / `-t`: item type (default "skill")

**Output:**
```
Name:        research
Type:        skill
Version:     v1.2.0 (latest)
Description: Tools for deep codebase research
Author:      barelias
Tags:        research, tooling
Files:       skill.md, examples/
Skillsets:   starter-pack

manifest.json: .amaru_registry/skills/research/manifest.json
Content:       .amaru_registry/skills/research/skill.md
```

### Phase 2: Validation & Versioning

#### `amaru repo validate` — `cmd/repo_validate.go`

**Checks (all read-only):**

| Check | Severity |
|-------|----------|
| Index entry has matching directory | Error |
| Directory has valid `manifest.json` | Error |
| `manifest.json` name matches directory name | Error |
| `manifest.json` type matches parent type dir | Error |
| `manifest.json` files array matches actual files | Warning |
| Orphaned directories not in index | Warning |
| Skillset members all exist in index | Error |
| Description drift between index and manifest | Warning |
| `latest` in index matches `version` in manifest | Warning |
| Valid item names (no illegal characters) | Error |

**Output:**
```
Validating registry at /path/to/registry...

  ✓ skills/research — OK
  ✓ skills/plan — OK
  ✗ skills/broken — manifest.json not found
  ! skills/old-thing — orphaned directory (not in index)
  ✓ commands/bootstrap — OK
  ✓ skillsets/starter-pack — all 3 members present

Errors: 1  Warnings: 1  OK: 4
```

**Exit code:** Non-zero if any errors (for CI usage).

#### `amaru repo tag <name> <version>` — `cmd/repo_tag.go`

**Flags:**
- `--type` / `-t`: item type (default "skill")
- `--note` / `-n`: changelog note (optional)
- `--push`: push the tag to remote after creation (optional)

**Flow:**
1. Detect registry root, load index
2. Validate item exists in index and on filesystem
3. Validate version is valid semver (via `semver.NewVersion`)
4. Check version is not already tagged (via `git tag -l "<type>/<name>/<version>"`)
5. Update `manifest.json` version field
6. If `--note` provided, append to changelog array in `manifest.json`
7. Update index `latest` field to new version
8. Update `updated_at`, save index
9. Stage changes: `git add amaru_registry.json .amaru_registry/<type>s/<name>/manifest.json`
10. Commit: `git commit -m "release: <type>/<name>/<version>"`
11. Create annotated git tag: `git tag -a <type>/<name>/<version> -m "<type>/<name> v<version>"`
12. Print success + suggest `git push --follow-tags` (or auto-push if `--push`)

**Guards:**
- Require clean working tree (except the files this command modifies)
- Require git repo
- Block re-tagging existing versions (no `--force` for safety)

### Phase 3: Documentation Updates

All documentation must be updated to reflect the new commands. This is a **required** part of the feature, not optional cleanup.

#### README.md Updates

Add a new section **"Registry Management"** after the existing "Self-Hosted Registry" section:

```markdown
## Registry Management

Manage items in a registry repository:

### `amaru repo add <name> [--type skill|command|agent|skillset]`

Create a new item in the registry with template files.

### `amaru repo remove <name> [--type skill|command|agent|skillset]`

Remove an item from the registry index and delete its files.

### `amaru repo list [--type skill|command|agent|skillset] [--json]`

List all items in the local registry.

### `amaru repo validate`

Check registry consistency (index vs filesystem).

### `amaru repo tag <name> <version> [--type skill|command|agent]`

Tag a new version of an item.

### `amaru repo info <name> [--type skill|command|agent]`

Show details about a specific item.
```

Also update the existing `amaru repo init` section's "Next steps" to reference the new commands.

#### CLAUDE.md Updates

Add to the "Key Conventions" section:

```markdown
- **Registry management**: `amaru repo` subcommands modify `amaru_registry.json` and `.amaru_registry/` contents.
  The index file is the source of truth for what's published; git tags are the source of truth for versions.
  All `repo` commands require CWD to contain `amaru_registry.json`.
- **Documentation sync**: When adding repo commands, always update README.md, AGENTS.md, CLAUDE.md,
  and the `amaru-usage` skill to reflect new CLI capabilities.
```

Add to the "Project Structure" section:

```
  scaffold/       # Registry repo scaffolding, templates, local index I/O, name validation
```

#### AGENTS.md Updates

Add to the "Key Data Flow" section:

```markdown
6. **Repo Add**: `cmd/repo_add.go` → `scaffold.FindRegistryRoot()` → `scaffold.LoadLocalIndex()` → `scaffold.SkillTemplate()` → write files → `scaffold.SaveLocalIndex()`
7. **Repo Tag**: `cmd/repo_tag.go` → validate item exists → update manifest.json + index → git commit → git tag
8. **Repo Validate**: `cmd/repo_validate.go` → `scaffold.LoadLocalIndex()` → walk `.amaru_registry/` → cross-reference entries vs filesystem
```

#### `amaru-usage` Skill Update

Add a new section to `.amaru_registry/skills/amaru-usage/skill.md`:

```markdown
## Registry Authoring

```bash
amaru repo init /path/to/registry       # Scaffold empty registry
amaru repo add my-skill                  # Create new skill with templates
amaru repo add my-cmd --type command     # Create new command
amaru repo add pack --type skillset --items "skill/foo,skill/bar"
amaru repo list                          # Show all items
amaru repo validate                      # Check consistency
amaru repo tag my-skill 1.0.0           # Tag version + update index
amaru repo remove old-skill              # Remove from registry
amaru repo info my-skill                 # Show item details
```

| Situation | Command |
|-----------|---------|
| Creating a new registry | `amaru repo init` |
| Adding a skill to registry | `amaru repo add <name>` |
| Publishing a version | `amaru repo tag <name> <version>` |
| Checking registry health | `amaru repo validate` |
| Removing unused items | `amaru repo remove <name>` |
```

Also update `manifest.json` files array to include any new referenced files.

### Phase 4: Scaffold Fix

Update `internal/scaffold/scaffold.go` to include `"skillsets": {}` in the generated `amaru_registry.json` — currently missing, which creates inconsistency with the `RegistryIndex` struct that supports skillsets.

## System-Wide Impact

- **Interaction graph**: `repo add` writes to filesystem + `amaru_registry.json`. `repo tag` additionally runs `git commit` + `git tag`. No callbacks/observers involved — purely local operations.
- **Error propagation**: All errors bubble up via `fmt.Errorf("context: %w", err)` to Cobra's `RunE`. No retries needed — these are local filesystem + git operations.
- **State lifecycle risks**: Partial failure during `repo tag` (e.g., crash after committing but before tagging) leaves a commit without a tag. `repo validate` would catch the `latest` field not matching any tag. Recovery: re-run `repo tag`.
- **API surface parity**: Consumer-side `amaru add --type X` mirrors `amaru repo add --type X`. Flag names are consistent.
- **Integration test scenarios**: (1) `repo init` → `repo add` → `repo validate` → `repo tag` full lifecycle. (2) `repo add` then `repo remove` with skillset dependency. (3) `repo validate` on a hand-edited registry with inconsistencies.

## Acceptance Criteria

### Functional Requirements

- [x] `amaru repo add <name>` creates directory, manifest.json, content template, and updates index — `cmd/repo_add.go`
- [x] `amaru repo add <name> --type skillset --items "..."` creates skillset entry in index — `cmd/repo_add.go`
- [x] `amaru repo remove <name>` removes index entry + directory, blocks if referenced by skillset — `cmd/repo_remove.go`
- [x] `amaru repo list` shows all items grouped by type with version/description — `cmd/repo_list.go`
- [x] `amaru repo list --json` outputs machine-readable JSON — `cmd/repo_list.go`
- [x] `amaru repo validate` checks all consistency rules, exits non-zero on errors — `cmd/repo_validate.go`
- [x] `amaru repo tag <name> <version>` updates manifest + index, creates annotated git tag — `cmd/repo_tag.go`
- [x] `amaru repo info <name>` shows item details including skillset membership — `cmd/repo_info.go`
- [x] Name validation rejects invalid names (spaces, special chars, too long) — `internal/types/validate.go`
- [x] All commands detect registry root via `amaru_registry.json` presence — `internal/scaffold/detect.go`
- [x] Index writes are atomic (temp file + rename) — `internal/scaffold/index.go`
- [x] Scaffold includes `"skillsets": {}` in generated index — `internal/scaffold/scaffold.go`

### Documentation Requirements

- [x] README.md updated with Registry Management section and all new command signatures
- [x] CLAUDE.md updated with registry management conventions and documentation sync requirement
- [x] AGENTS.md updated with new data flows for repo commands
- [x] `amaru-usage` skill updated with registry authoring section and table

### Testing Requirements

- [x] Unit tests for `ValidateItemName` — `internal/types/validate_test.go`
- [x] Unit tests for `LoadLocalIndex` / `SaveLocalIndex` — `internal/scaffold/index_test.go`
- [x] Unit tests for template generation — `internal/scaffold/templates_test.go`
- [x] Integration tests for each `repo` subcommand using `t.TempDir()` — `cmd/repo_test.go`
- [x] Table-driven tests following existing patterns (see `internal/registry/github_test.go`)

## File Inventory

### New Files

| File | Purpose |
|------|---------|
| `cmd/repo_add.go` | `amaru repo add` command (skill/command/agent/skillset) |
| `cmd/repo_remove.go` | `amaru repo remove` command |
| `cmd/repo_list.go` | `amaru repo list` command |
| `cmd/repo_validate.go` | `amaru repo validate` command |
| `cmd/repo_tag.go` | `amaru repo tag` command |
| `cmd/repo_info.go` | `amaru repo info` command |
| `cmd/repo_test.go` | Integration tests for all repo subcommands |
| `internal/types/validate.go` | Item name validation |
| `internal/types/validate_test.go` | Name validation tests |
| `internal/scaffold/detect.go` | Registry root detection |
| `internal/scaffold/index.go` | Local index load/save (atomic writes) |
| `internal/scaffold/index_test.go` | Index I/O tests |
| `internal/scaffold/templates.go` | Content templates for skill/command/agent |
| `internal/scaffold/templates_test.go` | Template generation tests |

### Modified Files

| File | Changes |
|------|---------|
| `cmd/repo.go` | Register new subcommands in `init()` |
| `internal/scaffold/scaffold.go` | Add `"skillsets": {}` to generated index |
| `README.md` | Add Registry Management section, update repo init next steps |
| `CLAUDE.md` | Add registry management conventions, documentation sync rule |
| `AGENTS.md` | Add repo command data flows |
| `.amaru_registry/skills/amaru-usage/skill.md` | Add Registry Authoring section |
| `.amaru_registry/skills/amaru-usage/manifest.json` | Update files array if needed |

### Phase 5: CI/CD — GitHub Actions

#### Release Workflow — `.github/workflows/release.yml`

Triggered on version tags (`v*`). Uses GoReleaser to build cross-platform binaries and create GitHub Releases.

```yaml
on:
  push:
    tags: ["v*"]
```

- Build for: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- Create GitHub Release with changelogs from conventional commits
- Upload binaries as release assets
- Uses `goreleaser/goreleaser-action`

**New file:** `.goreleaser.yaml` — GoReleaser config for binary builds, archive naming, and changelog generation.

#### Conventional Commits Check — `.github/workflows/conventional-commits.yml`

Runs on every PR. Validates that all commit messages follow the conventional commits spec (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`, `release:`).

```yaml
on:
  pull_request:
    branches: [main]
```

- Uses a lightweight commit-message linter (e.g., `webiny/action-conventional-commits` or custom script)
- Blocks merge if any commit violates the convention
- Clear error message showing which commits fail and the expected format

#### Version Bump Check — `.github/workflows/version-check.yml`

Runs on PRs that modify Go source files. Ensures the version constant in `cmd/root.go` has been updated when code changes are made.

```yaml
on:
  pull_request:
    branches: [main]
    paths: ["*.go", "**/*.go", "go.mod", "go.sum"]
```

- Compares `Version` in `cmd/root.go` between the PR branch and the base branch
- If Go source files changed but version didn't, the check fails with a reminder
- Skip check for docs-only or CI-only changes

#### CI Workflow — `.github/workflows/ci.yml`

Runs on every push and PR. Standard Go CI.

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
```

- `go build ./...`
- `go vet ./...`
- `go test ./...`
- Optionally: `golangci-lint` for stricter checks

### New CI Files

| File | Purpose |
|------|---------|
| `.github/workflows/release.yml` | GoReleaser build + GitHub Release on version tags |
| `.github/workflows/conventional-commits.yml` | PR check for conventional commit messages |
| `.github/workflows/version-check.yml` | PR check for version bump in `cmd/root.go` |
| `.github/workflows/ci.yml` | Standard Go CI (build, vet, test) |
| `.goreleaser.yaml` | GoReleaser config for cross-platform builds |

## Implementation Order

1. **Shared infra** (validate, detect, index I/O, templates) — no command changes yet, all testable in isolation
2. **`repo add`** — most foundational command, validates the template + index write pipeline
3. **`repo list`** — quick to build, useful for verifying `repo add` worked
4. **`repo validate`** — codifies all consistency rules, useful for development itself
5. **`repo info`** — simple read-only command
6. **`repo remove`** — needs skillset dependency check
7. **`repo tag`** — most complex (git operations), benefits from validate being available
8. **Scaffold fix** — one-line addition of `"skillsets": {}`
9. **Documentation** — README, CLAUDE.md, AGENTS.md, amaru-usage skill
10. **CI/CD** — GitHub Actions workflows (release, conventional commits, version check, CI)
11. **Tests** — unit tests alongside each phase, integration tests at the end

## Sources & References

### Internal References

- Registry types: [internal/registry/registry.go:11-65](internal/registry/registry.go#L11-L65) — `RegistryIndex`, `RegistryEntry`, `SkillsetEntry`, `ItemManifest`
- Existing scaffold: [internal/scaffold/scaffold.go](internal/scaffold/scaffold.go) — `ScaffoldRepo`, templates
- Item types: [internal/types/types.go](internal/types/types.go) — `ItemType` enum
- Installer (for reference): [internal/installer/installer.go](internal/installer/installer.go) — file write patterns, hash computation
- Current repo command: [cmd/repo.go](cmd/repo.go) — parent command + `repo init`
- Self-hosted example: [amaru_registry.json](amaru_registry.json), [.amaru_registry/skills/amaru-usage/](amaru_registry/skills/amaru-usage/)
- Existing plan: [docs/plans/2026-03-03-fix-url-parsing-add-404-feat-skillsets-plan.md](docs/plans/2026-03-03-fix-url-parsing-add-404-feat-skillsets-plan.md) — skillset implementation context
