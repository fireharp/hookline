package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
	"github.com/fireharp/hookline/internal/types"
)

func TestAddedTodoLinesDetectsProjectChanges(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "notes.md", "done\n")
	git(t, root, "add", "notes.md")
	git(t, root, "commit", "-m", "init")

	writeFile(t, root, "notes.md", "done\nTODO: follow up\n")

	if !addedTodoLines(root) {
		t.Fatal("expected TODO in normal project file to trigger")
	}
}

func TestAddedTodoLinesIgnoresIdentifierText(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "helper.go", "package main\n")
	git(t, root, "add", "helper.go")
	git(t, root, "commit", "-m", "init")

	writeFile(t, root, "helper.go", "package main\nfunc todoMarkerPatternName() {}\n")

	if addedTodoLines(root) {
		t.Fatal("expected todo inside an identifier not to trigger")
	}
}

func TestAddedTodoLinesIgnoresEvidenceAndFixturePaths(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "internal/bench/bench.go", "package bench\n")
	writeFile(t, root, "internal/codex/codex_test.go", "package codex\n")
	writeFile(t, root, "docs/cases/example.mdx", "---\ntitle: example\n---\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "init")

	writeFile(t, root, "internal/bench/bench.go", "package bench\nvar fixture = \"TODO: follow up\"\n")
	writeFile(t, root, "internal/codex/codex_test.go", "package codex\nvar fixture = \"TODO: follow up\"\n")
	writeFile(t, root, "docs/cases/example.mdx", "---\ntitle: example\n---\nTODO: follow up\n")

	if addedTodoLines(root) {
		t.Fatal("expected TODO in evidence and fixture paths to be ignored")
	}
}

func TestPostToolUseContinuesForTouchedHugeCodeFile(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "group_runner.ts", "a\nb\nc\nd\ne\n")
	git(t, root, "add", "group_runner.ts")
	git(t, root, "commit", "-m", "init")
	writeFile(t, root, "group_runner.ts", "a\nb\nc\nd\ne\n// harmless comment\n")

	cfg := config.Default()
	cfg.Recipes.Enabled = []string{recipes.LineCount}
	cfg.Limits.SplitReviewLineLimit = 5
	cfg.Limits.LargeDiffAdded = 1000
	decisions, err := Evaluate(context.Background(), types.Event{Event: "PostToolUse", CWD: root}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) == 0 || decisions[0].RuleID != "large-file-split-review" {
		t.Fatalf("expected large-file split review, got %#v", decisions)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init")
	git(t, root, "config", "user.email", "hookline@example.test")
	git(t, root, "config", "user.name", "Hookline")
	git(t, root, "config", "commit.gpgsign", "false")
	return root
}

func writeFile(t *testing.T, root, path, contents string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
