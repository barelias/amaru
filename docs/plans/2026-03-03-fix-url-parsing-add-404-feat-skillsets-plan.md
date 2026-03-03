---
title: "fix: SSH URL parsing, add 404 on unversioned skills, feat: skillsets"
type: fix/feat
status: active
date: 2026-03-03
---

# Fix URL Parsing, Fix Add 404, Add Skillsets

## Overview

Three changes to amaru:

1. **Bug fix**: `amaru init` accepts SSH URLs (`git@github.com:org/repo.git`) but `parseGitHubURL()` rejects them at runtime, breaking `browse`/`add`/`install`
2. **Bug fix**: `amaru add` fails with 404 when registry entries have no version tags (`entry.Latest` is empty), constructing an invalid git ref like `skill/name/`
3. **Feature**: Skillsets — registry-defined groups of skills/commands/agents that expand to individual items on install (VS Code Extension Pack pattern)

## Problem Statement

### Bug 1: SSH URL Format Rejected

`amaru init` accepts any URL string without validation. When the user enters `git@github.com:Visio-ai/ai_registry.git`, it's saved verbatim to `amaru.json`. Every subsequent command fails because `parseGitHubURL()` (`internal/registry/github.go:37-56`) only handles two formats:
- `github:org/repo`
- `https://github.com/org/repo[.git]`

The user tried multiple variants (`github.com:org/repo.git`, `github.com/org/repo.git`, etc.) — all rejected.

### Bug 2: Add Fails on Unversioned Registry Items

The Visio AI registry has skills without version tags. `registry.json` entries have empty `latest` fields. When `amaru add amaru-usage` runs:

1. `entry.Latest` is `""` (empty string)
2. `DownloadFiles()` constructs tag: `fmt.Sprintf("%s/%s/%s", itemType, name, version)` → `"skill/amaru-usage/"`
3. GitHub API returns 404: "No commit found for the ref skill/amaru-usage/"

The underlying `downloadDirectory()` already supports empty refs (downloads from default branch), but `DownloadFiles()` always constructs a tag ref.

### Feature: Skillsets

Registry maintainers want to offer curated bundles (e.g., "ory-full-stack" = 5 skills + 2 commands). Currently there's no grouping mechanism.

## Proposed Solution

### Bug 1 Fix: Normalize URLs

Two-part fix:

**A. Expand `parseGitHubURL()` to accept more formats.** Support:
- `git@github.com:org/repo[.git]` (SSH colon syntax)
- `ssh://git@github.com/org/repo[.git]` (SSH URL syntax)
- `http://github.com/org/repo[.git]` (auto-upgrade to HTTPS)
- `github.com/org/repo[.git]` (bare domain — user tried this in the bug report)

Reject with clear error:
- Non-GitHub SSH hosts (`git@gitlab.com:...`)
- URLs with extra path segments (`https://github.com/org/repo/tree/main`)
- Malformed URLs

Clean up edge cases:
- Trim trailing slashes from owner/repo
- Case-insensitive prefix matching for `git@GitHub.com:` etc.

```go
// internal/registry/github.go - parseGitHubURL()

// Normalize: lowercase the prefix for case-insensitive matching
lower := strings.ToLower(url)

// SSH colon syntax: git@github.com:org/repo[.git]
if strings.HasPrefix(lower, "git@github.com:") {
    trimmed := url[len("git@github.com:"):]
    trimmed = strings.TrimSuffix(trimmed, ".git")
    trimmed = strings.TrimRight(trimmed, "/")
    parts := strings.SplitN(trimmed, "/", 3) // limit 3 to catch extra segments
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
        return "", "", fmt.Errorf("invalid github SSH URL: %s (expected git@github.com:org/repo)", url)
    }
    return parts[0], parts[1], nil
}

// SSH URL syntax: ssh://git@github.com/org/repo[.git]
if strings.HasPrefix(lower, "ssh://git@github.com/") {
    trimmed := url[len("ssh://git@github.com/"):]
    trimmed = strings.TrimSuffix(trimmed, ".git")
    trimmed = strings.TrimRight(trimmed, "/")
    parts := strings.SplitN(trimmed, "/", 3)
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
        return "", "", fmt.Errorf("invalid github SSH URL: %s", url)
    }
    return parts[0], parts[1], nil
}

// HTTP: auto-upgrade to HTTPS
if strings.HasPrefix(lower, "http://github.com/") {
    url = "https://github.com/" + url[len("http://github.com/"):]
    // fall through to HTTPS handler below
}

// Bare domain: github.com/org/repo
if strings.HasPrefix(lower, "github.com/") {
    url = "https://" + url
    // fall through to HTTPS handler below
}
```

