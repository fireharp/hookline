package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fireharp/hookline/internal/bench"
	"github.com/fireharp/hookline/internal/codex"
	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/lines"
	"github.com/fireharp/hookline/internal/recipes"
	"github.com/fireharp/hookline/internal/secrets"
	"github.com/fireharp/hookline/internal/telemetry"
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
	registry, err := recipes.Load(root, cfg)
	if err != nil {
		return err
	}

	switch args[0] {
	case "hook":
		if len(args) < 2 || args[1] != "codex" {
			return errors.New("usage: hookline hook codex")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		if eventRoot := hookEventRoot(data); eventRoot != "" {
			if found, err := config.FindRoot(eventRoot); err == nil {
				root = found
				cfg, err = config.Load(root)
				if err != nil {
					return err
				}
				registry, err = recipes.Load(root, cfg)
				if err != nil {
					return err
				}
			}
		}
		if !config.HooksEnabled(cfg) || !registry.HasEnabledSurface("hook") {
			return nil
		}
		start := time.Now()
		var out bytes.Buffer
		hookErr := codex.Handle(ctx, bytes.NewReader(data), &out, cfg)
		if _, err := stdout.Write(out.Bytes()); err != nil {
			return err
		}
		if err := telemetry.Append(root, cfg, data, out.Bytes(), hookErr, time.Since(start)); err != nil {
			fmt.Fprintf(stderr, "hookline telemetry: %v\n", err)
		}
		return hookErr
	case "init":
		return initRecipes(args[1:], stdout, root, cfg, registry)
	case "doctor":
		opts, err := parseDoctorOptions(args[1:])
		if err != nil {
			return err
		}
		return doctor(ctx, stdout, root, cfg, registry, opts)
	case "recipe":
		return recipeCommand(args[1:], stdout, registry)
	case "scan":
		return scan(ctx, args[1:], stdout, root, cfg, registry)
	case "telemetry":
		return telemetryCommand(args[1:], stdout, root, cfg)
	case "bench":
		return runBench(ctx, args[1:], stdout, cfg)
	default:
		return usage(stdout)
	}
}

func usage(w io.Writer) error {
	_, err := fmt.Fprintln(w, `usage:
  hookline hook codex
  hookline recipe list [--json]
  hookline init --recipe <id>... [--scope project|user|both] [--command <cmd>] [--force] [--json]
  hookline doctor [--json] [--recipe <id>]
  hookline scan lines [--json]
  hookline scan secrets [--json]
  hookline telemetry status|tail [--limit N] [--json]
  hookline bench [--suite smoke] [--json]`)
	return err
}

func scan(ctx context.Context, args []string, stdout io.Writer, root string, cfg config.Config, registry recipes.Registry) error {
	if len(args) == 0 {
		return errors.New("usage: hookline scan lines|secrets")
	}
	jsonOut := hasFlag(args, "--json")
	switch args[0] {
	case "lines":
		if _, ok := registry.Get(recipes.LineCount); !ok {
			return errors.New("line-count recipe is not loaded")
		}
		result, err := lines.Scan(root, cfg.Limits)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, result)
		}
		for _, v := range result.Findings {
			fmt.Fprintf(stdout, "%s:%d over soft target %d lines; review split vs keep\n", v.Path, v.Lines, v.Limit)
		}
		if len(result.Findings) > 0 {
			fmt.Fprintln(stdout, "line scan completed with advisory findings")
			return nil
		}
		fmt.Fprintln(stdout, "line scan passed")
		return nil
	case "secrets":
		if _, ok := registry.Get(recipes.SecretsGitleaks); !ok {
			return errors.New("secrets-gitleaks recipe is not loaded")
		}
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

func telemetryCommand(args []string, stdout io.Writer, root string, cfg config.Config) error {
	if len(args) == 0 {
		return errors.New("usage: hookline telemetry status|tail [--limit N] [--json]")
	}
	jsonOut := hasFlag(args, "--json")
	switch args[0] {
	case "status":
		count, err := telemetry.Count(root, cfg.Telemetry)
		if err != nil {
			return err
		}
		status := map[string]any{
			"enabled": config.TelemetryEnabled(cfg),
			"path":    telemetry.Path(root, cfg.Telemetry),
			"events":  count,
		}
		if jsonOut {
			return writeJSON(stdout, status)
		}
		fmt.Fprintf(stdout, "telemetry enabled=%t events=%d path=%s\n", status["enabled"], status["events"], status["path"])
		return nil
	case "tail":
		limit := intFlag(args, "--limit", 20)
		records, err := telemetry.Tail(root, cfg.Telemetry, limit)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, records)
		}
		for _, record := range records {
			message := record.Reason
			if message == "" {
				message = record.AdditionalContext
			}
			fmt.Fprintf(stdout, "%s %s %s decision=%s empty=%t %s\n", record.Time, record.Event, record.Tool, record.Decision, record.OutputEmpty, message)
		}
		return nil
	default:
		return errors.New("usage: hookline telemetry status|tail [--limit N] [--json]")
	}
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func intFlag(args []string, flag string, fallback int) int {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			var value int
			if _, err := fmt.Sscanf(args[i+1], "%d", &value); err == nil {
				return value
			}
		}
		if strings.HasPrefix(arg, flag+"=") {
			var value int
			if _, err := fmt.Sscanf(strings.TrimPrefix(arg, flag+"="), "%d", &value); err == nil {
				return value
			}
		}
	}
	return fallback
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func hookEventRoot(data []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}
	cwd, _ := raw["cwd"].(string)
	return cwd
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
