const state = {
  scope: "project",
  selectedEvents: new Set(events.filter((event) => event.defaultOn).map((event) => event.id)),
  selectedRecipes: new Set(recipes.filter((recipe) => recipe.defaultOn).map((recipe) => recipe.id)),
  custom: {
    enabled: false,
    id: "personal-binary",
    path: "/Users/me/bin/personal-check",
    args: "--hook codex",
    surfaces: new Set(["hook", "doctor"]),
  },
  verificationMode: "idle",
};

const els = {
  lineStage: document.querySelector("#line-stage"),
  selectedCount: document.querySelector("#selected-count"),
  surfaceSummary: document.querySelector("#surface-summary"),
  customSummary: document.querySelector("#custom-summary"),
  setupCode: document.querySelector("#setup-code"),
  manifestCode: document.querySelector("#manifest-code"),
  customEnabled: document.querySelector("#custom-enabled"),
  customId: document.querySelector("#custom-id"),
  customPath: document.querySelector("#custom-path"),
  customArgs: document.querySelector("#custom-args"),
  customError: document.querySelector("#custom-error"),
  checks: document.querySelector("#checks"),
  runVerify: document.querySelector("#run-verify"),
};

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function yamlQuote(value) {
  return `"${String(value).replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

function shellQuote(value) {
  if (/^[A-Za-z0-9_./:$~-]+$/.test(value)) {
    return value;
  }
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}

function splitArgs(value) {
  const matches = String(value).trim().match(/"[^"]*"|'[^']*'|\S+/g) || [];
  return matches.map((part) => part.replace(/^["']|["']$/g, ""));
}

function selectedRecipeList() {
  return recipes.filter((recipe) => state.selectedRecipes.has(recipe.id));
}

function hasPrecommitRecipe() {
  return selectedRecipeList().some((recipe) => recipe.surfaces.includes("precommit")) ||
    (state.custom.enabled && state.custom.surfaces.has("precommit"));
}

function getCustomError() {
  if (!state.custom.enabled) {
    return "";
  }
  if (!/^[a-z0-9][a-z0-9-]*$/.test(state.custom.id)) {
    return "Use lowercase letters, numbers, and hyphens for the recipe id.";
  }
  const path = state.custom.path.trim();
  if (!path.startsWith("/") && !path.startsWith("$HOME/") && !path.startsWith("~/")) {
    return "Use an absolute path, a $HOME path, or a ~/ path for the binary.";
  }
  if (state.custom.surfaces.size === 0) {
    return "Choose at least one surface for the custom command.";
  }
  return "";
}

function renderLine() {
  const privateCount = state.custom.enabled && !getCustomError() ? 1 : 0;
  const builtInCount = state.selectedRecipes.size;
  els.selectedCount.textContent = privateCount
    ? `${builtInCount} built-in, ${privateCount} private`
    : `${builtInCount} built-in selected`;
  els.surfaceSummary.textContent = scopeSummary();
  els.customSummary.textContent = state.custom.enabled
    ? `${state.custom.id} on ${Array.from(state.custom.surfaces).join(", ") || "no surface"}`
    : "No local command";

  els.lineStage.innerHTML = `
    <div class="assembly-flow">
      <section class="assembly-column">
        <span class="column-label">1. When this happens</span>
        <div class="assembly-items">
          ${events
            .map((event) => {
              const active = state.selectedEvents.has(event.id);
              return `
                <label class="assembly-item ${active ? "" : "is-off"}">
                  <input type="checkbox" data-event="${event.id}" ${active ? "checked" : ""} />
                  <span>
                    <strong>${event.title}</strong>
                    <small>${event.description}</small>
                  </span>
                </label>
              `;
            })
            .join("")}
        </div>
      </section>
      <div class="flow-arrow" aria-hidden="true">runs</div>
      <section class="assembly-column">
        <span class="column-label">2. Hookline runs this</span>
        <div class="assembly-items">
          ${recipes
            .map((recipe) => {
              const active = state.selectedRecipes.has(recipe.id);
              return `
                <label class="assembly-item ${active ? "is-selected" : "is-off"}">
                  <input type="checkbox" data-recipe="${recipe.id}" ${active ? "checked" : ""} />
                  <span>
                    <strong>${recipe.title}</strong>
                    <small>${recipe.id} on ${recipe.surfaces.join(" + ")}</small>
                  </span>
                </label>
              `;
            })
            .join("")}
          <label class="assembly-item ${state.custom.enabled ? "is-selected" : "is-off"}">
            <input type="checkbox" data-custom-recipe ${state.custom.enabled ? "checked" : ""} />
            <span>
              <strong>Private command</strong>
              <small>${escapeHtml(state.custom.id)} on ${escapeHtml(Array.from(state.custom.surfaces).join(" + ") || "no surface")}</small>
            </span>
          </label>
        </div>
      </section>
      <div class="flow-arrow" aria-hidden="true">then</div>
      <section class="assembly-column">
        <span class="column-label">3. Doctor must prove</span>
        <div class="assembly-items">
          <article class="assembly-item">
            <strong>Hook files</strong>
            <span>Codex and git paths point at Hookline.</span>
          </article>
          <article class="assembly-item">
            <strong>Binaries</strong>
            <span>External commands exist and report versions.</span>
          </article>
          <article class="assembly-item">
            <strong>Local state</strong>
            <span>Snoozes, telemetry, and config resolve locally.</span>
          </article>
        </div>
      </section>
    </div>
  `;
  bindMapControls();
}

function bindMapControls() {
  els.lineStage.querySelectorAll("[data-event]").forEach((input) => {
    input.addEventListener("change", () => {
      input.checked ? state.selectedEvents.add(input.dataset.event) : state.selectedEvents.delete(input.dataset.event);
      state.verificationMode = "idle";
      render();
    });
  });
  els.lineStage.querySelectorAll("[data-recipe]").forEach((input) => {
    input.addEventListener("change", () => {
      input.checked ? state.selectedRecipes.add(input.dataset.recipe) : state.selectedRecipes.delete(input.dataset.recipe);
      if (hasPrecommitRecipe()) {
        state.selectedEvents.add("precommit");
      }
      state.verificationMode = "idle";
      render();
    });
  });
  const custom = els.lineStage.querySelector("[data-custom-recipe]");
  custom.addEventListener("change", () => {
    state.custom.enabled = custom.checked;
    state.verificationMode = "idle";
    render();
  });
}

function scopeSummary() {
  return {
    project: "Project scope writes repo config",
    user: "User scope writes home config",
    both: "Both scopes write repo and home config",
  }[state.scope];
}

function manifestPath() {
  if (state.scope === "user") {
    return "$HOME/.harness/recipes";
  }
  return ".harness/recipes";
}

function customRecipeId() {
  return state.custom.id.trim() || "personal-binary";
}

function setupRecipeIds() {
  const ids = selectedRecipeList().map((recipe) => recipe.id);
  if (state.custom.enabled && !getCustomError()) {
    ids.push(customRecipeId());
  }
  return ids;
}

function buildSetupCode() {
  const ids = setupRecipeIds();
  const lines = [];

  if (state.custom.enabled) {
    const dir = manifestPath();
    lines.push(`# Add the custom command manifest.`);
    lines.push(`mkdir -p ${shellQuote(dir)}`);
    lines.push(`cat > ${shellQuote(`${dir}/${customRecipeId()}.yaml`)} <<'YAML'`);
    lines.push(buildManifestCode());
    lines.push("YAML");
    lines.push("");
  }

  if (ids.length === 0) {
    lines.push("# Select at least one recipe to generate the init command.");
  } else {
    lines.push("# Enable the selected recipes.");
    lines.push("go run ./cmd/hookline init \\");
    ids.forEach((id) => {
      lines.push(`  --recipe ${id} \\`);
    });
    lines.push(`  --scope ${state.scope} \\`);
    lines.push("  --json");
  }

  lines.push("");
  lines.push("# Verify config, hook files, external commands, and active local state.");
  if (ids.length > 0) {
    lines.push("go run ./cmd/hookline doctor --json");
  } else {
    lines.push("# go run ./cmd/hookline doctor --json");
  }

  return lines.join("\n");
}