**B. Add `NormalizeURL()` exported function** — converts any accepted format to canonical `github:org/repo`.

```go
func NormalizeURL(url string) (string, error) {
    owner, repo, err := parseGitHubURL(url)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("github:%s/%s", owner, repo), nil
}
```

**C. Normalize in `init` BEFORE alias suggestion** — the current flow is URL → alias → auth → save. Normalization must happen before `suggestAlias()` to avoid garbled suggestions.

```go
// cmd/init_cmd.go - in runInit()
rawURL, _ := reader.ReadString('\n')
rawURL = strings.TrimSpace(rawURL)
if rawURL == "" {
    return fmt.Errorf("registry URL is required")
}

// Normalize URL to canonical format before anything else
url, err := registry.NormalizeURL(rawURL)
if err != nil {
    fmt.Printf("Error: %v\n", err)
    continue // Let user retry
}
if url != rawURL {
    fmt.Printf("  → normalized to: %s\n", url)
}

// Now suggest alias from the normalized URL
suggested := suggestAlias(url)
```

**D. Validate extra path segments** — `SplitN(trimmed, "/", 3)` with limit 3: if we get 3 parts, the URL has extra segments (like `/tree/main`) and should be rejected.

### Bug 2 Fix: Support Unversioned Downloads

**A. `DownloadFiles()` — skip tag when version is empty:**

```go
// internal/registry/github.go
func (c *GitHubClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error) {
    ref := ""
    if version != "" {
        ref = fmt.Sprintf("%s/%s/%s", itemType, name, version)
    }
    dirPath := types.ItemType(itemType).DirName() + "/" + name
    return c.downloadDirectory(ctx, dirPath, ref, "")
}
```

**B. `cmd/add.go` — handle empty `entry.Latest`:**

```go
version := entry.Latest
spec := manifest.DependencySpec{}
if version != "" {
    spec.Version = "^" + version
} else {
    spec.Version = "latest"
}
// ... later:
files, err := client.DownloadFiles(ctx, string(itemType), name, version)
// version is "" here for unversioned items → downloads from default branch
```

**C. `cmd/install.go` — `resolveVersion()` must short-circuit for `"latest"`:**

```go
func resolveVersion(ctx context.Context, client registry.Client, itemType, name, constraint string) (string, error) {
    if constraint == "latest" {
        return "", nil // empty version → download from default branch
    }
    // ... existing semver resolution
}
```

And in `installItem()`, handle the empty resolved version for display:

```go
displayVersion := resolved
if displayVersion == "" {
    displayVersion = "latest"
}
ui.Check("%s@%s (%s)", name, displayVersion, regAlias)
```

**D. `cmd/update.go` — skip semver comparison for `"latest"` items:**

When constraint is `"latest"`, re-download from the default branch unconditionally. Compare hash to see if content changed.

```go
// In updateItem(), before semver version resolution:
if spec.Version == "latest" {
    files, err := client.DownloadFiles(ctx, itemType, name, "")
    if err != nil { return err }
    hash, err := installer.Install(".", itemType, name, files)
    if err != nil { return err }
    if hash != locked.Hash {
        lockEntries[name] = manifest.NewLockedEntry("latest", regAlias, hash)
        ui.Check("%s@latest (%s) — updated", name, regAlias)
    } else {
        ui.Check("%s@latest (%s) — up to date", name, regAlias)
    }
    return nil
}
```

**E. `internal/checker/checker.go` — skip version check for `"latest"` items:**

```go
// In checkItem(), before semver operations:
if constraint == "latest" {
    // Only do hash-based drift detection, skip version comparison
    // No way to detect remote updates without downloading (skip for check)
    return nil // or just check local drift
}
```

