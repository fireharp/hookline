const events = [
  {
    id: "PreToolUse",
    title: "PreToolUse",
    description: "Before Bash, edit, write, or patch.",
    defaultOn: true,
  },
  {
    id: "PostToolUse",
    title: "PostToolUse",
    description: "After a tool finishes.",
    defaultOn: true,
  },
  {
    id: "UserPromptSubmit",
    title: "UserPromptSubmit",
    description: "Before a prompt enters the loop.",
    defaultOn: true,
  },
  {
    id: "Stop",
    title: "Stop",
    description: "Before the turn completes.",
    defaultOn: true,
  },
  {
    id: "precommit",
    title: "pre-commit",
    description: "Before git accepts staged files.",
    defaultOn: false,
  },
];

const recipes = [
  {
    id: "codex-hooks",
    title: "Codex Hooks",
    description: "Writes the Codex hook file that calls Hookline.",
    surfaces: ["codex", "doctor", "init"],
    defaultOn: true,
  },
  {
    id: "agent-steering",
    title: "Agent Steering",
    description: "Blocks risky shell, large diffs, and missing workflow context.",
    surfaces: ["hook", "doctor"],
    defaultOn: true,
  },
  {
    id: "line-count",
    title: "Line Count",
    description: "Warns when touched files cross the local size target.",
    surfaces: ["hook", "scan", "doctor"],
    defaultOn: true,
  },
  {
    id: "secrets-gitleaks",
    title: "Secrets and Gitleaks",
    description: "Checks staged changes for env literals and secret patterns.",
    surfaces: ["precommit", "scan", "doctor"],
    defaultOn: false,
  },
  {
    id: "coherence",
    title: "Coherence",
    description: "Runs repository consistency checks before commits.",
    surfaces: ["precommit", "doctor"],
    defaultOn: false,
  },
];
