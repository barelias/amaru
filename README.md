# 🐍 amaru

**Skills & commands manager for Claude Code.**

amaru connects your projects to centralized skill/command registries hosted on GitHub. It tracks versions, detects local drift, warns about updates, and keeps your whole team in sync — all through a simple manifest file.

> *The name **amaru** comes from the mythical Andean serpent — a symbol of transformation and connection between worlds. The tool connects centralized knowledge (registries) with local context (projects).*

---

## Install

```bash
go install github.com/barelias/amaru@latest
```

Or build from source:

```bash
git clone https://github.com/barelias/amaru.git
cd amaru
go build -o amaru .
```

## Quick Start

```bash
# 1. Initialize a manifest in your project
amaru init

# 2. Discover what's available
amaru browse

# 3. Add skills and commands
amaru add research
amaru add dev/bootstrap --command

# 4. Install everything
amaru install

# 5. Check for updates later
amaru check
```

## How It Works

amaru manages two files at the root of your project:

| File | Purpose | Committed? |
|---|---|---|
| `amaru.json` | Manifest — declares registries, skills, and commands with version ranges | Yes |
| `amaru.lock` | Lock — resolved versions, hashes, timestamps for reproducibility | Yes |

Skills are installed to `.claude/skills/` and commands to `.claude/commands/`, matching the Claude Code convention.

### Manifest (`amaru.json`)

```jsonc
{
  "version": "1.0.0",
  "registries": {
    "main": {
      "url": "github:acme-org/acme-skills",
      "auth": "github"
    },
    "platform": {
      "url": "github:acme-org/platform-skills",
      "auth": "github"
    }
  },
  "skills": {
    "research": { "version": "^1.0.0", "registry": "main" },
    "plan": { "version": "^1.0.0", "registry": "main" },
    "deploycheck": { "version": "^1.0.0", "registry": "platform" }
  },
  "commands": {
    "dev/bootstrap": { "version": "^2.0.0", "registry": "main" }
  }
}
```

**Shorthand** — when you have a single registry, skip the `registry` field:

```jsonc
{
  "version": "1.0.0",
  "registries": {
    "main": { "url": "github:acme-org/acme-skills", "auth": "github" }
  },
  "skills": {
    "research": "^1.0.0",
    "plan": "^1.0.0"
  }
}
```

**Version ranges** follow npm conventions: `^` (minor + patch), `~` (patch only), exact (`1.2.3`).

## Commands

### `amaru init`

Interactive setup — creates `amaru.json` with your registries configured.

```
$ amaru init
Registry URL (ex: github:org/skills-repo): github:acme-org/acme-skills
Registry alias [acme]: acme
Auth method (github/token/none) [github]: github
Adicionar outro registry? (y/N): N

amaru.json criado. Rode `amaru browse` para ver skills disponíveis.
```

### `amaru install [--force]`

Resolves versions, downloads files from registries, writes to `.claude/`, and generates the lock file.

```
$ amaru install
Authenticating registries...
  ✓ main (github:acme-org/acme-skills) — via gh CLI

Installing skills...
  ✓ research@1.0.3 (main)
  ✓ plan@1.0.1 (main)

Installing commands...
  ✓ dev/bootstrap@2.0.0 (main)

Lock file updated.
```

Idempotent — won't reinstall if the lock already matches. Use `--force` to override.

### `amaru check [--quiet]`

Compares your lock against the registries. Reports available updates and local drift.

```
$ amaru check
⚠ Atualizações disponíveis:
  research: 1.0.3 → 1.1.0 (minor) [main]
  compound: 1.1.0 → 2.0.0 (MAJOR — breaking) [main]

⚠ Drift detectado (editado localmente):
  plan: hash local b2c3d4 ≠ central a1b2c3 (v1.0.1) [main]

✓ 7 skills/commands atualizados
```

Use `--quiet` for the compact box format (designed for session-start hooks):

```
╭──────────────────────────────────────────────────╮
│ 🐍 amaru: 2 atualização(ões) disponível(is)      │
│   research 1.0.3 → 1.1.0 [main]                 │
│   compound 1.1.0 → 2.0.0 (MAJOR) [main]         │
│                                                  │
│   Rode `amaru update` para atualizar             │
╰──────────────────────────────────────────────────╯
```

Results are cached for 4 hours so it doesn't slow down your session startup.

### `amaru update [name]`

Updates to the latest version compatible with your declared ranges.

```
$ amaru update research
  ✓ Updating research: 1.0.3 → 1.1.0 (minor) [main]

Lock file updated.
```

Without arguments, updates everything.

### `amaru list`

Shows what's installed, with status and origin.

```
$ amaru list
Skills:
  research      1.0.3  ✓ up-to-date    [main]
  plan          1.0.1  ⚠ 1.0.2 avail   [main]
  deploycheck   1.0.0  ✓ up-to-date    [platform]

Commands:
  dev/bootstrap 2.0.0  ✓ up-to-date    [main]
```

### `amaru add <name> [--command] [--registry <alias>]`

Adds a skill or command to the manifest and installs it in one step.

```bash
amaru add research                         # add a skill
amaru add dev/bootstrap --command          # add a command
amaru add deploycheck --registry platform  # specify registry
```

If `--registry` is omitted and there are multiple registries, amaru searches all of them.

### `amaru browse [--registry <alias>]`

Discover what's available across your configured registries.

```
$ amaru browse
[main] github:acme-org/acme-skills
  Skills:
    research     1.0.3  [dev, core]      Search codebase and return compressed context
    plan         1.0.1  [dev, core]      Create plans with code snippets
  Commands:
    dev/bootstrap 2.0.0  [dev, setup]    Project bootstrap

[platform] github:acme-org/platform-skills
  Skills:
    deploycheck  1.0.0  [platform]       Verify deploy prerequisites
```

### `amaru ignore <name>` / `amaru unignore <name>`

Accept local drift for a specific item — `amaru check` will stop warning about hash mismatches for it.

```bash
amaru ignore plan        # accept local edits
amaru unignore plan      # re-enable drift warnings
```

## Authentication

amaru supports three auth methods per registry:

| Method | `auth` value | How it works |
|---|---|---|
| **GitHub CLI** | `"github"` | Uses your existing `gh` auth. **Recommended.** |
| **Token** | `"token"` | Reads `AMARU_TOKEN_<ALIAS>` env var. Good for CI/CD. |
| **Public** | `"none"` | No auth. For public registries. |

For token auth, the env var name is derived from the registry alias in uppercase:

```bash
# For a registry aliased as "platform":
export AMARU_TOKEN_PLATFORM="ghp_xxxxxxxxxxxx"
```

## Registry Structure

A registry is just a GitHub repo with this layout:

```
my-skills-registry/
├── skills/
│   ├── research/
│   │   ├── skill.md           # The skill content
│   │   ├── manifest.json      # Metadata + version
│   │   └── examples/          # Optional
│   └── plan/
│       ├── skill.md
│       └── manifest.json
├── commands/
│   └── dev/
│       └── bootstrap/
│           ├── command.md
│           └── manifest.json
└── registry.json              # Auto-generated index (by CI)
```

Versions are tracked via git tags: `skill/research/1.0.3`, `command/dev/bootstrap/2.0.0`.

amaru accesses registries through the GitHub API — it never clones the repo. It only downloads the files it needs.

## Session Start Hook

To get automatic update warnings when you start a Claude Code session, add a hook:

```bash
# .claude/hooks/session-start.sh
#!/bin/bash
if [ -f "amaru.json" ]; then
  amaru check --quiet 2>/dev/null
fi
```

## License

MIT
