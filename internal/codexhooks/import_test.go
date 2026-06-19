package codexhooks

import (
	"strings"
	"testing"
)

func TestImportCodexHooksJSON(t *testing.T) {
	data := []byte(`{
	  "hooks": {
	    "SessionStart": [{
	      "matcher": "^startup$",
	      "hooks": [{
	        "type": "command",
	        "command": "python3 ./.codex/hooks/scripts/hooks.py --hook SessionStart",
	        "timeout": 10,
	        "statusMessage": "Loading project context"
	      }]
	    }],
	    "PreToolUse": [{
	      "hooks": [{
	        "type": "prompt",
	        "command": "ignored"
	      }, {
	        "type": "command",
	        "command": "python3 ./.codex/hooks/scripts/hooks.py --hook PreToolUse",
	        "async": true
	      }]
	    }]
	  }
	}`)
	result, err := Import(data, "project-hooks", "", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected one imported hook, got %d", result.Imported)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("expected two skipped hooks, got %#v", result.Skipped)
	}
	hook := result.Manifest.CodexHooks[0]
	if hook.Event != "SessionStart" || hook.Matcher != "^startup$" || hook.Timeout != 10 {
		t.Fatalf("unexpected imported hook: %#v", hook)
	}
	if hook.StatusMessage != "Loading project context" {
		t.Fatalf("expected status message, got %q", hook.StatusMessage)
	}
}

func TestImportCodexHooksYAML(t *testing.T) {
	data := []byte(`
hooks:
  PostToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: ./hook.sh
          status_message: Reviewing
          command_windows: hook.bat
`)
	result, err := Import(data, "yaml-hooks", "", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	hook := result.Manifest.CodexHooks[0]
	if hook.Command != "./hook.sh" || hook.CommandWindows != "hook.bat" || hook.StatusMessage != "Reviewing" {
		t.Fatalf("unexpected YAML import: %#v", hook)
	}
}

func TestDefaultIDUsesProjectNameForDotCodexPath(t *testing.T) {
	got := DefaultID("/tmp/ProjectXYZ/.codex/hooks.json")
	if !strings.Contains(got, "projectxyz-hooks") {
		t.Fatalf("expected project-based id, got %q", got)
	}
}
