# Hookline

Hookline is a small local hook and steering CLI for agent-driven development.
The first version is deterministic: it adapts Codex hook JSON into a normalized
event, runs local policy checks, and maps the decision back to Codex hook output.

## Commands

```sh
go test ./...
go run ./cmd/hookline recipe list --json
go run ./cmd/hookline doctor --json
go run ./cmd/hookline scan lines --json
go run ./cmd/hookline scan secrets --json
go run ./cmd/hookline telemetry status --json
go run ./cmd/hookline telemetry tail --limit 20 --json
go run ./cmd/hookline snooze list --json
go run ./cmd/hookline decision list --json
go run ./cmd/hookline bench --suite smoke --json
```

The smoke bench records the full steering loop for each rule: starting problem,
Hookline signal, agent correction, and final result. See
[`docs/cases/hookline-steering-results.mdx`](docs/cases/hookline-steering-results.mdx).

## Recipe Setup

Hookline core owns routing, telemetry, output merging, and the deterministic
rule implementations. Recipe manifests are config files, not embedded runtime
data; this repo keeps its local recipe pack in `.harness/recipes/`.
Recipes do not affect hook behavior until enabled in config or applied with
`hookline init`.
Recipes can declare project-local `managed_files`; Hookline only writes or
prunes those files when the recipe is explicitly enabled or disabled.

Project recipes:

- `codex-hooks` writes Codex hook JSON.
- `coherence` adds deterministic repo consistency checks.
- `secrets-gitleaks` checks staged env literal leaks and gitleaks findings.
- `line-count` enables configurable line-count scan and hook review.
- `agent-steering` enables dangerous-shell, large-diff, sensitive-path, skill,
  and stop-continuation steering.
- `rtk-explicit-proxy` adds project-local RTK filters for explicit command use;
  it does not run `rtk init -g` or install global rewrite hooks.

One-project setup:

```sh
go run ./cmd/hookline init
```

With no `--recipe`, init applies the standard local setup: `codex-hooks`,
`coherence`, `secrets-gitleaks`, `line-count`, and `agent-steering`.
Pass one or more `--recipe` flags for a narrower setup.

Import existing Codex hook commands into a Hookline recipe:

```sh
go run ./cmd/hookline import codex-hooks \
  --from ../ProjectXYZ/.codex/hooks.json \
  --id projectxyz-hooks \
  --enable
go run ./cmd/hookline init --recipe codex-hooks --recipe projectxyz-hooks --force
```

For a one-step project replacement, use `--install --force`; Hookline writes
the imported recipe, enables it with `codex-hooks`, and replaces
`.codex/hooks.json` with Hookline wiring. The importer accepts `hooks.json`,
`hooks.yaml`, `hooks.yml`, and GitHub `blob` URLs.

Global Codex hook setup:

```sh
go install ./cmd/hookline
"$(go env GOPATH)/bin/hookline" init --recipe codex-hooks --scope user
```

`doctor` is read-only. Use it to verify setup without repairing anything:

```sh
go run ./cmd/hookline doctor --json
go run ./cmd/hookline doctor --recipe coherence --json
```

Optional recipes can be enabled or removed directly:

```sh
hookline recipe enable rtk-explicit-proxy
rtk git status
rtk test cargo test
hookline recipe disable rtk-explicit-proxy --prune-managed
```

## Codex Hooks

Codex can load hooks from user config (`~/.codex/hooks.json`) and project config
(`<repo>/.codex/hooks.json`). All matching hooks run. After init, use `/hooks`
in Codex to review and trust the new hook definition.

For this source repo, the project hook can call:

```sh
go run "$(git rev-parse --show-toplevel)/cmd/hookline" hook codex --source project
```

For a global user hook, use an installed binary instead:

```sh
go install ./cmd/hookline
"$(go env GOPATH)/bin/hookline" init --recipe codex-hooks --scope user
```

`init` writes source-tagged hooks. If user and project hooks are both present,
the project hook wins and the user hook suppresses itself. Older untagged hooks
are deduped locally for a short window and show up in telemetry as `source=auto`.

Project opt-out for global hooks:

```yaml
hooks:
  enabled: false
```

## Config

Config precedence is:

```text
built-in defaults < ~/.harness/hookline.yaml < harness.yaml
```

The project config defines the 500 LoC soft limit, dangerous shell patterns,
sensitive path globs, secret scanning settings, and skill-trigger nudges.
Telemetry is local-only by default and is written to `.harness/events.jsonl`.
Files over `limits.split_review_line_limit` trigger stronger steering when
touched by a hook event.

Legacy `.harness/harness.yaml` is read only as a migration fallback; root
`harness.yaml` wins when both exist.

Large-file steering can be snoozed or recorded for later runs:

```sh
hookline snooze add --rule large-file-split-review --path path/to/file.ts --scope session --duration 4h --session "$CODEX_SESSION_ID" --reason "unrelated to current task"
hookline decision add --rule large-file-split-review --path path/to/file.ts --action keep --why-split "clear seams exist" --why-not-now "unrelated to this task" --result "kept for now"
hookline telemetry tail --source project --rule large-file-split-review --json
```

Recipes are enabled explicitly:

```yaml
recipes:
  paths:
    - .harness/recipes
  enabled:
    - codex-hooks
    - coherence
    - secrets-gitleaks
    - line-count
    - agent-steering
```

Recipe manifests are loaded from `~/.harness/recipes/`,
`.harness/recipes/`, and any extra `recipes.paths` entries. A
manifest declares `id`, `title`, `description`, `surfaces`, optional
`commands`, optional `codex_hooks`, and optional `managed_files`.

`harness.yaml` is the tracked project config. `.harness/` is runtime/local
data and gitignored as a whole; see
[`docs/adr/0001-root-harness-config-and-ignored-harness-runtime.md`](docs/adr/0001-root-harness-config-and-ignored-harness-runtime.md).

## Secret Scanning

The generated pre-commit hook runs the enabled precommit recipes. For
`secrets-gitleaks`, that means:

```sh
hookline scan secrets
```

The scan compares staged additions against literal values in the local `.env`
file, reports only env keys and redacted value summaries, then runs gitleaks
with `.gitleaks.toml`.

## Coherence

This repo uses Coherence for local consistency checks. The git hook in
`.githooks/pre-commit` runs `coherence scan --staged` before secret checks.

Useful commands:

```sh
coherence doctor --json
coherence review --base=HEAD --worktree --json
coherence scan --staged --json
coherence index --json
```
