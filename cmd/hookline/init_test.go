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

func TestParseInitOptionsRequiresRecipe(t *testing.T) {
	if _, err := parseInitOptions([]string{"--scope", "project"}); err == nil || !strings.Contains(err.Error(), "--recipe") {
		t.Fatalf("expected recipe error, got %v", err)
	}
}

func TestInitRecipesWritesPrecommitAndConfiguresGitHooksPath(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")

	cfg := config.Default()
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
	if !strings.Contains(readFile(t, filepath.Join(root, ".fireharp", "harness.yaml")), "secrets-gitleaks") {
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
}

func TestInitRecipesRefusesUnmanagedPrecommitConflict(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")
	path := filepath.Join(root, ".githooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/usr/bin/env sh\necho custom\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
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