function buildManifestCode() {
  if (!state.custom.enabled) {
    return [
      "# Enable the local command switch to preview a recipe manifest.",
      "# Custom manifests are loaded from ~/.harness/recipes",
      "# or .harness/recipes.",
    ].join("\n");
  }

  const surfaces = Array.from(state.custom.surfaces);
  const args = [state.custom.path.trim(), ...splitArgs(state.custom.args)];
  const commandArgs = `[${args.map(yamlQuote).join(", ")}]`;
  const lines = [
    `id: ${customRecipeId()}`,
    "title: Personal Binary",
    "description: Run a local binary as part of the Hookline setup.",
    "surfaces:",
  ];

  if (surfaces.length === 0) {
    lines.push("  []");
  } else {
    surfaces.forEach((surface) => lines.push(`  - ${surface}`));
  }

  if (surfaces.length > 0) {
    lines.push("commands:");
    surfaces.forEach((surface) => {
      lines.push(`  ${surface}:`);
      lines.push(`    - args: ${commandArgs}`);
    });
  }

  return lines.join("\n");
}

function buildChecks() {
  const ids = setupRecipeIds();
  const customError = getCustomError();
  const selectedExternal = ids.includes("coherence") || ids.includes("secrets-gitleaks") || state.custom.enabled;
  const mode = state.verificationMode;
  const pendingStatus = mode === "running" ? "running" : "pending";

  const checks = [
    {
      title: "Repo root",
      status: mode === "complete" ? "pass" : pendingStatus,
      message: "FindRoot should resolve before Hookline reads project config.",
    },
    {
      title: "Enabled recipes",
      status: ids.length > 0 ? (mode === "complete" ? "pass" : pendingStatus) : "fail",
      message: ids.length > 0 ? ids.join(", ") : "Choose at least one recipe.",
    },
    {
      title: "Codex hook file",
      status: ids.includes("codex-hooks") ? (mode === "complete" ? "pass" : pendingStatus) : "pending",
      message: ids.includes("codex-hooks")
        ? "Init should write .codex/hooks.json or ~/.codex/hooks.json."
        : "Select codex-hooks to install Codex hook wiring.",
    },
    {
      title: "Pre-commit path",
      status: hasPrecommitRecipe() ? (mode === "complete" ? "pass" : pendingStatus) : "pending",
      message: hasPrecommitRecipe()
        ? "Init should write .githooks/pre-commit and set core.hooksPath."
        : "No pre-commit recipe is selected.",
    },
    {
      title: "Custom binary",
      status: state.custom.enabled
        ? customError
          ? "fail"
          : mode === "complete"
            ? "pass"
            : pendingStatus
        : "pending",
      message: state.custom.enabled
        ? customError || "Path shape is valid. A terminal doctor run must prove it exists."
        : "Local command is disabled.",
    },
    {
      title: "External versions",
      status: selectedExternal ? (mode === "complete" ? "pending" : pendingStatus) : "pass",
      message: selectedExternal
        ? "Browser demo cannot run binaries. Use doctor to verify gitleaks, coherence, and custom commands."
        : "No external version checks selected.",
    },
  ];

  return checks;
}

