package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
)

func TestBuildCodexHooksUsesCommandForAllEvents(t *testing.T) {
	file := buildCodexHooks("hookline hook codex")
	for _, event := range []string{"PreToolUse", "PostToolUse", "UserPromptSubmit", "Stop"} {
		matchers := file.Hooks[event]
		if len(matchers) != 1 || len(matchers[0].Hooks) != 1 {
			t.Fatalf("expected one hook for %s, got %#v", event, matchers)
		}
		if matchers[0].Hooks[0].Command != "hookline hook codex" {
			t.Fatalf("expected command for %s, got %#v", event, matchers[0].Hooks[0])
		}
	}
	if file.Hooks["PreToolUse"][0].Matcher == "" {
		t.Fatal("expected PreToolUse matcher")
	}
}

func TestCommandForScopeAddsSourceTag(t *testing.T) {
	got := commandForScope("hookline hook codex", "project")
	if got != "hookline hook codex --source project" {
		t.Fatalf("expected project source tag, got %q", got)
	}
	if existing := commandForScope("hookline hook codex --source user", "project"); existing != "hookline hook codex --source user" {
		t.Fatalf("expected existing source tag to remain, got %q", existing)
	}
}

func TestWriteHookFileRefusesDifferentExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeHookFile(path, "hookline hook codex", false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
	changed, err := writeHookFile(path, "hookline hook codex", true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected forced write to report changed")
	}
	changed, err = writeHookFile(path, "hookline hook codex", false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected identical file to report unchanged")
	}
}

func TestParseInitOptionsDefaultsToStandardRecipes(t *testing.T) {
	opts, err := parseInitOptions([]string{"--scope", "project"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(opts.Recipes, ",") != strings.Join(recipes.StandardRecipeIDs(), ",") {
		t.Fatalf("expected standard recipes, got %#v", opts.Recipes)
	}
}

func TestInitRecipesWithDefaultsWritesProjectSetup(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")
	coherenceCalls := stubCoherenceInit(t)

	cfg := config.Default()
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := initRecipes([]string{"--json"}, &out, root, cfg, registry); err != nil {
		t.Fatal(err)
	}
	configData := readFile(t, config.ProjectConfigPath(root))
	for _, id := range recipes.StandardRecipeIDs() {
		if !strings.Contains(configData, id) {
			t.Fatalf("expected default recipe %s in config:\n%s", id, configData)
		}
	}
	for _, path := range []string{
		filepath.Join(root, ".codex", "hooks.json"),
		filepath.Join(root, ".githooks", "pre-commit"),
		filepath.Join(root, ".gitleaks.toml"),
	} {
		if !fileExists(path) {
			t.Fatalf("expected %s", path)
		}
	}
	cmd := exec.Command("git", "config", "--get", "core.hooksPath")
	cmd.Dir = root
	got, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != ".githooks" {
		t.Fatalf("expected core.hooksPath .githooks, got %q", got)
	}
	if *coherenceCalls != 1 {
		t.Fatalf("expected coherence init to run once, got %d", *coherenceCalls)
	}
	if !fileExists(filepath.Join(root, "ontology.yml")) {
		t.Fatal("expected coherence init artifact")
	}
}

func TestInitRecipesWritesPrecommitAndConfiguresGitHooksPath(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")
	coherenceCalls := stubCoherenceInit(t)

	cfg := config.Default()
	writeRecipePack(t, root, recipes.Coherence, recipes.SecretsGitleaks)
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = initRecipes([]string{"--recipe", "coherence", "--recipe", "secrets-gitleaks", "--json"}, &out, root, cfg, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(root, ".githooks", "pre-commit")) {
		t.Fatal("expected pre-commit hook")
	}
	if !strings.Contains(readFile(t, config.ProjectConfigPath(root)), "secrets-gitleaks") {
		t.Fatal("expected harness recipe enablement")
	}
	if !strings.Contains(readFile(t, filepath.Join(root, ".gitleaks.toml")), managedByHookline) {
		t.Fatal("expected managed gitleaks config")
	}
	cmd := exec.Command("git", "config", "--get", "core.hooksPath")
	cmd.Dir = root
	got, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != ".githooks" {
		t.Fatalf("expected core.hooksPath .githooks, got %q", got)
	}
	if *coherenceCalls != 1 {
		t.Fatalf("expected coherence init to run once, got %d", *coherenceCalls)
	}
}

func TestInitRecipesRefusesUnmanagedPrecommitConflict(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")
	stubCoherenceInit(t)
	path := filepath.Join(root, ".githooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/usr/bin/env sh\necho custom\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	writeRecipePack(t, root, recipes.Coherence)
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := initRecipes([]string{"--recipe", "coherence"}, &bytes.Buffer{}, root, cfg, registry); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected unmanaged conflict error, got %v", err)
	}
	if err := initRecipes([]string{"--recipe", "coherence", "--force"}, &bytes.Buffer{}, root, cfg, registry); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), managedByHookline) {
		t.Fatal("expected force to replace pre-commit with managed hook")
	}
}

