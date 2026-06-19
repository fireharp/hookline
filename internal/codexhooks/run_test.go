package codexhooks

import (
	"context"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/recipes"
	"github.com/fireharp/hookline/internal/types"
)

func TestRunMatchingRunsMatchedHookWithInput(t *testing.T) {
	hooks := []recipes.CodexHook{{
		Event:   "PreToolUse",
		Matcher: "^Bash$",
		Type:    "command",
		Command: `read input; case "$input" in *PreToolUse*) printf '{"systemMessage":"matched"}';; *) exit 2;; esac`,
		Timeout: 5,
	}}
	event := types.Event{
		Event: "PreToolUse",
		CWD:   t.TempDir(),
		Tool:  &types.ToolEvent{Name: "Bash"},
		Raw:   map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash"},
	}
	results := RunMatching(context.Background(), hooks, event, []byte(`{"hook_event_name":"PreToolUse"}`), "", nil)
	if len(results) != 1 {
		t.Fatalf("expected one result, got %#v", results)
	}
	if results[0].Error != "" {
		t.Fatalf("expected hook success, got %s", results[0].Error)
	}
	if !strings.Contains(string(results[0].Output), "matched") {
		t.Fatalf("expected hook output, got %q", results[0].Output)
	}
}

func TestRunMatchingHonorsSessionStartMatcher(t *testing.T) {
	hooks := []recipes.CodexHook{{
		Event:   "SessionStart",
		Matcher: "^startup$",
		Type:    "command",
		Command: `printf startup`,
		Timeout: 5,
	}}
	event := types.Event{
		Event: "SessionStart",
		CWD:   t.TempDir(),
		Raw:   map[string]any{"hook_event_name": "SessionStart", "source": "resume"},
	}
	results := RunMatching(context.Background(), hooks, event, []byte(`{}`), "", nil)
	if len(results) != 0 {
		t.Fatalf("expected no match, got %#v", results)
	}
	event.Raw["source"] = "startup"
	results = RunMatching(context.Background(), hooks, event, []byte(`{}`), "", nil)
	if len(results) != 1 {
		t.Fatalf("expected startup match, got %#v", results)
	}
}
