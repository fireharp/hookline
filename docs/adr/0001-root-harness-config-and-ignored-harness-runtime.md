# ADR 0001: Root Harness Config and Ignored `.harness/` Runtime

## Status

Accepted

## Context

Hookline needs both durable configuration and local runtime state.

Configuration should be reviewable and portable when it defines shared project
behavior. Runtime state should be private to a machine and safe to delete. If
both live under `.harness/`, the repo needs fragile `.gitignore` exceptions
such as `!.harness/harness.yaml`, and it becomes easy to accidentally commit
logs, telemetry, cached hook payloads, recipe scratch files, or local machine
paths.

## Decision

`harness.yaml` at the repository root is the canonical tracked project config.

`.harness/` is reserved for local runtime data and is ignored as a whole.

Tracked project configuration stays outside `.harness/`:

- Project policy: `harness.yaml`
- Project Codex hook wiring: `.codex/hooks.json`

User-level defaults also stay outside project runtime state:

- User policy defaults: `~/.harness/hookline.yaml`
- User-level Codex hooks: `~/.codex/hooks.json`

Hookline must not introduce tracked config files under `.harness/`.

## Consequences

`.harness/` can contain telemetry, cached hook inputs and outputs, local state,
run artifacts, local recipe packs, and temporary files without per-file ignore
rules.

Repos can safely add this single ignore rule:

```gitignore
.harness/
```

Shared behavior remains reviewable through normal tracked files, especially
`harness.yaml` and `.codex/hooks.json`.

Project opt-out from user-level Hookline hooks stays in tracked project config:

```yaml
hooks:
  enabled: false
```

## Migration Rule

Legacy `.harness/harness.yaml` may be read as a migration fallback, but root
`harness.yaml` wins when both exist.

If a future version has config under `.harness/`, move it to `harness.yaml` for
project policy or `~/.harness/hookline.yaml` for user defaults before keeping
`.harness/` fully ignored.
