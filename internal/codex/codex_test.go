package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
)

func TestPreToolUseDangerousCommandDenies(t *testing.T) {
	input := `{
		"hook_event_name": "PreToolUse",
		"cwd": ".",
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf /"}
	}`
	var out bytes.Buffer
	if err := Handle(context.Background(), strings.NewReader(input), &out, testConfig(recipes.AgentSteering)); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	hookOutput := decoded["hookSpecificOutput"].(map[string]any)
	if hookOutput["permissionDecision"] != "deny" {
		t.Fatalf("expected deny, got %#v", hookOutput)
	}
}

func TestUserPromptSubmitAddsSkillContext(t *testing.T) {
	input := `{
		"hook_event_name": "UserPromptSubmit",
		"cwd": ".",
		"prompt": "Build a Temporal workflow"
	}`
	var out bytes.Buffer
	if err := Handle(context.Background(), strings.NewReader(input), &out, testConfig(recipes.AgentSteering)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "temporal-workflow-design") {
		t.Fatalf("expected skill context, got %s", out.String())
	}
}

func TestStopAllowsSoftLineCountFinding(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.md"), []byte(strings.Repeat("line\n", 4)), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Limits.FileLineLimit = 3
	input := `{
		"hook_event_name": "Stop",
		"cwd": "` + root + `",
		"stop_hook_active": false
	}`
	var out bytes.Buffer
	if err := Handle(context.Background(), strings.NewReader(input), &out, cfg); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected soft line-count finding not to force Stop continuation, got %s", out.String())
	}
}

func TestPostToolUseAddsContextWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init")
	git(t, root, "config", "user.email", "hookline@example.test")
	git(t, root, "config", "user.name", "Hookline")
	git(t, root, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "huge.ts"), []byte(strings.Repeat("line\n", 6)), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "huge.ts")
	git(t, root, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(root, "huge.ts"), []byte(strings.Repeat("line\n", 6)+"// comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Recipes.Enabled = []string{recipes.LineCount}
	cfg.Limits.SplitReviewLineLimit = 5
	cfg.Limits.LargeDiffAdded = 1000
	input := `{
		"hook_event_name": "PostToolUse",
		"cwd": "` + root + `",
		"tool_name": "Edit"
	}`
	var out bytes.Buffer
	if err := Handle(context.Background(), strings.NewReader(input), &out, cfg); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["decision"] != nil {
		t.Fatalf("expected PostToolUse context not block, got %s", out.String())
	}
	hookOutput := decoded["hookSpecificOutput"].(map[string]any)
	if !strings.Contains(hookOutput["additionalContext"].(string), "very large file") {
		t.Fatalf("expected large-file context, got %#v", hookOutput)
	}
}

func TestStopRequestsContinuationForNewTODO(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init")
	git(t, root, "config", "user.email", "hookline@example.test")
	git(t, root, "config", "user.name", "Hookline")
	git(t, root, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "notes.md")
	git(t, root, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("done\nTODO: follow up\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := `{
		"hook_event_name": "Stop",
		"cwd": "` + root + `",
		"stop_hook_active": false
	}`
	var out bytes.Buffer
	if err := Handle(context.Background(), strings.NewReader(input), &out, testConfig(recipes.AgentSteering)); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["decision"] != "block" {
		t.Fatalf("expected Codex continuation decision, got %#v", decoded)
	}
	if !strings.Contains(decoded["reason"].(string), "new TODO/FIXME added") {
		t.Fatalf("expected TODO reason, got %#v", decoded["reason"])
	}
}

func TestDefaultConfigHasNoHookBehavior(t *testing.T) {
	input := `{
		"hook_event_name": "PreToolUse",
		"cwd": ".",
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf /"}
	}`
	var out bytes.Buffer
	if err := Handle(context.Background(), strings.NewReader(input), &out, config.Default()); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output without enabled recipes, got %s", out.String())
	}
}

func testConfig(ids ...string) config.Config {
	cfg := config.Default()
	cfg.Recipes.Enabled = ids
	return cfg
}

func TestStopHookActiveSkipsSecondContinuation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.md"), []byte(strings.Repeat("line\n", 4)), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Limits.FileLineLimit = 3
	input := `{
		"hook_event_name": "Stop",
		"cwd": "` + root + `",
		"stop_hook_active": true
	}`
	var out bytes.Buffer
	if err := Handle(context.Background(), strings.NewReader(input), &out, cfg); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no second continuation, got %s", out.String())
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
