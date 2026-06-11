package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fireharp/hookline/internal/hookstate"
)

func snoozeCommand(args []string, stdout io.Writer, root string) error {
	if len(args) == 0 {
		return errors.New("usage: hookline snooze add|list|clear [--json]")
	}
	switch args[0] {
	case "add":
		entry, jsonOut, err := parseSnoozeAdd(args[1:])
		if err != nil {
			return err
		}
		entry, err = hookstate.New(root).AddSnooze(entry)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, entry)
		}
		fmt.Fprintf(stdout, "snoozed %s %s %s until %s\n", entry.RuleID, entry.Scope, entry.Path, entry.ExpiresAt)
		return nil
	case "list":
		jsonOut := hasFlag(args[1:], "--json")
		entries, err := hookstate.New(root).Snoozes()
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, entries)
		}
		for _, entry := range entries {
			fmt.Fprintf(stdout, "%s %s %s scope=%s expires=%s reason=%s\n", entry.ID, entry.RuleID, entry.Path, entry.Scope, entry.ExpiresAt, entry.Reason)
		}
		return nil
	case "clear":
		filter, jsonOut, err := parseSnoozeClear(args[1:])
		if err != nil {
			return err
		}
		removed, err := hookstate.New(root).ClearSnoozes(filter)
		if err != nil {
			return err
		}
		result := map[string]any{"removed": removed}
		if jsonOut {
			return writeJSON(stdout, result)
		}
		fmt.Fprintf(stdout, "removed %d snooze(s)\n", removed)
		return nil
	default:
		return errors.New("usage: hookline snooze add|list|clear [--json]")
	}
}

func decisionCommand(args []string, stdout io.Writer, root string) error {
	if len(args) == 0 {
		return errors.New("usage: hookline decision add|list [--json]")
	}
	switch args[0] {
	case "add":
		entry, jsonOut, err := parseDecisionAdd(args[1:])
		if err != nil {
			return err
		}
		entry, err = hookstate.New(root).AddDecision(entry)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, entry)
		}
		fmt.Fprintf(stdout, "recorded %s decision for %s\n", entry.Action, entry.Path)
		return nil
	case "list":
		jsonOut := hasFlag(args[1:], "--json")
		entries, err := hookstate.New(root).Decisions()
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, entries)
		}
		for _, entry := range entries {
			fmt.Fprintf(stdout, "%s %s %s action=%s result=%s\n", entry.Time, entry.RuleID, entry.Path, entry.Action, entry.Result)
		}
		return nil
	default:
		return errors.New("usage: hookline decision add|list [--json]")
	}
}

func parseSnoozeAdd(args []string) (hookstate.Snooze, bool, error) {
	values, jsonOut, err := parseFlags(args, map[string]bool{
		"--rule": true, "--path": true, "--scope": true, "--duration": true, "--session": true, "--reason": true,
	})
	if err != nil {
		return hookstate.Snooze{}, false, err
	}
	durationText := valueOr(values, "--duration", "4h")
	duration, err := time.ParseDuration(durationText)
	if err != nil {
		return hookstate.Snooze{}, false, fmt.Errorf("invalid --duration %q", durationText)
	}
	entry := hookstate.Snooze{
		RuleID:    values["--rule"],
		Path:      valueOr(values, "--path", "*"),
		Scope:     valueOr(values, "--scope", hookstate.ScopeSession),
		SessionID: values["--session"],
		ExpiresAt: time.Now().UTC().Add(duration).Format(time.RFC3339Nano),
		Reason:    values["--reason"],
	}
	if entry.RuleID == "" {
		return hookstate.Snooze{}, false, errors.New("snooze add requires --rule")
	}
	if entry.Scope != hookstate.ScopeSession && entry.Scope != hookstate.ScopeProject {
		return hookstate.Snooze{}, false, errors.New("--scope must be session or project")
	}
	if entry.Scope == hookstate.ScopeSession && entry.SessionID == "" {
		return hookstate.Snooze{}, false, errors.New("session snooze requires --session")
	}
	return entry, jsonOut, nil
}

func parseSnoozeClear(args []string) (hookstate.SnoozeFilter, bool, error) {
	values, jsonOut, err := parseFlags(args, map[string]bool{
		"--all": false, "--rule": true, "--path": true, "--scope": true, "--session": true,
	})
	if err != nil {
		return hookstate.SnoozeFilter{}, false, err
	}
	filter := hookstate.SnoozeFilter{
		RuleID:    values["--rule"],
		Path:      values["--path"],
		Scope:     values["--scope"],
		SessionID: values["--session"],
		All:       values["--all"] == "true",
	}
	if !filter.All && filter.RuleID == "" && filter.Path == "" && filter.Scope == "" && filter.SessionID == "" {
		return hookstate.SnoozeFilter{}, false, errors.New("snooze clear requires --all or a filter")
	}
	return filter, jsonOut, nil
}

func parseDecisionAdd(args []string) (hookstate.Decision, bool, error) {
	values, jsonOut, err := parseFlags(args, map[string]bool{
		"--rule": true, "--path": true, "--action": true, "--session": true, "--why-split": true, "--why-not-now": true, "--result": true,
	})
	if err != nil {
		return hookstate.Decision{}, false, err
	}
	entry := hookstate.Decision{
		RuleID:    values["--rule"],
		Path:      valueOr(values, "--path", "*"),
		Action:    values["--action"],
		SessionID: values["--session"],
		WhySplit:  values["--why-split"],
		WhyNotNow: values["--why-not-now"],
		Result:    values["--result"],
	}
	if entry.RuleID == "" || entry.Action == "" {
		return hookstate.Decision{}, false, errors.New("decision add requires --rule and --action")
	}
	switch entry.Action {
	case "split", "keep", "snooze":
		return entry, jsonOut, nil
	default:
		return hookstate.Decision{}, false, errors.New("--action must be split, keep, or snooze")
	}
}

func parseFlags(args []string, known map[string]bool) (map[string]string, bool, error) {
	values := map[string]string{}
	jsonOut := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--json" {
			jsonOut = true
			continue
		}
		needsValue, ok := known[arg]
		if !ok {
			if key, value, found := strings.Cut(arg, "="); found {
				needsValue, ok = known[key]
				if ok && needsValue {
					values[key] = value
					continue
				}
			}
			return nil, false, fmt.Errorf("unknown option %q", arg)
		}
		if !needsValue {
			values[arg] = "true"
			continue
		}
		if i+1 >= len(args) {
			return nil, false, fmt.Errorf("%s requires a value", arg)
		}
		i++
		values[arg] = args[i]
	}
	return values, jsonOut, nil
}

func valueOr(values map[string]string, key, fallback string) string {
	if values[key] != "" {
		return values[key]
	}
	return fallback
}
