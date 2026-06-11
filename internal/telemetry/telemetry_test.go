package telemetry

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildRecordReducesHookIO(t *testing.T) {
	input := []byte(`{"hook_event_name":"PostToolUse","cwd":"/repo","tool_name":"Edit","tool_input":{"command":"secret"}}`)
	output := []byte(`{"decision":"block","reason":"Touched very large file"}`)

	record := BuildRecord("/repo", input, output, nil, 12*time.Millisecond)

	if record.Event != "PostToolUse" || record.Tool != "Edit" || record.Decision != "block" {
		t.Fatalf("unexpected record: %#v", record)
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
