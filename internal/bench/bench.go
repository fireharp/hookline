package bench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/engine"
	"github.com/fireharp/hookline/internal/lines"
	"github.com/fireharp/hookline/internal/secrets"
	"github.com/fireharp/hookline/internal/types"
)

type Result struct {
	Suite     string     `json:"suite"`
	Pass      bool       `json:"pass"`
	Scenarios []Scenario `json:"scenarios"`
}

type Scenario struct {
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Resolved    bool     `json:"resolved"`
	Start       string   `json:"start,omitempty"`
	Hookline    string   `json:"hookline,omitempty"`
	AgentAction string   `json:"agent_action,omitempty"`
	Result      string   `json:"result,omitempty"`
	Message     string   `json:"message,omitempty"`
	Evidence    Evidence `json:"evidence,omitempty"`
}

type Evidence struct {
	CaseID        string       `json:"case_id,omitempty"`
	Project       string       `json:"project,omitempty"`
	Fixture       string       `json:"fixture,omitempty"`
	Rule          string       `json:"rule,omitempty"`
	InitialState  []string     `json:"initial_state,omitempty"`
	Communication []Message    `json:"communication,omitempty"`
	Hook          HookEvidence `json:"hook,omitempty"`
	FinalState    []string     `json:"final_state,omitempty"`
	Verification  []string     `json:"verification,omitempty"`
}

type Message struct {
	Actor   string `json:"actor"`
	Message string `json:"message"`
}

