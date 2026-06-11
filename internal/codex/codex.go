package codex

import (
	"context"
	"encoding/json"
	"io"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/engine"
	"github.com/fireharp/hookline/internal/types"
)

type hookOutput struct {
	Decision           string         `json:"decision,omitempty"`
	Reason             string         `json:"reason,omitempty"`
	SystemMessage      string         `json:"systemMessage,omitempty"`
	HookSpecificOutput map[string]any `json:"hookSpecificOutput,omitempty"`
}

type Options struct {
	Root string
}

type Result struct {
	Event    types.Event
	Decision types.Decision
}

func Handle(ctx context.Context, stdin io.Reader, stdout io.Writer, cfg config.Config) error {
	_, err := HandleWithOptions(ctx, stdin, stdout, cfg, Options{})
	return err
}

func HandleWithOptions(ctx context.Context, stdin io.Reader, stdout io.Writer, cfg config.Config, opts Options) (Result, error) {
	event, err := Decode(stdin)
	if err != nil {
		return Result{}, err
	}
	return HandleEvent(ctx, event, stdout, cfg, opts)
}

func HandleEvent(ctx context.Context, event types.Event, stdout io.Writer, cfg config.Config, opts Options) (Result, error) {
	root := opts.Root
	if root == "" {
		root = event.CWD
	}
	decisions, err := engine.EvaluateWithRoot(ctx, event, cfg, root)
	if err != nil {
		return Result{Event: event}, err
	}
	decision := strictest(decisions)
	out := encode(event.Event, decision)
	if out == nil {
		return Result{Event: event, Decision: decision}, nil
	}
	enc := json.NewEncoder(stdout)
	return Result{Event: event, Decision: decision}, enc.Encode(out)
}

func Decode(r io.Reader) (types.Event, error) {
	var raw map[string]any
	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return types.Event{}, err
	}
	event := types.Event{
		Agent:          "codex",
		Event:          stringField(raw, "hook_event_name"),
		SessionID:      stringField(raw, "session_id"),
		TurnID:         stringField(raw, "turn_id"),
		CWD:            stringField(raw, "cwd"),
		PermissionMode: stringField(raw, "permission_mode"),
		Prompt:         stringField(raw, "prompt"),
		Raw:            raw,
	}
	event.StopHookActive = boolField(raw, "stop_hook_active")
	event.LastAssistantMessage = stringField(raw, "last_assistant_message")
	if name := stringField(raw, "tool_name"); name != "" {
		input := raw["tool_input"]
		event.Tool = &types.ToolEvent{Name: name, Input: input, Output: raw["tool_response"]}
		if obj, ok := input.(map[string]any); ok {
			event.Tool.Command = stringField(obj, "command")
		}
	}
	return event, nil
}

func encode(eventName string, decision types.Decision) *hookOutput {
	if decision.Mode == types.ModeAllow || decision.Message == "" {
		return nil
	}
	switch eventName {
	case "PreToolUse":
		if decision.Mode == types.ModeBlock {
			return &hookOutput{HookSpecificOutput: map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": decision.Message,
			}}
		}
		return additionalContext("PreToolUse", decision.Message)
	case "UserPromptSubmit":
		if decision.Mode == types.ModeBlock {
			return &hookOutput{Decision: "block", Reason: decision.Message}
		}
		return additionalContext("UserPromptSubmit", decision.Message)
	case "PostToolUse":
		if decision.Mode == types.ModeContinue || decision.Mode == types.ModeBlock {
			return additionalContext("PostToolUse", decision.Message)
		}
		return additionalContext("PostToolUse", decision.Message)
	case "Stop":
		if decision.Mode == types.ModeContinue || decision.Mode == types.ModeBlock {
			return &hookOutput{Decision: "block", Reason: decision.Message}
		}
		return &hookOutput{SystemMessage: decision.Message}
	default:
		return &hookOutput{SystemMessage: decision.Message}
	}
}

func additionalContext(eventName, message string) *hookOutput {
	return &hookOutput{HookSpecificOutput: map[string]any{
		"hookEventName":     eventName,
		"additionalContext": message,
	}}
}

func strictest(decisions []types.Decision) types.Decision {
	best := types.Decision{Mode: types.ModeAllow}
	rank := map[string]int{types.ModeAllow: 0, types.ModeNudge: 1, types.ModeContinue: 2, types.ModeBlock: 3}
	for _, decision := range decisions {
		if rank[decision.Mode] > rank[best.Mode] {
			best = decision
		}
	}
	return best
}

func stringField(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return value
}

func boolField(raw map[string]any, key string) bool {
	value, _ := raw[key].(bool)
	return value
}
