package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestPreToolUseDangerousCommandDenies(t *testing.T) {
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
	if err := Handle(context.Background(), strings.NewReader(input), &out, config.Default()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "temporal-workflow-design") {
		t.Fatalf("expected skill context, got %s", out.String())
	}
}
