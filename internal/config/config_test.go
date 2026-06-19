package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesDefaultsUserAndProjectConfig(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)

	userDir := filepath.Dir(UserConfigPath(home))
	projectDir := filepath.Dir(ProjectConfigPath(root))
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(UserConfigPath(home), []byte("telemetry:\n  path: custom/events.jsonl\nlimits:\n  file_line_limit: 450\nsecrets:\n  min_value_length: 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProjectConfigPath(root), []byte("hooks:\n  enabled: false\nlimits:\n  file_line_limit: 321\n  split_review_line_limit: 1500\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.FileLineLimit != 321 {
		t.Fatalf("expected project config to override user config, got %d", cfg.Limits.FileLineLimit)
	}
	if cfg.Secrets.MinValueLength != 10 {
		t.Fatalf("expected user config to fill unset project value, got %d", cfg.Secrets.MinValueLength)
	}
	if cfg.Limits.LargeDiffAdded != 700 {
		t.Fatalf("expected default value to remain, got %d", cfg.Limits.LargeDiffAdded)
	}
	if cfg.Limits.SplitReviewLineLimit != 1500 {
		t.Fatalf("expected project split review limit, got %d", cfg.Limits.SplitReviewLineLimit)
	}
	if cfg.Telemetry.Path != "custom/events.jsonl" {
		t.Fatalf("expected user telemetry path, got %s", cfg.Telemetry.Path)
	}
	if configEnabled := HooksEnabled(cfg); configEnabled {
		t.Fatal("expected project config to disable hooks")
	}
	if len(cfg.Recipes.Enabled) != 0 {
		t.Fatalf("expected recipes to be disabled by default, got %#v", cfg.Recipes.Enabled)
	}
}

func TestLoadMergesRecipeConfig(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Dir(ProjectConfigPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProjectConfigPath(root), []byte("recipes:\n  enabled:\n    - line-count\n  paths:\n    - .harness/recipes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Recipes.Enabled) != 1 || cfg.Recipes.Enabled[0] != "line-count" {
		t.Fatalf("expected enabled line-count recipe, got %#v", cfg.Recipes.Enabled)
	}
	if len(cfg.Recipes.Paths) != 1 || cfg.Recipes.Paths[0] != ".harness/recipes" {
		t.Fatalf("expected recipe path, got %#v", cfg.Recipes.Paths)
	}
}

func TestRootHarnessOverridesLegacyHarnessDirConfig(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Dir(LegacyProjectConfigPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LegacyProjectConfigPath(root), []byte("limits:\n  file_line_limit: 450\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProjectConfigPath(root), []byte("limits:\n  file_line_limit: 321\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.FileLineLimit != 321 {
		t.Fatalf("expected root harness.yaml to override legacy config, got %d", cfg.Limits.FileLineLimit)
	}
}