type HookEvidence struct {
	Event  string `json:"event,omitempty"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	Effect string `json:"effect,omitempty"`
}

func Run(ctx context.Context, suite string, cfg config.Config) (Result, error) {
	if suite == "" {
		suite = "smoke"
	}
	result := Result{Suite: suite, Pass: true}
	checks := []func(context.Context, config.Config) Scenario{
		lineCountSoftReview,
		envLeak,
		dangerousBash,
		largeDiff,
		sensitivePath,
		stopContinuation,
	}
	for _, check := range checks {
		scenario := check(ctx, cfg)
		if scenario.Status != "pass" {
			result.Pass = false
		}
		result.Scenarios = append(result.Scenarios, scenario)
	}
	return result, nil
}

func lineCountSoftReview(ctx context.Context, cfg config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("line-count-soft-review", err.Error())
	}
	defer cleanup()
	path := filepath.Join(root, "cohesive-reference.md")
	if err := os.WriteFile(path, []byte(linesText(cfg.Limits.FileLineLimit+1)), 0o644); err != nil {
		return fail("line-count-soft-review", err.Error())
	}
	result, err := lines.Scan(root, cfg.Limits)
	if err != nil {
		return fail("line-count-soft-review", err.Error())
	}
	if len(result.Findings) != 1 {
		return fail("line-count-soft-review", "expected one advisory line finding")
	}
	decisions, err := engine.Evaluate(ctx, types.Event{Event: "Stop", CWD: root}, cfg)
	if err != nil {
		return fail("line-count-soft-review", err.Error())
	}
	if decisions[0].Mode != types.ModeAllow {
		return fail("line-count-soft-review", "expected soft line finding not to force Stop continuation")
	}
	return resolved(
		"line-count-soft-review",
		"Created cohesive-reference.md with 501 lines against the 500-line soft target.",
		"Line scan reported an advisory finding, not a hard violation.",
		"Agent reviewed the file shape and kept it together because it was cohesive.",
		"Stop evaluation was allowed; the line-count finding remained advisory evidence.",
		lineCountSoftReviewEvidence(),
	)
}

func envLeak(_ context.Context, cfg config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("env-leak", err.Error())
	}
	defer cleanup()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("LOCAL_PROJECT_ROOT=/Users/myuser/Work/private-client\n"), 0o600); err != nil {
		return fail("env-leak", err.Error())
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("project_root=/Users/myuser/Work/private-client\n"), 0o644); err != nil {
		return fail("env-leak", err.Error())
	}
	if err := git(root, "add", "README.md"); err != nil {
		return fail("env-leak", err.Error())
	}
	leaks, err := secrets.EnvLeaks(root, cfg.Secrets)
	if err != nil {
		return fail("env-leak", err.Error())
	}
	if len(leaks) == 0 || strings.Contains(leaks[0].Redacted, "/Users/myuser/Work/private-client") {
		return fail("env-leak", "expected redacted env leak")
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("project_root=${LOCAL_PROJECT_ROOT}\n"), 0o644); err != nil {
		return fail("env-leak", err.Error())
	}
	if err := git(root, "add", "README.md"); err != nil {
		return fail("env-leak", err.Error())
	}
	leaks, err = secrets.EnvLeaks(root, cfg.Secrets)
	if err != nil {
		return fail("env-leak", err.Error())
	}
	if len(leaks) != 0 {
		return fail("env-leak", "expected placeholder rewrite to remove env leak")
	}
	return resolved(
		"env-leak",
		"Staged README.md containing a literal value from .env.",
		"Secret guard reported only LOCAL_PROJECT_ROOT with a redacted path summary.",
		"Agent replaced the literal with ${LOCAL_PROJECT_ROOT} and staged the rewrite.",
		"Secret guard passed with no env value leaks.",
		envLeakEvidence(),
	)
}

func dangerousBash(ctx context.Context, cfg config.Config) Scenario {
	event := types.Event{
		Event: "PreToolUse",
		CWD:   ".",
		Tool:  &types.ToolEvent{Name: "Bash", Command: "rm -rf /"},
	}
	decisions, err := engine.Evaluate(ctx, event, cfg)
	if err != nil {
		return fail("dangerous-bash", err.Error())
	}
	if decisions[0].Mode != types.ModeBlock {
		return fail("dangerous-bash", "expected block")
	}
	event.Tool.Command = "printf cleanup-reviewed"
	decisions, err = engine.Evaluate(ctx, event, cfg)
	if err != nil {
		return fail("dangerous-bash", err.Error())
	}
	if decisions[0].Mode != types.ModeAllow {
		return fail("dangerous-bash", "expected safe replacement command")
	}
	return resolved(
		"dangerous-bash",
		"Agent attempted rm -rf / through a Bash tool call.",
		"PreToolUse denied the destructive command before it ran.",
		"Agent switched to a harmless command after the denial.",
		"Replacement command was allowed by the rule engine.",
		dangerousBashEvidence(),
	)
}

func largeDiff(ctx context.Context, cfg config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("large-diff", err.Error())
	}
	defer cleanup()
	if err := git(root, "config", "user.email", "hookline@example.test"); err != nil {
		return fail("large-diff", err.Error())
	}
	if err := git(root, "config", "user.name", "Hookline"); err != nil {
		return fail("large-diff", err.Error())
	}
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("start\n"), 0o644); err != nil {
		return fail("large-diff", err.Error())
	}
	if err := git(root, "add", "notes.md"); err != nil {
		return fail("large-diff", err.Error())
	}
	if err := git(root, "commit", "-m", "init"); err != nil {
		return fail("large-diff", err.Error())
	}
	if err := os.WriteFile(path, []byte("start\n"+linesText(cfg.Limits.LargeDiffAdded+1)), 0o644); err != nil {
		return fail("large-diff", err.Error())
	}
	decisions, err := engine.Evaluate(ctx, types.Event{Event: "PostToolUse", CWD: root}, cfg)
	if err != nil {
		return fail("large-diff", err.Error())
	}
	if !hasRule(decisions, "large-diff") {
		return fail("large-diff", "expected large diff steering")
	}
	if err := os.WriteFile(path, []byte("start\n"+linesText(20)), 0o644); err != nil {
		return fail("large-diff", err.Error())
	}
	decisions, err = engine.Evaluate(ctx, types.Event{Event: "PostToolUse", CWD: root}, cfg)
	if err != nil {
		return fail("large-diff", err.Error())
	}
	if hasRule(decisions, "large-diff") {
		return fail("large-diff", "expected reduced diff to clear large diff steering")
	}
	return resolved(
		"large-diff",
		"Agent produced a 701-line diff against a tracked Markdown file.",
		"PostToolUse returned continuation feedback to review accidental rewrites.",
		"Agent reduced the edit to a focused 20-line change.",
		"PostToolUse passed without a large-diff decision.",
		largeDiffEvidence(),
	)
}

func sensitivePath(ctx context.Context, cfg config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("sensitive-path", err.Error())
	}
	defer cleanup()
	if err := os.MkdirAll(filepath.Join(root, "services", "billing"), 0o755); err != nil {
		return fail("sensitive-path", err.Error())
	}
	if err := os.WriteFile(filepath.Join(root, "services", "billing", "pay.go"), []byte("package billing\n"), 0o644); err != nil {
		return fail("sensitive-path", err.Error())
	}
	decisions, err := engine.Evaluate(ctx, types.Event{Event: "PostToolUse", CWD: root}, cfg)
	if err != nil {
		return fail("sensitive-path", err.Error())
	}
	for _, decision := range decisions {
		if decision.RuleID == "sensitive-path" {
			if err := os.WriteFile(filepath.Join(root, "services", "billing", "pay_test.go"), []byte("package billing\n"), 0o644); err != nil {
				return fail("sensitive-path", err.Error())
			}
			decisions, err = engine.Evaluate(ctx, types.Event{Event: "PostToolUse", CWD: root}, cfg)
			if err != nil {
				return fail("sensitive-path", err.Error())
			}
			if hasRule(decisions, "sensitive-path") {
				return fail("sensitive-path", "expected test file to clear sensitive path nudge")
			}
			return resolved(
				"sensitive-path",
				"Agent changed services/billing/pay.go without a test change.",
				"PostToolUse added context about auth, secrets, idempotency, and regression coverage.",
				"Agent added services/billing/pay_test.go.",
				"Sensitive-path check passed because the sensitive change now has test coverage.",
				sensitivePathEvidence(),
			)
		}
	}
	return fail("sensitive-path", "expected sensitive path nudge")
}

func stopContinuation(ctx context.Context, cfg config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("stop-continuation", err.Error())
	}
	defer cleanup()
	if err := git(root, "config", "user.email", "hookline@example.test"); err != nil {
		return fail("stop-continuation", err.Error())
	}
	if err := git(root, "config", "user.name", "Hookline"); err != nil {
		return fail("stop-continuation", err.Error())
	}
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("done\n"), 0o644); err != nil {
		return fail("stop-continuation", err.Error())
	}
	if err := git(root, "add", "notes.md"); err != nil {
		return fail("stop-continuation", err.Error())
	}
	if err := git(root, "commit", "-m", "init"); err != nil {
		return fail("stop-continuation", err.Error())
	}
	if err := os.WriteFile(path, []byte("done\nTODO: follow up\n"), 0o644); err != nil {
		return fail("stop-continuation", err.Error())
	}
	decisions, err := engine.Evaluate(ctx, types.Event{Event: "Stop", CWD: root}, cfg)
	if err != nil {
		return fail("stop-continuation", err.Error())
	}
	if decisions[0].Mode != types.ModeContinue {
		return fail("stop-continuation", "expected continue decision")
	}
	if err := os.WriteFile(path, []byte("done\nfollow-up tracked in issue HL-1\n"), 0o644); err != nil {
		return fail("stop-continuation", err.Error())
	}
	decisions, err = engine.Evaluate(ctx, types.Event{Event: "Stop", CWD: root}, cfg)
	if err != nil {
		return fail("stop-continuation", err.Error())
	}
	if decisions[0].Mode != types.ModeAllow {
		return fail("stop-continuation", "expected agent follow-up to clear stop continuation")
	}
	return resolved(
		"stop-continuation",
		"Agent tried to stop after adding a TODO line.",
		"Stop hook asked Codex to continue once and address or justify the TODO.",
		"Agent replaced the TODO with a concrete follow-up reference.",
		"Second Stop evaluation passed with no continuation request.",
		stopContinuationEvidence(),
	)
}

func tempRepo() (string, func(), error) {
	dir, err := os.MkdirTemp("", "hookline-bench-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := git(dir, "init"); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := git(dir, "config", "commit.gpgsign", "false"); err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

func git(root string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.Run()
}

func pass(name string) Scenario {
	return Scenario{Name: name, Status: "pass", Resolved: true}
}

func fail(name, message string) Scenario {
	return Scenario{Name: name, Status: "fail", Message: message}
}

func resolved(name, start, hookline, agentAction, result string, evidence Evidence) Scenario {
	return Scenario{
		Name:        name,
		Status:      "pass",
		Resolved:    true,
		Start:       start,
		Hookline:    hookline,
		AgentAction: agentAction,
		Result:      result,
		Evidence:    evidence,
	}
}

func hasRule(decisions []types.Decision, ruleID string) bool {
	for _, decision := range decisions {
		if decision.RuleID == ruleID {
			return true
		}
	}
	return false
}

func linesText(count int) string {
	return strings.Repeat("line\n", count)
}
