---
title: "feat: Add Comprehensive Unit Tests for All Packages"
type: feat
status: completed
date: 2026-03-02
---

# feat: Add Comprehensive Unit Tests for All Packages

## Overview

Add unit tests for every internal package in Amaru that currently lacks test coverage, and fill gaps in already-tested packages. Uses only the standard Go `testing` package with hand-written mock structs — no external test dependencies.

## Problem Statement / Motivation

Amaru has ~3,700 lines of Go code but only ~536 lines of tests across 5 files, covering 4 of 12 packages. Several critical paths (checker, ctxdocs, vcs) have zero tests. As the project grows, untested code is a liability — regressions will slip through, and refactoring becomes risky.

## Proposed Solution

Add test files to all untested packages and fill coverage gaps in existing ones. Where functions hardcode dependencies (e.g., `ctxdocs` calling `vcs.Detect()` directly), apply minimal refactoring to accept interfaces. Create hand-written mock structs for the three existing interfaces: `registry.Client`, `registry.Authenticator`, `vcs.Backend`.

## Technical Considerations

### Testability Refactoring Required

Three functions in `ctxdocs` hardcode `vcs.Detect()` and need a `Backend` parameter:

- `Init()` at `internal/ctxdocs/ctxdocs.go:71`
- `Sync()` at `internal/ctxdocs/ctxdocs.go:118`
- `Push()` at `internal/ctxdocs/ctxdocs.go:130`

**Proposed change:** Add a `Backend` field to `Config` struct, or change function signatures to accept `vcs.Backend`. Update callers in `cmd/context.go` to pass `vcs.Detect()`.

The `checker` package calls `installer.IsInstalled()` and `installer.ComputeHash()` directly — these are filesystem operations that work fine with `t.TempDir()`, so no refactoring needed there.

The `ui` package writes to `os.Stdout` via `fmt.Printf` — test the logic functions (`Box`, `Table`) by capturing stdout, or focus on testing the pure computation parts.

### Mock Structs Needed

```go
// mockRegistryClient implements registry.Client
type mockRegistryClient struct {
    index    *registry.RegistryIndex
    indexErr error
    versions map[string][]*semver.Version   // key: "itemType/name"
    files    map[string][]registry.File     // key: "itemType/name/version"
}

// mockVCSBackend implements vcs.Backend
type mockVCSBackend struct {
    name       string
    cloneErr   error
    pullErr    error
    hasChanges bool
    addErr     error
    pushErr    error
    calls      []string  // track method calls for assertions
}

// mockAuthenticator implements registry.Authenticator
type mockAuthenticator struct {
    token    string
    tokenErr error
    method   string
}
```

### Conventions (from existing tests)

- Standard `testing` package only — no testify, no gomock
- Table-driven tests: `tests` slice, `tt` variable, `t.Run(tt.name, ...)`
- `t.TempDir()` for filesystem operations
- `t.Fatalf` for setup errors, `t.Errorf` for assertions
- Tests in same package (not external `_test` package)
- Function naming: `Test<Function><Scenario>`

## Acceptance Criteria

### New Test Files

- [x] `internal/types/types_test.go` — `AllInstallableTypes`, `DirName`, `Singular`, `Plural` (including unknown type fallback)
- [x] `internal/checker/checker_test.go` — `Check` with mock registry client (updates detected, drift detected, up-to-date, missing client error, missing lock entries)
- [x] `internal/checker/cache_test.go` — `SaveCache`/`LoadCache` round-trip, expired cache returns nil, corrupt cache returns nil, missing file returns nil
- [x] `internal/scaffold/scaffold_test.go` — `ScaffoldRepo` with/without project, verify directory structure + file contents; `RootAgentsMD`, `ProjectAgentsMD`, `SparseProfile` output verification
- [x] `internal/hooks/hooks_test.go` — `InstallHook` creates new hook, appends to existing hook, skips if already installed; `PostCheckoutScript`/`PostCommitScript` return non-empty strings
- [x] `internal/ui/ui_test.go` — `Box` and `Table` output verification (capture stdout); empty input edge cases
- [x] `internal/ctxdocs/ctxdocs_test.go` — `ResolveConfig` (valid, missing context, missing registry, default path); `RepoURL` (github: prefix, plain URL); `SparsePaths`; `EnsureGitIgnore` (creates, appends, skips duplicate); `LocalPath` (nil context, custom path, default)
- [x] `internal/vcs/vcs_test.go` — `Detect` returns a valid Backend; `SaplingBackend.Name()` and `GitBackend.Name()` return correct strings

### Coverage Gaps in Existing Packages

- [x] `internal/manifest/manifest_test.go` — add tests for `ResolveRegistry`, `DepsForType`, `AllDeps`, `IsIgnored`
- [x] `internal/manifest/lock_test.go` — add tests for `EntriesForType`, `NewLockedEntry`
- [x] `internal/resolver/resolver_test.go` — add test for `LatestAvailable` (empty list, single version, multiple versions)
- [x] `internal/registry/github_test.go` — add tests for `NewAuthenticator` factory (github, token, none, unknown), `RegistryIndex.EntriesForType`
- [x] `internal/installer/installer_test.go` — add test for `DirForType` (all types + unknown)

### Quality Gates

- [x] All tests pass: `go test ./...`
- [x] No new dependencies added to `go.mod`
- [x] Testability refactoring limited to `ctxdocs` function signatures (minimal blast radius)
- [x] All callers of refactored `ctxdocs` functions updated

## Implementation Order

