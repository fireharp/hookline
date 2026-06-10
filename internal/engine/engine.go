package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/lines"
	"github.com/fireharp/hookline/internal/types"
)

func Evaluate(ctx context.Context, event types.Event, cfg config.Config) ([]types.Decision, error) {
	var decisions []types.Decision
	switch event.Event {
	case "PreToolUse":
		decisions = append(decisions, dangerousShell(event, cfg)...)
	case "UserPromptSubmit":
		decisions = append(decisions, skillTriggers(event, cfg)...)
	case "PostToolUse":
		decisions = append(decisions, diffChecks(event.CWD, cfg)...)
	case "Stop":
		if !event.StopHookActive {
			stop, err := stopChecks(ctx, event.CWD, cfg)
			if err != nil {
				return nil, err
			}
			decisions = append(decisions, stop...)
		}
	}
	if len(decisions) == 0 {
		return []types.Decision{{Mode: types.ModeAllow}}, nil
	}
	return decisions, nil
}

func dangerousShell(event types.Event, cfg config.Config) []types.Decision {
	if event.Tool == nil || event.Tool.Name != "Bash" || event.Tool.Command == "" {
		return nil
	}
	for _, pattern := range cfg.DangerousShell {
		matched, err := regexp.MatchString(pattern, event.Tool.Command)
		if err == nil && matched {
			return []types.Decision{{
				Mode:    types.ModeBlock,
				RuleID:  "dangerous-shell",
				Message: "Dangerous shell command blocked by Hookline policy.",
			}}
		}
	}
	return nil
}

func skillTriggers(event types.Event, cfg config.Config) []types.Decision {
	var decisions []types.Decision
	for _, rule := range cfg.SkillTriggers {
		if rule.Regex == "" {
			continue
		}
		matched, err := regexp.MatchString(rule.Regex, event.Prompt)
		if err == nil && matched {
			message := rule.Message
			if message == "" && len(rule.Skills) > 0 {
				message = "Relevant skills: " + strings.Join(rule.Skills, ", ")
			}
			decisions = append(decisions, types.Decision{
				Mode:     types.ModeNudge,
				RuleID:   rule.ID,
				Severity: rule.Severity,
				Message:  message,
			})
		}
	}
	return decisions
}

func diffChecks(root string, cfg config.Config) []types.Decision {
	var decisions []types.Decision
	stats := changedFiles(root)
	added := 0
	for _, stat := range stats {
		added += stat.Added
	}
	if cfg.Limits.LargeDiffAdded > 0 && added > cfg.Limits.LargeDiffAdded {
		decisions = append(decisions, types.Decision{
			Mode:    types.ModeContinue,
			RuleID:  "large-diff",
			Message: fmt.Sprintf("Large diff detected (%d added lines). Review for accidental rewrites before stopping.", added),
		})
	}
	if sensitiveWithoutTests(stats, cfg) {
		decisions = append(decisions, types.Decision{
			Mode:    types.ModeNudge,
			RuleID:  "sensitive-path",
			Message: "Sensitive path touched. Check idempotency, auth boundaries, secrets, and regression coverage.",
		})
	}
	return decisions
}

func stopChecks(ctx context.Context, root string, cfg config.Config) ([]types.Decision, error) {
	_ = ctx
	var failures []string
	lineResult, err := lines.Scan(root, cfg.Limits)
	if err != nil {
		return nil, err
	}
	for _, violation := range lineResult.Violations {
		failures = append(failures, fmt.Sprintf("%s has %d lines (limit %d)", violation.Path, violation.Lines, violation.Limit))
	}
	stats := changedFiles(root)
	if sensitiveWithoutTests(stats, cfg) {
		failures = append(failures, "sensitive paths changed without a test file change")
	}
	if codeWithoutTests(stats) {
		failures = append(failures, "code changed without a test file change")
	}
	if addedTodoLines(root) {
		failures = append(failures, "new TODO/FIXME added")
	}
	if len(failures) == 0 {
		return nil, nil
	}
	return []types.Decision{{
		Mode:    types.ModeContinue,
		RuleID:  "stop-review",
		Message: "Before stopping, address or explicitly justify: " + strings.Join(failures, "; "),
	}}, nil
}

func sensitiveWithoutTests(stats []diffStat, cfg config.Config) bool {
	sensitive := false
	tests := false
	for _, stat := range stats {
		if isTest(stat.Path) {
			tests = true
		}
		for _, pattern := range cfg.SensitivePaths {
			if globMatch(pattern, stat.Path) {
				sensitive = true
			}
		}
	}
	return sensitive && !tests
}

func codeWithoutTests(stats []diffStat) bool {
	code := false
	tests := false
	for _, stat := range stats {
		if isTest(stat.Path) {
			tests = true
		}
		if isCode(stat.Path) {
			code = true
		}
	}
	return code && !tests
}

func isTest(path string) bool {
	return strings.Contains(path, "_test.") || strings.Contains(path, ".test.") || strings.Contains(path, "/test/") || strings.Contains(path, "/tests/")
}

func isCode(path string) bool {
	for _, suffix := range []string{".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rs"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
