# Amaru — Agent Navigation

## Architecture

```
amaru (CLI entry point)
├── cmd/           — Cobra commands: init, add, install, update, check, list, browse, ignore, context, repo
├── internal/
│   ├── manifest/  — amaru.json + amaru.lock read/write (Manifest, Lock, DependencySpec)
│   ├── registry/  — GitHub API client, RegistryIndex, SkillsetEntry, authentication
│   ├── installer/ — Write files to .claude/{skills,commands,agents}/, compute content hashes
│   ├── checker/   — Compare lock against registries: detect updates + local drift
│   ├── resolver/  — Semver constraint resolution (^, ~, exact) + version classification
│   ├── types/     — ItemType enum (skill, command, agent) + shared helpers
│   ├── ui/        — Terminal formatting: colors, tables, headers, check/warn/error marks
│   ├── ctxdocs/   — Sparse-checkout context docs from registry
│   ├── hooks/     — Install/manage git hooks for context sync
│   ├── scaffold/  — Registry repository scaffolding
│   └── vcs/       — VCS backend detection (Sapling vs Git)
└── main.go        — Entry point
```

## Key Data Flow

1. **Add**: `cmd/add.go` → `registry.Client.FetchIndex()` → `manifest.SetDep()` → `registry.Client.DownloadFiles()` → `installer.Install()` → `manifest.SaveLock()`
2. **Install**: `cmd/install.go` → for each dep: `resolver.Resolve()` → `DownloadFiles()` → `Install()` → `SaveLock()`
3. **Update**: `cmd/update.go` → `resolver.Resolve()` finds best compatible version → downloads + installs if newer
4. **Check**: `internal/checker/checker.go` → compares locked versions against registry, detects hash drift
5. **Skillsets**: `cmd/add.go:runAddSkillset()` → validates all members → installs each → records digest in `lock.Skillsets`

## Important Types

- `manifest.Manifest` — parsed amaru.json (registries, skills, commands, agents)
- `manifest.Lock` — parsed amaru.lock (locked entries + skillsets)
- `manifest.DependencySpec` — version constraint + optional registry + optional group
- `registry.RegistryIndex` — parsed registry.json from remote (entries + skillsets)
- `registry.Client` — interface for FetchIndex, ListVersions, DownloadFiles
- `types.ItemType` — "skill" | "command" | "agent"
