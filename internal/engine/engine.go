package engine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/hookstate"
	"github.com/fireharp/hookline/internal/recipes"
	"github.com/fireharp/hookline/internal/types"
)

func Evaluate(ctx context.Context, event types.Event, cfg config.Config) ([]types.Decision, error) {
	return EvaluateWithRoot(ctx, event, cfg, event.CWD)
}

func EvaluateWithRoot(ctx context.Context, event types.Event, cfg config.Config, root string) ([]types.Decision, error) {
	if root == "" {
		root = event.CWD
	}
	var decisions []types.Decision
	switch event.Event {
	case "PreToolUse":
		if recipeEnabled(cfg, recipes.AgentSteering) {
			decisions = append(decisions, dangerousShell(event, cfg)...)
		}
	case "UserPromptSubmit":
		if recipeEnabled(cfg, recipes.AgentSteering) {
			decisions = append(decisions, skillTriggers(event, cfg)...)
		}
	case "PostToolUse":
		decisions = append(decisions, diffChecks(root, event, cfg)...)
	case "Stop":
		if !event.StopHookActive {
			stop, err := stopChecks(ctx, event, root, cfg)
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

func diffChecks(root string, event types.Event, cfg config.Config) []types.Decision {
	var decisions []types.Decision
	stats := changedFiles(root)
	if recipeEnabled(cfg, recipes.LineCount) {
		files, snoozed := activeOversizedFiles(root, event, stats, cfg)
		if len(files) > 0 {
			decisions = append(decisions, types.Decision{
				Mode:    types.ModeContinue,
				RuleID:  "large-file-split-review",
				Message: largeFileMessage(root, event, files),
			})
		} else if snoozed != nil {
			decisions = append(decisions, *snoozed)
		}
	}
	if !recipeEnabled(cfg, recipes.AgentSteering) {
		return decisions
	}
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

func stopChecks(ctx context.Context, event types.Event, root string, cfg config.Config) ([]types.Decision, error) {
	_ = ctx
	var failures []string
	var snoozed *types.Decision
	stats := changedFiles(root)
	if recipeEnabled(cfg, recipes.AgentSteering) {
		if sensitiveWithoutTests(stats, cfg) {
			failures = append(failures, "sensitive paths changed without a test file change")
		}
		if codeWithoutTests(stats) {
			failures = append(failures, "code changed without a test file change")
		}
		if addedTodoLines(root) {
			failures = append(failures, "new TODO/FIXME added")
		}
	}
	if recipeEnabled(cfg, recipes.LineCount) {
		files, skipped := activeOversizedFiles(root, event, stats, cfg)
		if len(files) > 0 {
			failures = append(failures, largeFileMessage(root, event, files))
		} else if skipped != nil {
			snoozed = skipped
		}
	}
	if len(failures) == 0 {
		if snoozed != nil {
			return []types.Decision{*snoozed}, nil
		}
		return nil, nil
	}
	return []types.Decision{{
		Mode:    types.ModeContinue,
		RuleID:  "stop-review",
		Message: "Before stopping, address or explicitly justify: " + strings.Join(failures, "; "),
	}}, nil
}

func recipeEnabled(cfg config.Config, id string) bool {
	for _, enabled := range cfg.Recipes.Enabled {
		if enabled == id {
			return true
		}
	}
	return false
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

type oversizedFile struct {
	Path  string
	Lines int
}

func oversizedTouchedFiles(root string, stats []diffStat, cfg config.Config) []oversizedFile {
	limit := cfg.Limits.SplitReviewLineLimit
	if limit <= 0 {
		return nil
	}
	var files []oversizedFile
	for _, stat := range stats {
		if !isCode(stat.Path) {
			continue
		}
		lines, err := lineCount(filepath.Join(root, stat.Path))
		if err == nil && lines >= limit {
			files = append(files, oversizedFile{Path: stat.Path, Lines: lines})
		}
	}
	return files
}

func activeOversizedFiles(root string, event types.Event, stats []diffStat, cfg config.Config) ([]oversizedFile, *types.Decision) {
	files := oversizedTouchedFiles(root, stats, cfg)
	if len(files) == 0 {
		return nil, nil
	}
	store := hookstate.New(root)
	now := time.Now()
	var active []oversizedFile
	var lastSnooze *hookstate.Snooze
	for _, file := range files {
		snooze, err := store.ActiveSnooze("large-file-split-review", file.Path, event.SessionID, now)
		if err != nil || snooze == nil {
			active = append(active, file)
			continue
		}
		lastSnooze = snooze
	}
	if len(active) > 0 {
		return active, nil
	}
	if lastSnooze == nil {
		return nil, nil
	}
	return nil, &types.Decision{
		Mode:        types.ModeAllow,
		RuleID:      "large-file-split-review",
		Snoozed:     true,
		SnoozeScope: lastSnooze.Scope,
		SnoozePath:  lastSnooze.Path,
	}
}

func largeFileMessage(root string, event types.Event, files []oversizedFile) string {
	first := files[0]
	message := "Touched very large file(s): " + oversizedSummary(files) + ". Choose one: split now, snooze if unrelated, or record a keep decision with why-split and why-not-now."
	message += " Suggested seam: " + suggestedSeam(first.Path) + "."
	if decision := latestDecisionSummary(root, first.Path); decision != "" {
		message += " " + decision
	}
	message += " Snooze: " + snoozeCommand(event.SessionID, first.Path) + "."
	message += " Record decision: " + decisionCommand(event.SessionID, first.Path) + "."
	if len(files) > 1 {
		message += " Repeat per path or use --path '*'."
	}
	return message
}

func latestDecisionSummary(root, path string) string {
	decision, err := hookstate.New(root).LatestDecision("large-file-split-review", path)
	if err != nil || decision == nil {
		return ""
	}
	var parts []string
	if decision.Action != "" {
		parts = append(parts, "action="+decision.Action)
	}
	if decision.WhySplit != "" {
		parts = append(parts, "why split: "+decision.WhySplit)
	}
	if decision.WhyNotNow != "" {
		parts = append(parts, "why not now: "+decision.WhyNotNow)
	}
	if decision.Result != "" {
		parts = append(parts, "result: "+decision.Result)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Previous decision: " + strings.Join(parts, "; ") + "."
}

func suggestedSeam(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "fixtures") && strings.Contains(lower, ".test."):
		return "keep a thin runner and split fixture cases by domain under fixtures/*.test.*"
	case strings.Contains(lower, "group_runner"):
		return "extract routing tables, handlers, and tests into focused modules"
	case isTest(lower):
		return "split test suites by behavior or fixture type"
	default:
		return "extract cohesive helpers/types and keep a thin entrypoint"
	}
}

func snoozeCommand(sessionID, path string) string {
	args := []string{
		"hookline", "snooze", "add",
		"--rule", "large-file-split-review",
		"--path", path,
		"--scope", "session",
		"--duration", "4h",
	}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	args = append(args, "--reason", "unrelated to current task")
	return shellWords(args)
}

func decisionCommand(sessionID, path string) string {
	args := []string{
		"hookline", "decision", "add",
		"--rule", "large-file-split-review",
		"--path", path,
		"--action", "keep",
		"--why-split", "obvious split seams exist",
		"--why-not-now", "unrelated or too risky for this task",
		"--result", "kept for now",
	}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	return shellWords(args)
}

func shellWords(args []string) string {
	var quoted []string
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return strings.Join(quoted, " ")
}

func oversizedSummary(files []oversizedFile) string {
	var parts []string
	for i, file := range files {
		if i >= 3 {
			parts = append(parts, fmt.Sprintf("+%d more", len(files)-i))
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d lines)", file.Path, file.Lines))
	}
	return strings.Join(parts, ", ")
}

func lineCount(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
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
