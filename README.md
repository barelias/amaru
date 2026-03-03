# 🐍 amaru

**Skills, commands & agents manager for Claude Code.**

amaru connects your projects to centralized registries hosted on GitHub — managing skills, commands, and agents. It tracks versions, detects local drift, warns about updates, syncs shared context documentation, and keeps your whole team in sync — all through a simple manifest file.

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

# 3. Add skills, commands, and agents
amaru add research
amaru add dev/bootstrap --type command
amaru add code-reviewer --type agent

# 3b. Or add a whole skillset at once
amaru add starter-pack --type skillset

# 4. Install everything
amaru install

# 5. Set up shared context documentation
amaru context init

# 6. Check for updates later
amaru check
```

## How It Works

amaru manages two files at the root of your project:

| File | Purpose | Committed? |
|---|---|---|
| `amaru.json` | Manifest — declares registries, skills, commands, agents, skillsets, and context config | Yes |
| `amaru.lock` | Lock — resolved versions, hashes, skillset digests, timestamps for reproducibility | Yes |

Skills are installed to `.claude/skills/`, commands to `.claude/commands/`, and agents to `.claude/agents/`, matching the Claude Code convention.

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
  },
  "agents": {
    "code-reviewer": { "version": "^1.0.0", "registry": "main" }
  },
  "context": {
    "registry": "main",
    "project": "my-app",
    "path": "docs/context"
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
  },
  "agents": {
    "code-reviewer": "^1.0.0"
  }
}
```

**Version ranges** follow npm conventions: `^` (minor + patch), `~` (patch only), exact (`1.2.3`).

## Commands

### `amaru init`

Interactive setup — creates `amaru.json` with your registries configured.

```
$ amaru init
Registry URL (ex: github:org/skills-repo): git@github.com:acme-org/acme-skills.git
  → normalized to: github:acme-org/acme-skills
Registry alias [acme]: acme
Auth method (github/token/none) [github]: github
Adicionar outro registry? (y/N): N

amaru.json criado. Rode `amaru browse` para ver skills disponíveis.
```

Accepts any GitHub URL format — SSH (`git@github.com:org/repo.git`), HTTPS (`https://github.com/org/repo`), `ssh://`, `http://`, bare domain (`github.com/org/repo`), or the canonical shorthand (`github:org/repo`). All formats are normalized automatically.

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

Installing agents...
  ✓ code-reviewer@1.0.0 (main)

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

### `amaru update --skillset <name>`

Updates all members of a skillset. Detects new members added to the skillset in the registry and installs them automatically.

```
$ amaru update --skillset starter-pack
Updating skillset "starter-pack" (4 members)...
  ✓ Updating research: 1.0.3 → 1.1.0 (minor) [main]
  ✓ Added command deploy-check@1.0.0 (new member)

Skillset "starter-pack": 1 updated, 1 added.
```

The skillset digest in the lock file tracks whether any member versions have changed since the last install.

### `amaru list`

Shows what's installed, with status and origin.

```
$ amaru list
Skills:
  research      1.0.3  ✓ up-to-date    [main] (via starter-pack)
  plan          1.0.1  ⚠ 1.0.2 avail   [main] (via starter-pack)
  deploycheck   1.0.0  ✓ up-to-date    [platform]

Commands:
  dev/bootstrap 2.0.0  ✓ up-to-date    [main]

Agents:
  code-reviewer 1.0.0  ✓ up-to-date    [main]
```

Items installed via a skillset show their group provenance.

### `amaru add <name> [--type <type>] [--registry <alias>]`

Adds a skill, command, agent, or skillset to the manifest and installs it in one step.

```bash
amaru add research                         # add a skill (default)
amaru add dev/bootstrap --type command     # add a command
amaru add code-reviewer --type agent       # add an agent
amaru add deploycheck --registry platform  # specify registry
amaru add starter-pack --type skillset     # add all items in a skillset
```

If `--registry` is omitted and there are multiple registries, amaru searches all of them.

**Skillsets** are registry-defined groups of skills, commands, and agents. Adding a skillset expands it to individual items in your manifest, each tagged with its origin group. Items that are already in your manifest are skipped.

**Unversioned items** (no git tags in the registry) are tracked with the `"latest"` constraint — amaru downloads from the default branch and uses content hashing instead of semver for change detection.

### `amaru browse [--registry <alias>]`

Discover what's available across your configured registries.

```
$ amaru browse
[main] github:acme-org/acme-skills
  Skills:
    research      1.0.3  [dev, core]      Search codebase and return compressed context
    plan          1.0.1  [dev, core]      Create plans with code snippets
  Commands:
    dev/bootstrap 2.0.0  [dev, setup]     Project bootstrap
  Agents:
    code-reviewer 1.0.0  [dev, review]    Review code changes with context
  Skillsets:
    starter-pack  (3 items) [onboarding]  Essential skills for new projects

[platform] github:acme-org/platform-skills
  Skills:
    deploycheck   1.0.0  [platform]       Verify deploy prerequisites
```

