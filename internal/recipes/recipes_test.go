package recipes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestLoadProjectRecipesDisabledByDefault(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	writeRecipe(t, root, "line-count", "id: line-count\ntitle: Line Count\nsurfaces: [scan]\n")
	writeRecipe(t, root, "rtk-explicit-proxy", "id: rtk-explicit-proxy\ntitle: RTK Explicit Proxy\nmanaged_files:\n  - path: .rtk/filters.toml\n    content: |\n      # Managed by hookline\n")

	registry, err := Load(root, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get(LineCount); !ok {
		t.Fatal("expected project line-count recipe")
	}
	if registry.AnyEnabled() {
		t.Fatalf("expected no enabled recipes by default, got %#v", registry.EnabledIDs())
	}
	if _, ok := registry.Get(RTKExplicitProxy); !ok {
		t.Fatal("expected project rtk-explicit-proxy recipe")
	}
	rtk, _ := registry.Get(RTKExplicitProxy)
	if len(rtk.ManagedFiles) != 1 || rtk.ManagedFiles[0].Path != filepath.Join(".rtk", "filters.toml") {
		t.Fatalf("expected RTK managed file from manifest, got %#v", rtk.ManagedFiles)
	}
	for _, id := range StandardRecipeIDs() {
		if id == RTKExplicitProxy {
			t.Fatal("expected rtk-explicit-proxy to stay out of standard recipes")
		}
	}
}

func TestLoadIncludesStandardRecipesWithoutProjectPack(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)

	registry, err := Load(root, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range StandardRecipeIDs() {
		if _, ok := registry.Get(id); !ok {
			t.Fatalf("expected standard recipe %s", id)
		}
	}
	if registry.AnyEnabled() {
		t.Fatalf("expected standard recipes loaded but disabled, got %#v", registry.EnabledIDs())
	}
}

func writeRecipe(t *testing.T, root, id, manifest string) {
	t.Helper()
	dir := config.ProjectRecipesPath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectRecipeOverridesUserRecipe(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	dir := config.ProjectRecipesPath(root)
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
	dir := config.ProjectRecipesPath(root)
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

func TestLoadRejectsUnsafeManagedFilePath(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	dir := config.ProjectRecipesPath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("id: bad\nmanaged_files:\n  - path: ../outside\n    content: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, config.Default()); err == nil {
		t.Fatal("expected unsafe managed file path to fail")
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
