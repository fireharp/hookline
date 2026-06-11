package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserSourceSuppressedWhenProjectHookTagged(t *testing.T) {
	root := setupHookRepo(t, true)
	t.Chdir(root)
	input := hookInput(root, "s1", "t1")

	var userOut bytes.Buffer
	if err := run(context.Background(), []string{"hook", "codex", "--source", "user"}, strings.NewReader(input), &userOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if userOut.Len() != 0 {
		t.Fatalf("expected user hook to suppress itself, got %s", userOut.String())
	}

	var projectOut bytes.Buffer
	if err := run(context.Background(), []string{"hook", "codex", "--source", "project"}, strings.NewReader(input), &projectOut, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projectOut.String(), "Touched very large file") {
		t.Fatalf("expected project hook output, got %s", projectOut.String())
	}
}

func TestLegacyDuplicateSuppressedByDedupe(t *testing.T) {
	root := setupHookRepo(t, false)
	t.Chdir(root)
	input := hookInput(root, "s1", "t1")

	var first bytes.Buffer
	if err := run(context.Background(), []string{"hook", "codex", "--source", "auto"}, strings.NewReader(input), &first, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if first.Len() == 0 {
		t.Fatal("expected first legacy hook to emit output")
	}
	var second bytes.Buffer
	if err := run(context.Background(), []string{"hook", "codex", "--source", "auto"}, strings.NewReader(input), &second, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if second.Len() != 0 {
		t.Fatalf("expected duplicate legacy hook to be suppressed, got %s", second.String())
	}
}

func setupHookRepo(t *testing.T, taggedProjectHook bool) string {
	t.Helper()
	root := t.TempDir()
	mustGit(t, root, "init")
	mustGit(t, root, "config", "user.email", "hookline@example.test")
	mustGit(t, root, "config", "user.name", "Hookline")
	mustGit(t, root, "config", "commit.gpgsign", "false")
	writeTestFile(t, root, ".fireharp/harness.yaml", "recipes:\n  enabled:\n    - codex-hooks\n    - line-count\nlimits:\n  split_review_line_limit: 5\n  large_diff_added: 1000\n")
	writeTestFile(t, root, "huge.ts", "a\nb\nc\nd\ne\n")
	mustGit(t, root, "add", ".")
	mustGit(t, root, "commit", "-m", "init")
	writeTestFile(t, root, "huge.ts", "a\nb\nc\nd\ne\n// harmless comment\n")
	if taggedProjectHook {
		if _, err := writeHookFile(filepath.Join(root, ".codex", "hooks.json"), "hookline hook codex --source project", true); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func hookInput(root, session, turn string) string {
	return `{
		"hook_event_name": "Stop",
		"cwd": "` + root + `",
		"session_id": "` + session + `",
		"turn_id": "` + turn + `",
		"stop_hook_active": false
	}`
}

func writeTestFile(t *testing.T, root, path, contents string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
