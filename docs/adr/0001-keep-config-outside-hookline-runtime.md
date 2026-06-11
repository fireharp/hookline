# ADR 0001: Keep Config Outside `.hookline/`

## Status

Accepted

## Context

Hookline needs both durable configuration and local runtime state.

Configuration should be reviewable and portable when it defines shared project
behavior. Runtime state should be private to a machine and safe to delete. If
both live under `.hookline/`, the repo needs fragile `.gitignore` exceptions
such as `!.hookline/config.yaml`, and it becomes easy to accidentally commit
logs, telemetry, cached hook payloads, or local machine paths.

## Decision

`.hookline/` is reserved for local runtime data and is ignored as a whole.

Tracked project configuration stays outside `.hookline/`:

- Project policy: `.fireharp/harness.yaml`
- Project Codex hook wiring: `.codex/hooks.json`

User-level defaults also stay outside project runtime state:

- User policy defaults: `~/.fireharp/hookline.yaml`
- User-level Codex hooks: `~/.codex/hooks.json`

Hookline must not introduce tracked config files under `.hookline/`.

## Consequences

`.hookline/` can contain telemetry, cached hook inputs and outputs, local state,
run artifacts, and temporary files without per-file ignore rules.

Repos can safely add this single ignore rule:

```gitignore
.hookline/
```

Shared behavior remains reviewable through normal tracked files, especially
`.fireharp/harness.yaml` and `.codex/hooks.json`.

Project opt-out from user-level Hookline hooks stays in tracked project config:

```yaml
hooks:
  enabled: false
```

## Migration Rule

If a future version has config under `.hookline/`, move it to
`.fireharp/harness.yaml` for project policy or `~/.fireharp/hookline.yaml` for
user defaults before keeping `.hookline/` fully ignored.
