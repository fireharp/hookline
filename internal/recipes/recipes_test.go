package recipes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestLoadBundledRecipesDisabledByDefault(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)

	registry, err := Load(root, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get(LineCount); !ok {
		t.Fatal("expected bundled line-count recipe")
	}
	if registry.AnyEnabled() {
		t.Fatalf("expected no enabled recipes by default, got %#v", registry.EnabledIDs())
	}
}

func TestLoadProjectRecipeOverridesBundledRecipe(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(root, ".fireharp", "hookline", "recipes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "line-count.yaml"), []byte("id: line-count\ntitle: Custom Line Count\nsurfaces: [scan]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Recipes.Enabled = []string{LineCount}
	registry, err := Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	manifest, ok := registry.Get(LineCount)
	if !ok {
		t.Fatal("expected line-count recipe")
	}
	if manifest.Title != "Custom Line Count" {
		t.Fatalf("expected project recipe override, got %#v", manifest)
	}
	if !registry.Enabled(LineCount) {
		t.Fatal("expected line-count to be enabled")
	}
}

func TestLoadMalformedManifestFails(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(root, ".fireharp", "hookline", "recipes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("title: Missing ID\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, config.Default()); err == nil {
		t.Fatal("expected malformed recipe to fail")
	}
}

func TestRunCommandsReportsCommandStatus(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "ok")
	if err := os.WriteFile(bin, []byte("#!/usr/bin/env sh\necho recipe-ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{ID: "local", Commands: map[string][]Command{
		"doctor": {{Args: []string{bin}}},
	}}
	results := RunCommands(context.Background(), root, manifest, "doctor")
	if len(results) != 1 || !results[0].OK || results[0].Output != "recipe-ok" {
		t.Fatalf("expected successful command result, got %#v", results)
	}
}
