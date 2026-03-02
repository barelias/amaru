---
title: "Agents, Context Sync, and Sapling VCS Support for Amaru"
date: 2026-03-02
status: done
category: feature-implementations
tags:
  - agents
  - context-sync
  - sapling
  - vcs
  - sparse-checkout
  - compound-engineering
  - go-cli
  - registry
components:
  - internal/types
  - internal/vcs
  - internal/ctxdocs
  - internal/scaffold
  - internal/hooks
  - cmd/context
  - cmd/repo
  - internal/manifest
  - internal/registry
  - internal/installer
  - internal/checker
severity: n/a
resolution_time: single-session
---

# Agents, Context Sync, and Sapling VCS Support

## Problem

Amaru had hardcoded support for only two installable types (skills, commands) with copy-pasted dual loops throughout the codebase. There was no support for:

1. **Agents** as a third installable type (Claude Code agent definitions)
2. **Context documentation** following the compound engineering docs format (brainstorms/, plans/, solutions/)
3. **Sapling VCS** for efficient sparse checkout of large registries
4. **Registry scaffolding** to create centralized repos
5. **Auto-sync hooks** for consuming projects

## Root Cause

The original architecture embedded type-specific logic directly in each command and data structure rather than using a generic type system. Adding a new installable type required changes to 15+ files with copy-pasted patterns.

## Solution

### Phase 1: Generic Type System + Agents

Created `internal/types/types.go` as the foundation:

```go
type ItemType string

const (
    Skill   ItemType = "skill"
    Command ItemType = "command"
    Agent   ItemType = "agent"
)

func AllInstallableTypes() []ItemType {
    return []ItemType{Skill, Command, Agent}
}

func (t ItemType) DirName() string {
    switch t {
    case Skill:   return "skills"
    case Command: return "commands"
    case Agent:   return "agents"
    default:      return string(t) + "s"
    }
}
```

Added generic accessors to manifest, lock, and registry:

```go
// Manifest
func (m *Manifest) DepsForType(t types.ItemType) map[string]DependencySpec
func (m *Manifest) SetDep(t types.ItemType, name string, spec DependencySpec)
func (m *Manifest) AllDeps(fn func(t types.ItemType, name string, spec DependencySpec) error) error
func (m *Manifest) HasDep(name string) bool

// Lock
func (l *Lock) EntriesForType(t types.ItemType) map[string]LockedEntry

// RegistryIndex
func (idx *RegistryIndex) EntriesForType(t types.ItemType) map[string]RegistryEntry
```

Replaced all dual-loop patterns in cmd/ files with:

```go
for _, itemType := range types.AllInstallableTypes() {
    deps := m.DepsForType(itemType)
    lockEntries := lock.EntriesForType(itemType)
    for name, spec := range deps {
        // ... generic processing
    }
}
```

### Phase 2: VCS Abstraction + Context Documentation

Created `internal/vcs/vcs.go` with a `Backend` interface:

```go
type Backend interface {
    Name() string
    SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error
    Pull(ctx context.Context, dir string) error
    HasChanges(ctx context.Context, dir string) bool
    Add(ctx context.Context, dir string, paths []string) error
    CommitAndPush(ctx context.Context, dir, message string) error
}

func Detect() Backend {
    if _, err := exec.LookPath("sl"); err == nil {
        return &SaplingBackend{}
    }
    return &GitBackend{}
}
```

**Sapling** uses `sl clone --enable-profile` with sparse profiles committed to the repo.
**Git** falls back to `git clone --filter=blob:none --no-checkout` + `git sparse-checkout set`.

Created `internal/ctxdocs/ctxdocs.go` for context lifecycle:

- `Init`: Sparse clone into `.claude/.amaru-context/`, symlink to `docs/context`
- `Sync`: Pull latest from centralized repo
- `Push`: Stage context changes, commit, and push
- `EnsureGitIgnore`: Add clone dir to `.gitignore`

Manifest gains a `context` section:

```json
{
  "context": {
    "registry": "origin",
    "project": "my-app",
    "path": "docs/context"
  }
}
```

### Phase 3: Registry Scaffolding

Created `internal/scaffold/scaffold.go` and `cmd/repo.go`:

```bash
amaru repo init /path/to/registry --project my-app
```

Generates:

