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
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func Run(ctx context.Context, suite string, cfg config.Config) (Result, error) {
	if suite == "" {
		suite = "smoke"
	}
	result := Result{Suite: suite, Pass: true}
	checks := []func(context.Context, config.Config) Scenario{
		oversizedFile,
		envLeak,
		dangerousBash,
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

func oversizedFile(_ context.Context, cfg config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("oversized-file", err.Error())
	}
	defer cleanup()
	path := filepath.Join(root, "big.go")
	if err := os.WriteFile(path, []byte(strings.Repeat("package main\n", cfg.Limits.FileLineLimit+1)), 0o644); err != nil {
		return fail("oversized-file", err.Error())
	}
	result, err := lines.Scan(root, cfg.Limits)
	if err != nil {
		return fail("oversized-file", err.Error())
	}
	if len(result.Violations) == 0 {
		return fail("oversized-file", "expected line violation")
	}
	return pass("oversized-file")
}

func envLeak(_ context.Context, cfg config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("env-leak", err.Error())
	}
	defer cleanup()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("LOCAL_VALUE=abcdef123456\n"), 0o600); err != nil {
		return fail("env-leak", err.Error())
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("token abcdef123456\n"), 0o644); err != nil {
		return fail("env-leak", err.Error())
	}
	if err := git(root, "add", "README.md"); err != nil {
		return fail("env-leak", err.Error())
	}
	leaks, err := secrets.EnvLeaks(root, cfg.Secrets)
	if err != nil {
		return fail("env-leak", err.Error())
	}
	if len(leaks) == 0 || strings.Contains(leaks[0].Redacted, "abcdef123456") {
		return fail("env-leak", "expected redacted env leak")
	}
	return pass("env-leak")
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
	return pass("dangerous-bash")
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
			return pass("sensitive-path")
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
	if err := os.WriteFile(filepath.Join(root, "big.md"), []byte(strings.Repeat("line\n", cfg.Limits.FileLineLimit+1)), 0o644); err != nil {
		return fail("stop-continuation", err.Error())
	}
	decisions, err := engine.Evaluate(ctx, types.Event{Event: "Stop", CWD: root}, cfg)
	if err != nil {
		return fail("stop-continuation", err.Error())
	}
	if decisions[0].Mode != types.ModeContinue {
		return fail("stop-continuation", "expected continue decision")
	}
	return pass("stop-continuation")
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
	return dir, cleanup, nil
}

func git(root string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.Run()
}

func pass(name string) Scenario {
	return Scenario{Name: name, Status: "pass"}
}

func fail(name, message string) Scenario {
	return Scenario{Name: name, Status: "fail", Message: message}
}
