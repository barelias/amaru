# Amaru — Development Guide

## Overview

Amaru is a Go CLI tool that manages skills, commands, and agents for Claude Code. It connects projects to centralized GitHub-hosted registries via a manifest file (`amaru.json`) and lock file (`amaru.lock`).

## Build & Test

```bash
go build ./...          # Build all packages
go test ./...           # Run all tests
go vet ./...            # Static analysis
```

No special setup needed — standard Go toolchain.

## Project Structure

```
cmd/              # Cobra CLI commands (one file per command)
internal/
  checker/        # Compares lock vs registry (update detection, drift detection)
  ctxdocs/        # Context documentation sync (sparse checkout)
  hooks/          # Git hook management (post-checkout, post-commit)
  installer/      # File installation + hash computation
  manifest/       # amaru.json and amaru.lock parsing/saving
  registry/       # GitHub API client, registry types, authentication
  resolver/       # Semver constraint resolution
  scaffold/       # Registry repo scaffolding
  types/          # Shared type definitions (ItemType)
  ui/             # Terminal output formatting (colors, tables)
  vcs/            # VCS backend detection (Git vs Sapling)
```

## Key Conventions

- **Item types**: `skill`, `command`, `agent` — defined in `internal/types/types.go`
- **Skillsets**: Registry-defined groups that expand to individual items on install. Not an item type — they live in the registry index and lock file only.
- **Version constraints**: Follow npm conventions (`^`, `~`, exact). `"latest"` is a special non-semver constraint for unversioned items.
- **Registry URLs**: Canonical form is `github:org/repo`. Multiple formats accepted (SSH, HTTPS, bare domain) and normalized on init.
- **Registry layout**: `amaru_registry.json` at repo root, all content under `.amaru_registry/` (skills/, commands/, agents/, context/). The `amaru_version` field in the index enables future structure migrations.
- **Self-hosted registries**: Any repo can be its own registry — this repo ships an `amaru-usage` skill via `amaru_registry.json` + `.amaru_registry/skills/amaru-usage/`.
- **DependencySpec**: Marshals as shorthand string when only version is set, full object when registry or group is present.
- **Lock entries**: Store resolved version, registry alias, content hash, and timestamp.

## Testing Patterns

- Tests use `t.TempDir()` for filesystem isolation
- Registry client is mocked via `mockRegistryClient` implementing `registry.Client`
- Table-driven tests preferred (see `github_test.go`, `manifest_test.go`)
- No external service calls in tests — everything is mocked or uses local fixtures

## Code Style

- Standard Go formatting (`gofmt`)
- User-facing messages in Portuguese (project convention)
- Internal code, comments, and docs in English
- Error wrapping with `fmt.Errorf("context: %w", err)` pattern
- Cobra commands: one file per command in `cmd/`, registered via `init()`
