package codexhooks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/fireharp/hookline/internal/recipes"
	"github.com/fireharp/hookline/internal/types"
)

type RunResult struct {
	Hook   recipes.CodexHook
	Output []byte
	Error  string
}

func EnabledHooks(registry recipes.Registry) []recipes.CodexHook {
	var hooks []recipes.CodexHook
	for _, manifest := range registry.EnabledManifests() {
		hooks = append(hooks, manifest.CodexHooks...)
	}
	return hooks
}

func RunMatching(ctx context.Context, hooks []recipes.CodexHook, event types.Event, input []byte, root string, stderr io.Writer) []RunResult {
	var results []RunResult
	for _, hook := range hooks {
		if !matches(hook, event) {
			continue
		}
		results = append(results, runHook(ctx, hook, input, event.CWD, root, stderr))
	}
	return results
}

func Outputs(results []RunResult) [][]byte {
	var outputs [][]byte
	for _, result := range results {
		if len(bytes.TrimSpace(result.Output)) > 0 {
			outputs = append(outputs, result.Output)
		}
	}
	return outputs
}

func runHook(ctx context.Context, hook recipes.CodexHook, input []byte, cwd, root string, stderr io.Writer) RunResult {
	command := hook.Command
	if runtime.GOOS == "windows" && hook.CommandWindows != "" {
		command = hook.CommandWindows
	}
	timeout := time.Duration(hook.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := shellCommand(runCtx, command)
	cmd.Dir = cwd
	if cmd.Dir == "" {
		cmd.Dir = root
	}
	cmd.Stdin = bytes.NewReader(input)
	var stdout, hookStderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &hookStderr
	err := cmd.Run()
	if hookStderr.Len() > 0 && stderr != nil {
		fmt.Fprintf(stderr, "hookline imported hook stderr (%s): %s\n", hook.Event, strings.TrimSpace(hookStderr.String()))
	}
	result := RunResult{Hook: hook, Output: stdout.Bytes()}
	if err != nil {
		result.Error = err.Error()
		if runCtx.Err() == context.DeadlineExceeded {
			result.Error = "timeout"
		}
		if stderr != nil {
			fmt.Fprintf(stderr, "hookline imported hook failed (%s): %s\n", hook.Event, result.Error)
		}
	}
	return result
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

func matches(hook recipes.CodexHook, event types.Event) bool {
	if hook.Event != event.Event {
		return false
	}
	matcher := strings.TrimSpace(hook.Matcher)
	if matcher == "" || matcher == "*" || eventIgnoresMatcher(event.Event) {
		return true
	}
	target := matcherTarget(event)
	ok, err := regexp.MatchString(matcher, target)
	return err == nil && ok
}

func eventIgnoresMatcher(eventName string) bool {
	return eventName == "Stop" || eventName == "UserPromptSubmit"
}

func matcherTarget(event types.Event) string {
	if event.Tool != nil {
		return event.Tool.Name
	}
	switch event.Event {
	case "SessionStart":
		return firstString(event.Raw, "source", "session_start_source", "start_source")
	case "PreCompact", "PostCompact":
		return firstString(event.Raw, "trigger", "compact_trigger")
	case "SubagentStart", "SubagentStop":
		return firstString(event.Raw, "subagent_type", "agent_type", "type")
	default:
		return ""
	}
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok {
			return value
		}
	}
	return ""
}
