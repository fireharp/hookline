package telemetry

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildRecordReducesHookIO(t *testing.T) {
	input := []byte(`{"hook_event_name":"PostToolUse","cwd":"/repo","session_id":"s1","turn_id":"t1","tool_name":"Edit","tool_input":{"command":"secret"}}`)
	output := []byte(`{"decision":"block","reason":"Touched very large file"}`)

	record := BuildRecord("/repo", input, output, nil, 12*time.Millisecond, Meta{Source: "project", RuleID: "large-file-split-review", Snoozed: true, SnoozeScope: "session", SnoozePath: "huge.ts"})

	if record.Event != "PostToolUse" || record.Tool != "Edit" || record.Decision != "block" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if record.SessionID != "s1" || record.TurnID != "t1" || record.Source != "project" || record.RuleID != "large-file-split-review" || !record.Snoozed {
		t.Fatalf("missing hook metadata: %#v", record)
	}
	if strings.Contains(record.Reason, "secret") {
		t.Fatalf("record leaked raw input: %#v", record)
	}
}

func TestBuildRecordCapturesError(t *testing.T) {
	record := BuildRecord("/repo", []byte(`{"hook_event_name":"Stop"}`), nil, errors.New("boom"), 0)
	if record.Error != "boom" || !record.OutputEmpty {
		t.Fatalf("unexpected record: %#v", record)
	}
}