### `amaru ignore <name>` / `amaru unignore <name>`

Accept local drift for a specific item — `amaru check` will stop warning about hash mismatches for it.

```bash
amaru ignore plan        # accept local edits
amaru unignore plan      # re-enable drift warnings
```

### `amaru context init`

Sets up shared context documentation for your project. Sparse-clones the context directory from your registry into `.claude/.amaru-context/` and symlinks it to `docs/context`.

```bash
$ amaru context init
Cloning context for project "my-app"...
  ✓ Sparse checkout from main registry
  ✓ Symlinked to docs/context
  ✓ Added .claude/.amaru-context/ to .gitignore
  ✓ Installed git hooks (post-checkout, post-commit)
```

This gives your project access to shared brainstorms, plans, and solutions from the centralized registry.

### `amaru context sync`

Pulls the latest context documentation from the registry.

```bash
amaru context sync
```

This runs automatically via the post-checkout git hook after branch switches.

### `amaru context push`

Stages and pushes local context changes back to the centralized registry.

```bash
amaru context push
```

The post-commit git hook auto-pushes when it detects changes to context files.

### `amaru context path`

Prints the local context directory path.

```bash
$ amaru context path
docs/context
```

### `amaru repo init <path> --project <name>`

Scaffolds a new registry repository with the standard directory structure.

```bash
$ amaru repo init /path/to/registry --project my-app
Creating registry at /path/to/registry...
  ✓ registry.json
  ✓ AGENTS.md
  ✓ skills/
  ✓ commands/
  ✓ agents/
  ✓ context/my-app/
  ✓ .sparse-profiles/my-app
```

The generated structure includes `AGENTS.md` navigation files and a per-project context directory with `brainstorms/`, `plans/`, and `solutions/` subdirectories.

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
├── amaru_registry.json            # Package index (auto-updated by CI)
├── AGENTS.md                      # Root navigation + registry structure
└── .amaru_registry/               # All registry content
    ├── .sparse-profiles/          # Sapling sparse checkout profiles
    │   └── my-app
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
    ├── agents/
    │   └── code-reviewer/
    │       ├── agent.md
    │       └── manifest.json
    └── context/
        └── my-app/
            ├── AGENTS.md          # Per-project navigation
            ├── brainstorms/
            ├── plans/
            └── solutions/
```

The `.amaru_registry/` prefix keeps registry content separate from the repo's own source code, making it easy for any tool to double as its own registry.

Versions are tracked via git tags: `skill/research/1.0.3`, `command/dev/bootstrap/2.0.0`, `agent/code-reviewer/1.0.0`.

Skillsets are defined in `amaru_registry.json`:

```jsonc
{
  "skillsets": {
    "starter-pack": {
      "description": "Essential skills for new projects",
      "tags": ["onboarding"],
      "items": [
        { "type": "skill", "name": "research" },
        { "type": "skill", "name": "plan" },
        { "type": "command", "name": "dev/bootstrap" }
      ]
    }
  }
}
```

amaru accesses registries through the GitHub API for installable items. For context sync, it uses sparse checkout via Sapling (preferred) or Git.

## VCS Support

amaru supports two version control backends for context sync:

| Backend | Detection | Sparse checkout method |
|---|---|---|
| **Sapling** | `sl` on PATH | `sl clone --enable-profile` with sparse profiles |
| **Git** | Fallback | `git clone --filter=blob:none --no-checkout` + `git sparse-checkout set` |

Sapling is preferred when available — it handles sparse checkouts of large registries more efficiently. amaru auto-detects the available backend.

## Hooks

`amaru context init` installs two git hooks automatically:

- **post-checkout** — runs `amaru context sync` after branch switches
- **post-commit** — detects context file changes and auto-pushes via `amaru context push`

Hooks are idempotent (safe to re-install) and fail silently — they never block your workflow.

### Session Start Hook

To get automatic update warnings when you start a Claude Code session, add a hook:

```bash
# .claude/hooks/session-start.sh
#!/bin/bash
if [ -f "amaru.json" ]; then
  amaru check --quiet 2>/dev/null
fi
```

## Self-Hosted Registry

This repo is its own registry — it ships an `amaru-usage` skill that teaches Claude Code how to use amaru. Any tool can do the same: add `amaru_registry.json` and `.amaru_registry/` to your repo.

```bash
# In any project:
amaru init                    # Use github:barelias/amaru as the registry URL
amaru add amaru-usage         # Install the amaru-usage skill
```

## License

MIT