func TestRecipeEnableDisableRTKExplicitProxy(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")

	cfg := config.Default()
	writeRecipePack(t, root, recipes.RTKExplicitProxy)
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := recipeCommand([]string{"enable", "rtk-explicit-proxy", "--json"}, &bytes.Buffer{}, root, cfg, registry); err != nil {
		t.Fatal(err)
	}
	configPath := config.ProjectConfigPath(root)
	if !strings.Contains(readFile(t, configPath), "rtk-explicit-proxy") {
		t.Fatal("expected rtk recipe to be enabled in project config")
	}
	filtersPath := filepath.Join(root, ".rtk", "filters.toml")
	if !strings.Contains(readFile(t, filtersPath), managedByHookline) {
		t.Fatal("expected managed RTK filters file")
	}
	if err := recipeCommand([]string{"disable", "rtk-explicit-proxy", "--prune-managed", "--json"}, &bytes.Buffer{}, root, cfg, registry); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, configPath), "rtk-explicit-proxy") {
		t.Fatal("expected rtk recipe to be removed from config")
	}
	if fileExists(filtersPath) {
		t.Fatal("expected managed RTK filters file to be pruned")
	}
}

func TestRecipeEnableDisableCustomManagedFile(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")

	recipeDir := config.ProjectRecipesPath(root)
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: local-tool
title: Local Tool
surfaces: [init]
managed_files:
  - path: .local-tool/config.txt
    action: write-local-tool-config
    content: |
      # Managed by hookline
      enabled = true
`
	if err := os.WriteFile(filepath.Join(recipeDir, "local-tool.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := recipeCommand([]string{"enable", "local-tool", "--json"}, &bytes.Buffer{}, root, cfg, registry); err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(root, ".local-tool", "config.txt")
	if !strings.Contains(readFile(t, managedPath), "enabled = true") {
		t.Fatal("expected custom managed file content")
	}
	if err := recipeCommand([]string{"disable", "local-tool", "--prune-managed", "--json"}, &bytes.Buffer{}, root, cfg, registry); err != nil {
		t.Fatal(err)
	}
	if fileExists(managedPath) {
		t.Fatal("expected custom managed file to be pruned")
	}
}

func TestRecipeDisableRefusesUnmanagedRTKFile(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")
	if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".rtk", "filters.toml"), []byte("schema_version = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	writeRecipePack(t, root, recipes.RTKExplicitProxy)
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = recipeCommand([]string{"disable", "rtk-explicit-proxy", "--prune-managed"}, &bytes.Buffer{}, root, cfg, registry)
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("expected unmanaged file error, got %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func stubCoherenceInit(t *testing.T) *int {
	t.Helper()
	old := coherenceInitializer
	calls := 0
	coherenceInitializer = func(root string) (initResult, error) {
		calls++
		if err := os.WriteFile(filepath.Join(root, "ontology.yml"), []byte("rules: []\n"), 0o644); err != nil {
			return initResult{}, err
		}
		return initResult{
			RecipeID: recipes.Coherence,
			Scope:    "project",
			Action:   "run-coherence-init",
			Changed:  true,
			Message:  "stub coherence init",
		}, nil
	}
	t.Cleanup(func() {
		coherenceInitializer = old
	})
	return &calls
}

func writeRecipePack(t *testing.T, root string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		switch id {
		case recipes.CodexHooks:
			writeRecipeManifest(t, root, id, `id: codex-hooks
title: Codex Hooks
surfaces: [codex, doctor, init]
`)
		case recipes.Coherence:
			writeRecipeManifest(t, root, id, `id: coherence
title: Coherence
surfaces: [doctor, init, precommit]
commands:
  doctor:
    - args: ["coherence", "doctor", "--json"]
`)
		case recipes.SecretsGitleaks:
			writeRecipeManifest(t, root, id, `id: secrets-gitleaks
title: Secrets and Gitleaks
surfaces: [doctor, init, precommit, scan]
commands:
  doctor:
    - args: ["gitleaks", "version"]
managed_files:
  - path: .gitleaks.toml
    content: |
      # Managed by hookline
`)
		case recipes.LineCount:
			writeRecipeManifest(t, root, id, `id: line-count
title: Line Count
surfaces: [doctor, hook, scan]
`)
		case recipes.AgentSteering:
			writeRecipeManifest(t, root, id, `id: agent-steering
title: Agent Steering
surfaces: [doctor, hook]
`)
		case recipes.RTKExplicitProxy:
			writeRecipeManifest(t, root, id, `id: rtk-explicit-proxy
title: RTK Explicit Proxy
surfaces: [doctor, init]
managed_files:
  - path: .rtk/filters.toml
    content: |
      # Managed by hookline
`)
		default:
			t.Fatalf("unknown test recipe %q", id)
		}
	}
}

func writeRecipeManifest(t *testing.T, root, id, manifest string) {
	t.Helper()
	dir := config.ProjectRecipesPath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}
