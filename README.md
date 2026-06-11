# Hookline

Hookline is a small local hook and steering CLI for agent-driven development.
The first version is deterministic: it adapts Codex hook JSON into a normalized
event, runs local policy checks, and maps the decision back to Codex hook output.

## Commands

```sh
go test ./...
go run ./cmd/hookline doctor --json
go run ./cmd/hookline scan lines --json
go run ./cmd/hookline scan secrets --json
go run ./cmd/hookline telemetry status --json
go run ./cmd/hookline telemetry tail --limit 20 --json
go run ./cmd/hookline bench --suite smoke --json
```

The smoke bench records the full steering loop for each rule: starting problem,
Hookline signal, agent correction, and final result. See
[`docs/cases/hookline-steering-results.mdx`](docs/cases/hookline-steering-results.mdx).

## Codex Hook Setup

Codex can load hooks from user config (`~/.codex/hooks.json`) and project config
(`<repo>/.codex/hooks.json`). All matching hooks run, so prefer one of these:

- All projects: install Hookline once, then run `hookline init --scope user`.
- One project only: run `hookline init --scope project` from that repo.

After init, use `/hooks` in Codex to review and trust the new hook definition.

For this source repo, the project hook can call:

```sh
go run "$(git rev-parse --show-toplevel)/cmd/hookline" hook codex
```

For a global user hook, use an installed binary instead:

```sh
go install ./cmd/hookline
"$(go env GOPATH)/bin/hookline" init --scope user
```

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

`.hookline/` is runtime-only and gitignored as a whole. Config intentionally
lives outside that directory; see
[`docs/adr/0001-keep-config-outside-hookline-runtime.md`](docs/adr/0001-keep-config-outside-hookline-runtime.md).

## Secret Scanning

The pre-commit hook runs:

```sh
.githooks/check-env-leaks.sh
gitleaks git --pre-commit --staged --redact --no-banner .
```

`check-env-leaks.sh` compares staged additions against literal values in the
local `.env` file, but reports only env keys and redacted value summaries.
Gitleaks handles generic token and private-key shapes using `.gitleaks.toml`.

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
