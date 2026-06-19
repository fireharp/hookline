package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestImportCommandWritesRecipeAndInstallsHookline(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"printf imported","timeout":10}]}]}}`
	if err := os.WriteFile(source, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := importCommand([]string{"codex-hooks", "--from", source, "--id", "projectxyz-hooks", "--enable", "--install", "--force", "--json"}, &out, root)
	if err != nil {
		t.Fatal(err)
	}
	recipePath := filepath.Join(config.ProjectRecipesPath(root), "projectxyz-hooks.yaml")
	recipe := readFile(t, recipePath)
	if !strings.Contains(recipe, "codex_hooks:") || !strings.Contains(recipe, "printf imported") {
		t.Fatalf("expected imported recipe, got %s", recipe)
	}
	configData := readFile(t, config.ProjectConfigPath(root))
	if !strings.Contains(configData, "projectxyz-hooks") || !strings.Contains(configData, "codex-hooks") {
		t.Fatalf("expected enabled recipes, got %s", configData)
	}
	installed := readFile(t, source)
	if !strings.Contains(installed, "hook codex --source project") {
		t.Fatalf("expected Hookline Codex hook wiring, got %s", installed)
	}
	if !strings.Contains(installed, "SessionStart") || !strings.Contains(installed, "PermissionRequest") {
		t.Fatalf("expected lifecycle events in installed hooks, got %s", installed)
	}
}

func TestImportCommandRefusesInstallWithoutForce(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"printf imported"}]}]}}`
	if err := os.WriteFile(source, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	err := importCommand([]string{"codex-hooks", "--from", source, "--id", "projectxyz-hooks", "--install"}, &bytes.Buffer{}, root)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
}