Ordered by dependency chain — each phase can reference mocks from earlier phases:

### Phase 1: Zero-Dependency Packages

Pure functions with no external dependencies. Start here to build momentum.

| # | File | Tests | Notes |
|---|------|-------|-------|
| 1 | `internal/types/types_test.go` | `TestAllInstallableTypes`, `TestDirName`, `TestSingular`, `TestPlural` | Table-driven, include unknown type edge case |
| 2 | `internal/installer/installer_test.go` | Add `TestDirForType` | Table-driven for all types |
| 3 | `internal/resolver/resolver_test.go` | Add `TestLatestAvailable` | Empty list, single, multiple versions |

### Phase 2: Manifest & Registry Gaps

Fill coverage holes in already-tested packages.

| # | File | Tests | Notes |
|---|------|-------|-------|
| 4 | `internal/manifest/manifest_test.go` | Add `TestResolveRegistry`, `TestDepsForType`, `TestAllDeps`, `TestIsIgnored` | Test with populated manifest struct |
| 5 | `internal/manifest/lock_test.go` | Add `TestEntriesForType`, `TestNewLockedEntry` | Verify all three item types |
| 6 | `internal/registry/github_test.go` | Add `TestNewAuthenticator`, `TestRegistryIndexEntriesForType` | Factory returns correct type for each auth method |

### Phase 3: Filesystem-Heavy Packages

These use `t.TempDir()` extensively for file/directory assertions.

| # | File | Tests | Notes |
|---|------|-------|-------|
| 7 | `internal/scaffold/scaffold_test.go` | `TestScaffoldRepo`, `TestScaffoldRepoWithProject`, `TestRootAgentsMD`, `TestProjectAgentsMD`, `TestSparseProfile` | Verify dirs, files, and content |
| 8 | `internal/hooks/hooks_test.go` | `TestInstallHookNew`, `TestInstallHookAppend`, `TestInstallHookSkipDuplicate`, `TestPostCheckoutScript`, `TestPostCommitScript` | Create fake .git/hooks in tmpdir |
| 9 | `internal/checker/cache_test.go` | `TestSaveAndLoadCache`, `TestLoadCacheExpired`, `TestLoadCacheMissing`, `TestLoadCacheCorrupt` | Uses `t.TempDir()`, may need time manipulation for expiry test |

### Phase 4: Mock-Dependent Packages

Require hand-written mock structs for interfaces.

| # | File | Tests | Notes |
|---|------|-------|-------|
| 10 | `internal/checker/checker_test.go` | `TestCheckDetectsUpdates`, `TestCheckDetectsDrift`, `TestCheckUpToDate`, `TestCheckMissingClient`, `TestCheckSkipsUnlockedDeps` | Create `mockRegistryClient`, set up manifest + lock + temp files |

### Phase 5: Testability Refactoring + Tests

Requires changing function signatures.

| # | File | Changes | Notes |
|---|------|---------|-------|
| 11 | `internal/ctxdocs/ctxdocs.go` | Refactor `Init`, `Sync`, `Push` to accept `vcs.Backend` | Minimal change: add param, remove `vcs.Detect()` call |
| 12 | `cmd/context.go` | Update callers to pass `vcs.Detect()` | One-line change per call site |
| 13 | `internal/ctxdocs/ctxdocs_test.go` | `TestResolveConfig`, `TestRepoURL`, `TestSparsePaths`, `TestEnsureGitIgnore`, `TestLocalPath`, `TestInit`, `TestSync`, `TestPush` | Pure functions + mock backend for Init/Sync/Push |

### Phase 6: Remaining Packages

| # | File | Tests | Notes |
|---|------|-------|-------|
| 14 | `internal/ui/ui_test.go` | `TestBox`, `TestBoxEmpty`, `TestTable`, `TestTableEmpty` | Capture stdout with `os.Pipe()` |
| 15 | `internal/vcs/vcs_test.go` | `TestDetect`, `TestSaplingBackendName`, `TestGitBackendName` | Only test Name() and Detect() return type — skip exec-dependent methods |

## Dependencies & Risks

| Risk | Mitigation |
|------|-----------|
| `ctxdocs` refactoring breaks callers | Minimal change (add one parameter), update all callers in same PR |
| `checker` cache TTL test flakiness | Write cache entry with past timestamp instead of manipulating system time |
| `ui` stdout capture complexity | Use `os.Pipe()` to redirect, restore after test. Skip color codes in assertions (use `color.NoColor = true`) |
| VCS backend tests require git/sl | Only test `Name()` and `Detect()`, skip exec-dependent methods |

## Success Metrics

- Every `internal/` package has at least one `_test.go` file
- `go test ./...` passes with zero failures
- Coverage for pure/testable functions approaches ~80%+ in each package
- No new external dependencies

## Sources & References

### Internal References

- Existing test patterns: [manifest_test.go](internal/manifest/manifest_test.go), [resolver_test.go](internal/resolver/resolver_test.go), [installer_test.go](internal/installer/installer_test.go)
- Interfaces to mock: [registry.go:65-75](internal/registry/registry.go#L65-L75), [vcs.go:11-24](internal/vcs/vcs.go#L11-L24), [auth.go:12-17](internal/registry/auth.go#L12-L17)
- Testability issue: [ctxdocs.go:71](internal/ctxdocs/ctxdocs.go#L71), [ctxdocs.go:118](internal/ctxdocs/ctxdocs.go#L118), [ctxdocs.go:130](internal/ctxdocs/ctxdocs.go#L130)
