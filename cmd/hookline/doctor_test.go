package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
)

func TestDoctorPassesWithEnabledRecipeSetup(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")
	mustGit(t, root, "config", "core.hooksPath", ".githooks")
	fakeBin(t, home, "coherence")
	fakeBin(t, home, "gitleaks")
	t.Setenv("PATH", filepath.Join(home, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := os.MkdirAll(filepath.Join(root, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".githooks", "pre-commit"), []byte("#!/usr/bin/env sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "hooks.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Recipes.Enabled = recipes.StandardRecipeIDs()
	writeRecipePack(t, root, cfg.Recipes.Enabled...)
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := doctor(context.Background(), &out, root, cfg, registry, doctorOptions{JSON: true}); err != nil {
		t.Fatal(err)
	}
	var result doctorResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("expected doctor pass, got %#v", result)
	}
}

func TestDoctorFailsMissingHooksPathForPrecommitRecipe(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	mustGit(t, root, "init")
	fakeBin(t, home, "gitleaks")
	t.Setenv("PATH", filepath.Join(home, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Join(root, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".githooks", "pre-commit"), []byte("#!/usr/bin/env sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Recipes.Enabled = []string{recipes.SecretsGitleaks}
	writeRecipePack(t, root, cfg.Recipes.Enabled...)
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := doctor(context.Background(), &bytes.Buffer{}, root, cfg, registry, doctorOptions{}); err == nil {
		t.Fatal("expected doctor to fail without core.hooksPath")
	}
}

func fakeBin(t *testing.T, home, name string) {
	t.Helper()
	dir := filepath.Join(home, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/usr/bin/env sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGitConfigReadsHooksPath(t *testing.T) {
	root := t.TempDir()
	mustGit(t, root, "init")
	cmd := exec.Command("git", "config", "core.hooksPath", ".githooks")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	got, err := gitConfig(root, "core.hooksPath")
	if err != nil {
		t.Fatal(err)
	}
	if got != ".githooks" {
		t.Fatalf("expected .githooks, got %q", got)
	}
}
