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

Hookline core is recipe-less by default. Built-in recipes are available, but
they do not affect hook behavior until enabled in config or applied with
`hookline init --recipe ...`.

Built-in recipes:

- `codex-hooks` writes Codex hook JSON.
- `coherence` adds deterministic repo consistency checks.
- `secrets-gitleaks` checks staged env literal leaks and gitleaks findings.
- `line-count` enables configurable line-count scan and hook review.
- `agent-steering` enables dangerous-shell, large-diff, sensitive-path, skill,
  and stop-continuation steering.

One-project setup:

```sh
go run ./cmd/hookline init \
  --recipe codex-hooks \
  --recipe coherence \
  --recipe secrets-gitleaks \
  --recipe line-count \
  --recipe agent-steering
```

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
built-in defaults < ~/.fireharp/hookline.yaml < .fireharp/harness.yaml
```

The project config defines the 500 LoC soft limit, dangerous shell patterns,
sensitive path globs, secret scanning settings, and skill-trigger nudges.
Telemetry is local-only by default and is written to `.hookline/events.jsonl`.
Files over `limits.split_review_line_limit` trigger stronger steering when
touched by a hook event.

Large-file steering can be snoozed or recorded for later runs:

```sh
hookline snooze add --rule large-file-split-review --path path/to/file.ts --scope session --duration 4h --session "$CODEX_SESSION_ID" --reason "unrelated to current task"
hookline decision add --rule large-file-split-review --path path/to/file.ts --action keep --why-split "clear seams exist" --why-not-now "unrelated to this task" --result "kept for now"
hookline telemetry tail --source project --rule large-file-split-review --json
```

Recipes are enabled explicitly:

```yaml
recipes:
  enabled:
    - codex-hooks
    - coherence
    - secrets-gitleaks
    - line-count
    - agent-steering
```

Optional command-plugin manifests can be placed under
`~/.fireharp/hookline/recipes/` or `.fireharp/hookline/recipes/`, or referenced
with `recipes.paths`.

`.hookline/` is runtime-only and gitignored as a whole. Config intentionally
lives outside that directory; see
[`docs/adr/0001-keep-config-outside-hookline-runtime.md`](docs/adr/0001-keep-config-outside-hookline-runtime.md).

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