**F. `cmd/list.go` — handle `"latest"` in display:**

When `locked.Version` is `""` or `"latest"`, display `"latest"` instead of trying to parse semver.

**G. Lock file: store `"latest"` as version string** — `NewLockedEntry("latest", regAlias, hash)` when version is empty. Every code path that reads `LockedEntry.Version` via `semver.NewVersion()` must guard with a `if version == "latest"` check.

**H. `add.go` display fix** — when `entry.Latest` is empty, `ui.Check("Added %s %s@%s ...")` shows `name@` with nothing. Fix: display `"latest"` instead.

### Feature: Skillsets (VS Code Extension Pack Pattern)

Based on research of npm, apt metapackages, Homebrew bundles, and VS Code extension packs — the **VS Code Extension Pack model** is the best fit:

- **Registry defines groups** in `registry.json` with a list of member items
- **`amaru add <name> --type=skillset`** expands members into individual `amaru.json` entries
- **Soft provenance** via optional `group` field in `DependencySpec` (informational only)
- **Individual removal freely allowed** — no cascade deletion (avoids apt's notorious problem)
- **Lock file tracks only individual items** — no group entries
- **Nested skillsets explicitly rejected** — `GroupItem.Type` validated to exclude `"skillset"`

#### Registry-Side: `registry.json` Additions

```json
{
  "skillsets": {
    "ory-full-stack": {
      "description": "Complete Ory integration for auth, permissions, and API gateway",
      "tags": ["ory", "auth"],
      "items": [
        { "type": "skill", "name": "ory-keto-seed-tuples" },
        { "type": "skill", "name": "ory-react-auth-ui-integration" },
        { "type": "skill", "name": "ory-oathkeeper-add-api" },
        { "type": "command", "name": "ory-setup" }
      ]
    }
  }
}
```

Note: No `latest` version field on skillsets — they are not versioned artifacts. They are just named lists of items.

#### Client-Side: Expansion on Install

`amaru add ory-full-stack --type=skillset` does:

1. Fetch registry index
2. Look up skillset by name in `idx.Skillsets`
3. Validate all member types are `skill`, `command`, or `agent` (reject `"skillset"`)
4. For each item in the skillset:
   a. Look up its `RegistryEntry` in the appropriate type map to get `latest` version
   b. Skip with warning if already in manifest
   c. Add as individual entry to `amaru.json` with `"group"` provenance
   d. Download and install
   e. Update lock with individual entry
5. If any member is not found in registry → **abort** with clear error listing missing members (fail-fast)

```json
// amaru.json after installing skillset
{
  "skills": {
    "ory-keto-seed-tuples": { "version": "latest", "group": "ory-full-stack" },
    "ory-react-auth-ui-integration": { "version": "latest", "group": "ory-full-stack" }
  },
  "commands": {
    "ory-setup": { "version": "latest", "group": "ory-full-stack" }
  }
}
```

#### `DependencySpec` Changes

Add `Group` field with `omitempty`:

```go
type DependencySpec struct {
    Version  string `json:"version"`
    Registry string `json:"registry,omitempty"`
    Group    string `json:"group,omitempty"`
}
```

**Critical: Update `MarshalJSON`** — the shorthand condition must check both `Registry` and `Group`:

```go
func (d DependencySpec) MarshalJSON() ([]byte, error) {
    if d.Registry == "" && d.Group == "" {
        return json.Marshal(d.Version)
    }
    type alias DependencySpec
    return json.Marshal(alias(d))
}
```

Without this change, a spec `{Version: "latest", Group: "ory-full-stack"}` would marshal as `"latest"`, silently losing the group provenance.

#### Browse Display

New "Skillsets:" section after Agents in `amaru browse` output:

```
[visio_ai_registry] github:Visio-ai/ai_registry
  Skills:
      ory-keto-seed-tuples    Create Keto relation tuples...
  Skillsets:
      ory-full-stack          4 items    Complete Ory integration...
```

#### Auto-Detection in `add` (Nice-to-have)

When `amaru add <name>` (default `--type=skill`) fails to find the item, check if it exists as a skillset. If so, suggest:
```
Error: skill "ory-full-stack" not found. Did you mean: amaru add ory-full-stack --type=skillset?
```

## Technical Considerations

### Files to Modify

**Bug 1 (URL normalization):**
- `internal/registry/github.go` — expand `parseGitHubURL()` + add `NormalizeURL()`
- `internal/registry/github_test.go` — test all URL formats (SSH, ssh://, http://, bare domain, extra paths, trailing slashes)
- `cmd/init_cmd.go` — normalize URL before `suggestAlias()`, show normalized form

**Bug 2 (unversioned downloads):**
- `internal/registry/github.go` — `DownloadFiles()`: skip tag when version is empty
- `cmd/add.go` — handle empty `entry.Latest`, use `"latest"` marker, fix display
- `internal/resolver/resolver.go` — handle `"latest"` constraint (skip semver, return nil)
- `cmd/install.go` — `resolveVersion()`: short-circuit for `"latest"`; `installItem()`: handle display
- `cmd/update.go` — `updateItem()`: re-download from default branch for `"latest"` items
- `internal/checker/checker.go` — `checkItem()`: skip semver for `"latest"`, only do hash drift
- `cmd/list.go` — handle `"latest"` version display without semver parsing

**Feature (skillsets):**
- `internal/registry/registry.go` — add `GroupEntry`, `GroupItem` types, extend `RegistryIndex` with `Skillsets` map
- `internal/manifest/manifest.go` — add `Group` field to `DependencySpec`, update `MarshalJSON` condition
- `cmd/add.go` — skillset expansion logic: detect `--type=skillset`, fetch members, expand, install
- `cmd/browse.go` — display skillsets section with member count and description
- `cmd/list.go` — show group provenance `(via <group-name>)` in list output

### Architecture Impacts

- No breaking changes to existing `amaru.json` files — `Group` field is `omitempty`, `"latest"` is a new valid version value
- `"latest"` version constraint bypasses semver resolution across all commands
- Skillsets are not an installable type (no `.claude/skillsets/` directory) — they expand to existing types
- Nested skillsets are explicitly unsupported and validated against

### Error Handling

- SSH URL with non-GitHub host (e.g., `git@gitlab.com:...`) → clear error: "unsupported URL format: only GitHub URLs are supported"
- URLs with extra path segments → "invalid github URL: unexpected path segments after org/repo"
- Skillset member not found in registry → abort with: "skillset 'X': member skill 'Y' not found in registry"
- Skillset member already in manifest → skip with warning: "skill 'Y' already in manifest, skipping"
- Nested skillset → "skillset 'X': nested skillsets are not supported (member 'Y' has type 'skillset')"

## System-Wide Impact

- **`"latest"` propagation**: Introducing a non-semver version string affects `add`, `install`, `update`, `check`, and `list`. Every code path that calls `semver.NewVersion()` or `semver.NewConstraint()` on user-provided version strings must guard against `"latest"`. Missing even one path causes a runtime panic.
- **Lock file consistency**: Lock entries for `"latest"` items store `Version: "latest"` (not empty string) so lock readers always have a parseable value.
- **URL normalization at init only**: Existing `amaru.json` files with raw SSH URLs will start working (because `parseGitHubURL` now handles them) but remain non-canonical. No migration needed — the URLs just work.

## Acceptance Criteria

### Bug 1: URL Parsing
- [ ] `amaru init` with `git@github.com:org/repo.git` normalizes to `github:org/repo` in `amaru.json`
- [ ] `amaru init` with `ssh://git@github.com/org/repo.git` normalizes to `github:org/repo`
- [ ] `amaru init` with `http://github.com/org/repo` normalizes to `github:org/repo`
- [ ] `amaru init` with `github.com/org/repo` normalizes to `github:org/repo`
- [ ] `amaru init` with `https://github.com/org/repo.git` normalizes to `github:org/repo`
- [ ] `amaru init` with `github:org/repo` keeps it as-is (already canonical)
- [ ] URLs with extra path segments (e.g., `.../tree/main`) are rejected with clear error
- [ ] Non-GitHub SSH hosts are rejected with clear error
- [ ] Alias suggestion works correctly for all accepted URL formats
- [ ] User sees `→ normalized to: github:org/repo` feedback when URL is transformed
- [ ] Tests cover all URL format variants including edge cases (trailing slash, case sensitivity)

### Bug 2: Unversioned Downloads
- [ ] `amaru add <name>` works when `entry.Latest` is empty in registry
- [ ] Files are downloaded from the default branch (no ref parameter in API call)
- [ ] `amaru.json` records `"latest"` as the version constraint
- [ ] `amaru.lock` stores `Version: "latest"` for unversioned items
- [ ] `amaru install` handles `"latest"` constraint (skips semver resolution, downloads from default branch)
- [ ] `amaru update` re-downloads from default branch for `"latest"` items, updates lock if hash changed
- [ ] `amaru check` skips version comparison for `"latest"` items, only checks local hash drift
- [ ] `amaru list` displays `"latest"` cleanly without semver parse errors
- [ ] `amaru add` output shows `name@latest` instead of `name@` for unversioned items
- [ ] Tests cover `DownloadFiles()` with empty version, `resolveVersion()` with `"latest"` constraint

### Feature: Skillsets
- [ ] `registry.json` supports a `"skillsets"` section with `GroupEntry` items
- [ ] `amaru browse` displays skillsets section with member count and description
- [ ] `amaru add <name> --type=skillset` expands to individual items in `amaru.json`
- [ ] Each expanded item has `"group": "<skillset-name>"` provenance field
- [ ] `DependencySpec.MarshalJSON` preserves `Group` field (not lost to shorthand marshaling)
- [ ] Individual items can be freely removed (no cascade)
- [ ] Already-installed items are skipped with a warning during skillset install
- [ ] Missing members cause abort with clear error listing all missing items
- [ ] Nested skillsets (`type: "skillset"` in items) are rejected with clear error
- [ ] `amaru list` shows `(via <group-name>)` provenance when present
- [ ] `amaru add <name>` (without `--type`) suggests `--type=skillset` when name exists as skillset

## Success Metrics

- Users can use any common GitHub URL format in `amaru init` without errors
- `amaru add` works against registries without version tags
- Skillsets reduce the number of `amaru add` commands for common bundles

## Dependencies & Risks

- **SSH URL detection**: Supports `git@github.com:` and `ssh://git@github.com/` only. Other Git hosts out of scope.
- **Unversioned items**: Drift detection for `"latest"` items is local-only (hash drift). No way to detect remote changes without downloading. `amaru update` handles this by re-downloading.
- **Skillset versioning**: Skillsets have no version. When a skillset definition changes (adds/removes members), users must re-run `amaru add <skillset> --type=skillset` to pick up new members.
- **No `remove` command**: The acceptance criterion "individual items can be freely removed" relies on manually editing `amaru.json`. A `remove` command is out of scope for this plan.

## Sources & References

### Internal References
- URL parsing: [internal/registry/github.go:37-56](internal/registry/github.go#L37-L56)
- DownloadFiles: [internal/registry/github.go:152-160](internal/registry/github.go#L152-L160)
- Init command: [cmd/init_cmd.go:42-84](cmd/init_cmd.go#L42-L84)
- Add command: [cmd/add.go:38-133](cmd/add.go#L38-L133)
- Install command: [cmd/install.go:70-128](cmd/install.go#L70-L128)
- Update command: [cmd/update.go](cmd/update.go)
- Check command: [internal/checker/checker.go](internal/checker/checker.go)
- Registry types: [internal/registry/registry.go](internal/registry/registry.go)
- Manifest types: [internal/manifest/manifest.go](internal/manifest/manifest.go)

### External References (Skillset Design)
- VS Code Extension Pack model: [VS Code API - extensionPack](https://code.visualstudio.com/api/references/extension-manifest)
- apt metapackage cascade removal problem: [DEP-6 (Rejected)](https://dep-team.pages.debian.net/deps/dep6/)
- npm workspaces: [npm Workspaces Documentation](https://docs.npmjs.com/cli/v11/using-npm/workspaces/)
