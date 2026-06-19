package recipes

var standardRecipeManifests = []string{
	`id: codex-hooks
title: Codex Hooks
description: Write Codex hooks.json routing so Codex events invoke hookline hook codex.
surfaces: [codex, doctor, init]
`,
	`id: coherence
title: Coherence
description: Run external coherence checks during doctor and generated pre-commit hooks.
surfaces: [doctor, init, precommit]
commands:
  doctor:
    - args: ["coherence", "doctor", "--json"]
`,
	`id: secrets-gitleaks
title: Secrets and Gitleaks
description: Enable scan secrets, verify gitleaks, and manage .gitleaks.toml for staged secret checks.
surfaces: [doctor, init, precommit, scan]
commands:
  doctor:
    - args: ["gitleaks", "version"]
managed_files:
  - path: .gitleaks.toml
    mode: "0644"
    action: write-gitleaks-config
    content: |
      title = "hookline gitleaks config"
      # Managed by hookline. Rerun hookline init to update.

      [extend]
      useDefault = true

      [[rules]]
      id = "hookline-tailscale-magicdns"
      description = "Tailscale MagicDNS hostname"
      regex = '''[A-Za-z0-9][A-Za-z0-9-]*\.[A-Za-z0-9-]+\.ts\.net'''
      keywords = [".ts.net"]
      tags = ["hostname", "tailscale"]

      [[rules.allowlists]]
      description = "Allow documented placeholder hostnames"
      paths = [
        '''\.gitleaks\.toml$''',
        '''\.env\.example$''',
        '''^docs/''',
      ]
`,
	`id: line-count
title: Line Count
description: Enable scan lines and hook split-review checks using limits.file_line_limit and limits.split_review_line_limit.
surfaces: [doctor, hook, scan]
`,
	`id: agent-steering
title: Agent Steering
description: Enable hook decisions driven by dangerous_shell, sensitive_paths, skill_triggers, large_diff, TODO, and stop-review config.
surfaces: [doctor, hook]
`,
}