function renderChecks() {
  els.checks.innerHTML = buildChecks()
    .map(
      (check) => `
        <article class="verify-row">
          <span class="status ${check.status}">${check.status}</span>
          <span class="verify-message">
            <strong>${escapeHtml(check.title)}</strong>
            <span>${escapeHtml(check.message)}</span>
          </span>
        </article>
      `,
    )
    .join("");
}

function bindStaticControls() {
  document.querySelectorAll("input[name='scope']").forEach((input) => {
    input.addEventListener("change", () => {
      state.scope = input.value;
      state.verificationMode = "idle";
      render();
    });
  });

  els.customEnabled.addEventListener("change", () => {
    state.custom.enabled = els.customEnabled.checked;
    state.verificationMode = "idle";
    render();
  });

  [
    [els.customId, "id"],
    [els.customPath, "path"],
    [els.customArgs, "args"],
  ].forEach(([input, key]) => {
    input.addEventListener("input", () => {
      state.custom[key] = input.value;
      state.verificationMode = "idle";
      render();
    });
  });

  document.querySelectorAll("input[name='customSurface']").forEach((input) => {
    input.addEventListener("change", () => {
      input.checked ? state.custom.surfaces.add(input.value) : state.custom.surfaces.delete(input.value);
      if (hasPrecommitRecipe()) {
        state.selectedEvents.add("precommit");
      }
      state.verificationMode = "idle";
      render();
    });
  });

  document.querySelectorAll("[data-copy]").forEach((button) => {
    button.addEventListener("click", async () => {
      const target = document.querySelector(`#${button.dataset.copy}`);
      await navigator.clipboard.writeText(target.textContent);
      const previous = button.textContent;
      button.textContent = "Copied";
      setTimeout(() => {
        button.textContent = previous;
      }, 900);
    });
  });

  els.runVerify.addEventListener("click", () => {
    state.verificationMode = "running";
    renderChecks();
    els.runVerify.disabled = true;
    setTimeout(() => {
      state.verificationMode = "complete";
      els.runVerify.disabled = false;
      renderChecks();
    }, 650);
  });
}

function renderCustomControls() {
  els.customEnabled.checked = state.custom.enabled;
  els.customId.value = state.custom.id;
  els.customPath.value = state.custom.path;
  els.customArgs.value = state.custom.args;
  document.querySelectorAll("input[name='customSurface']").forEach((input) => {
    input.checked = state.custom.surfaces.has(input.value);
  });

  const customError = getCustomError();
  els.customError.hidden = !customError;
  els.customError.textContent = customError;
}

function renderOutputs() {
  els.setupCode.textContent = buildSetupCode();
  els.manifestCode.textContent = buildManifestCode();
}

function render() {
  renderCustomControls();
  renderLine();
  renderOutputs();
  renderChecks();
}

bindStaticControls();
render();
