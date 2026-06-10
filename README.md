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
go run ./cmd/hookline bench --suite smoke --json
```

Codex project hooks live in `.codex/hooks.json` and call:

```sh
go run "$(git rev-parse --show-toplevel)/cmd/hookline" hook codex
```

Codex requires project hooks to be trusted. Use `/hooks` in Codex to review and
trust the project hook definitions after they change.

## Config

Config precedence is:

```text
built-in defaults < ~/.fireharp/hookline.yaml < .fireharp/harness.yaml
```

The project config defines the 500 LoC soft limit, dangerous shell patterns,
sensitive path globs, secret scanning settings, and skill-trigger nudges.

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
