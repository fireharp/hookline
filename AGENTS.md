# Repository Guidelines

## Project Structure & Module Organization

Hookline is a Go CLI. `cmd/hookline` owns command dispatch; internal packages
keep behavior small and testable: `config` loads defaults plus user/project
YAML, `codex` translates Codex hook JSON, `engine` evaluates policy decisions,
`lines` scans file sizes, `secrets` checks staged leaks, and `bench` runs smoke
scenarios. Project policy lives in `harness.yaml`; Codex hook wiring
lives in `.codex/hooks.json`.

## Build, Test, and Development Commands

- `go test ./...` runs all Go tests.
- `go run ./cmd/hookline doctor --json` checks local setup.
- `go run ./cmd/hookline scan lines --json` reports files over the soft line limit.
- `go run ./cmd/hookline scan secrets --json` checks staged secret leaks.
- `go run ./cmd/hookline telemetry tail --limit 20 --json` shows local hook runs.
- `go run ./cmd/hookline snooze list --json` shows active local steering snoozes.
- `go run ./cmd/hookline bench --suite smoke --json` runs realistic local scenarios.
- `coherence review --base=HEAD --worktree --json` checks repo drift before handoff.

## Coding Style & Naming Conventions

Use `gofmt` on Go files. Keep packages narrow and files around the 500 LoC soft
limit; split by responsibility before files grow into broad policy buckets.
Config is YAML for user/project policy and JSON only where Codex requires it.

## Testing Guidelines

Add focused Go tests for adapters, rule decisions, line scanning, secret
redaction, and bench scenarios. Codex hook fixtures live under `testdata/codex`
and should exercise supported hook events without requiring a live Codex run.

## Commit & Pull Request Guidelines

The short git history uses concise lowercase subjects. Preserve the existing
Coherence setup: `.coherence/` stays ignored, while `ontology.yml`,
`.githooks/`, and `.agents/skills/coherence/` are durable repo files.