```
registry/
├── registry.json
├── AGENTS.md              # Root navigation + registry structure
├── .sparse-profiles/
│   └── my-app             # Sapling sparse profile
├── skills/
├── commands/
├── agents/
└── context/
    └── my-app/
        ├── AGENTS.md      # Per-project navigation + compound docs workflow
        ├── brainstorms/
        ├── plans/
        └── solutions/
```

Both AGENTS.md files are generated with templates explaining navigation, structure, and workflow.

### Phase 4: Git Hooks

Created `internal/hooks/hooks.go` with idempotent hook installation:

- **post-checkout**: Auto-runs `amaru context sync` after branch switches
- **post-commit**: Detects context file changes via `git diff-tree`, auto-pushes via `amaru context push`

Hooks are installed during `amaru context init` and append to existing hooks (detecting "amaru:" signature to avoid duplication).

## Files Changed

### New Files (7)
| File | Purpose |
|------|---------|
| `internal/types/types.go` | ItemType abstraction with generic accessors |
| `internal/vcs/vcs.go` | Sapling/Git VCS backend interface |
| `internal/ctxdocs/ctxdocs.go` | Context documentation sync lifecycle |
| `internal/scaffold/scaffold.go` | Registry repo scaffolding + templates |
| `internal/hooks/hooks.go` | Git hook scripts + idempotent installer |
| `cmd/context.go` | `amaru context init\|sync\|push\|path` commands |
| `cmd/repo.go` | `amaru repo init` command |

### Modified Files (12)
| File | Change |
|------|--------|
| `internal/manifest/manifest.go` | Added Agents, ContextConfig, generic helpers |
| `internal/manifest/lock.go` | Added Agents, EntriesForType |
| `internal/registry/registry.go` | Added Agents to RegistryIndex, EntriesForType |
| `internal/registry/github.go` | Uses ItemType.DirName() instead of hardcoded paths |
| `internal/installer/installer.go` | Added AgentsDir, DirForType |
| `internal/checker/checker.go` | Generic type iteration replacing dual loops |
| `cmd/install.go` | Generic iteration |
| `cmd/add.go` | Rewritten with `--type` flag |
| `cmd/update.go` | Generic iteration |
| `cmd/list.go` | Rewritten with generic iteration |
| `cmd/browse.go` | Rewritten with generic iteration |
| `cmd/ignore.go` | Uses HasDep instead of checking maps individually |

## Before / After

### Before: Adding a new type required ~15 file changes

```go
// cmd/install.go (before) — duplicated for each type
for name, spec := range m.Skills {
    // install skill...
}
for name, spec := range m.Commands {
    // install command... (copy-pasted logic)
}
```

### After: Adding a new type requires 1 constant + 3 map fields

```go
// internal/types/types.go — add one constant
const NewType ItemType = "newtype"

// Then add map fields to Manifest, Lock, RegistryIndex
// All commands auto-iterate via AllInstallableTypes()
```

## Verification

1. `go build ./...` — compiles with no errors
2. `go test ./...` — all existing tests pass (installer, manifest, registry, resolver)
3. `amaru repo init /tmp/test --project my-app` — creates correct directory structure with templates
4. VCS detection prefers Sapling when `sl` is on PATH, falls back to Git

## Prevention / Best Practices

### Adding Future Installable Types

1. Add constant to `internal/types/types.go` with `DirName()` case
2. Add `AllInstallableTypes()` entry
3. Add map field to `Manifest`, `Lock`, and `RegistryIndex`
4. Add cases to `DepsForType`, `EntriesForType`, `SetDep` methods
5. Add install directory constant to `internal/installer/installer.go`
6. All commands auto-discover the new type — no cmd/ changes needed

### VCS Backend Extension

The `Backend` interface allows adding new VCS systems (e.g., Jujutsu) by implementing the 6 interface methods without changing any calling code.

### Hook Safety

- Hooks use `2>/dev/null || true` to fail silently — never block user workflow
- Idempotent installation via "amaru:" signature detection
- Post-commit hook only triggers on context file changes (not all commits)

## Related

- Sapling VCS documentation: https://sapling-scm.com/docs/sparse-profiles
- Git sparse-checkout: https://git-scm.com/docs/git-sparse-checkout
- Compound engineering docs format: brainstorms/ -> plans/ -> solutions/ workflow
