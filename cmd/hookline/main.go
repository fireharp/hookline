package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fireharp/hookline/internal/bench"
	"github.com/fireharp/hookline/internal/codex"
	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/lines"
	"github.com/fireharp/hookline/internal/secrets"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usage(stdout)
	}
	root, err := config.FindRoot(".")
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	switch args[0] {
	case "hook":
		if len(args) < 2 || args[1] != "codex" {
			return errors.New("usage: hookline hook codex")
		}
		return codex.Handle(ctx, stdin, stdout, cfg)
	case "doctor":
		return doctor(ctx, stdout, root, cfg, hasFlag(args, "--json"))
	case "scan":
		return scan(ctx, args[1:], stdout, root, cfg)
	case "bench":
		return runBench(ctx, args[1:], stdout, cfg)
	default:
		return usage(stdout)
	}
}

func usage(w io.Writer) error {
	_, err := fmt.Fprintln(w, `usage:
  hookline hook codex
  hookline doctor [--json]
  hookline scan lines [--json]
  hookline scan secrets [--json]
  hookline bench [--suite smoke] [--json]`)
	return err
}

func scan(ctx context.Context, args []string, stdout io.Writer, root string, cfg config.Config) error {
	if len(args) == 0 {
		return errors.New("usage: hookline scan lines|secrets")
	}
	jsonOut := hasFlag(args, "--json")
	switch args[0] {
	case "lines":
		result, err := lines.Scan(root, cfg.Limits)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, result)
		}
		for _, v := range result.Violations {
			fmt.Fprintf(stdout, "%s:%d exceeds %d lines\n", v.Path, v.Lines, v.Limit)
		}
		if len(result.Violations) > 0 {
			return fmt.Errorf("%d file(s) exceed line limits", len(result.Violations))
		}
		fmt.Fprintln(stdout, "line scan passed")
		return nil
	case "secrets":
		result, err := secrets.ScanStaged(ctx, root, cfg.Secrets)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, result)
		}
		for _, leak := range result.EnvLeaks {
			fmt.Fprintf(stdout, "%s: %s appears in staged additions\n", leak.Key, leak.Redacted)
		}
		if len(result.EnvLeaks) > 0 {
			return fmt.Errorf("%d local env value(s) found in staged changes", len(result.EnvLeaks))
		}
		fmt.Fprintln(stdout, "secret scan passed")
		return nil
	default:
		return errors.New("usage: hookline scan lines|secrets")
	}
}

func runBench(ctx context.Context, args []string, stdout io.Writer, cfg config.Config) error {
	jsonOut := hasFlag(args, "--json")
	suite := "smoke"
	for i, arg := range args {
		if arg == "--suite" && i+1 < len(args) {
			suite = args[i+1]
		}
		if strings.HasPrefix(arg, "--suite=") {
			suite = strings.TrimPrefix(arg, "--suite=")
		}
	}
	result, err := bench.Run(ctx, suite, cfg)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, result)
	}
	for _, scenario := range result.Scenarios {
		fmt.Fprintf(stdout, "%s: %s\n", scenario.Name, scenario.Status)
	}
	if !result.Pass {
		return errors.New("bench failed")
	}
	return nil
}

func doctor(ctx context.Context, stdout io.Writer, root string, cfg config.Config, jsonOut bool) error {
	type check struct {
		ID      string `json:"id"`
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	checks := []check{
		{ID: "root", OK: root != "", Message: root},
		{ID: "project-config", OK: true, Message: filepath.Join(root, ".fireharp", "harness.yaml")},
		{ID: "codex-hooks", OK: fileExists(filepath.Join(root, ".codex", "hooks.json")), Message: ".codex/hooks.json"},
		{ID: "pre-commit", OK: fileExists(filepath.Join(root, ".githooks", "pre-commit")), Message: ".githooks/pre-commit"},
		{ID: "gitleaks", OK: commandExists("gitleaks"), Message: "gitleaks on PATH"},
		{ID: "line-limit", OK: cfg.Limits.FileLineLimit > 0, Message: fmt.Sprintf("%d", cfg.Limits.FileLineLimit)},
	}
	if commandExists("coherence") {
		cmd := exec.CommandContext(ctx, "coherence", "doctor", "--json")
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			checks = append(checks, check{ID: "coherence", OK: false, Message: err.Error()})
		} else {
			checks = append(checks, check{ID: "coherence", OK: true, Message: "coherence doctor passed"})
		}
	} else {
		checks = append(checks, check{ID: "coherence", OK: false, Message: "coherence not on PATH"})
	}
	ok := true
	for _, c := range checks {
		ok = ok && c.OK
	}
	result := map[string]any{"ok": ok, "checks": checks}
	if jsonOut {
		return writeJSON(stdout, result)
	}
	for _, c := range checks {
		status := "ok"
		if !c.OK {
			status = "fail"
		}
		fmt.Fprintf(stdout, "%s: %s (%s)\n", c.ID, status, c.Message)
	}
	if !ok {
		return errors.New("doctor failed")
	}
	return nil
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
