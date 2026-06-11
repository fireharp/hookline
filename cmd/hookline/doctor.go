package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
	"github.com/fireharp/hookline/internal/telemetry"
)

type doctorOptions struct {
	JSON      bool
	RecipeIDs []string
}

type doctorCheck struct {
	ID          string `json:"id"`
	RecipeID    string `json:"recipe_id,omitempty"`
	OK          bool   `json:"ok"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type doctorResult struct {
	OK             bool          `json:"ok"`
	EnabledRecipes []string      `json:"enabled_recipes"`
	Checks         []doctorCheck `json:"checks"`
}

func parseDoctorOptions(args []string) (doctorOptions, error) {
	var opts doctorOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--recipe" && i+1 < len(args):
			i++
			opts.RecipeIDs = append(opts.RecipeIDs, args[i])
		case strings.HasPrefix(arg, "--recipe="):
			opts.RecipeIDs = append(opts.RecipeIDs, strings.TrimPrefix(arg, "--recipe="))
		default:
			return doctorOptions{}, fmt.Errorf("unknown doctor option %q", arg)
		}
	}
	return opts, nil
}

func doctor(ctx context.Context, stdout io.Writer, root string, cfg config.Config, registry recipes.Registry, opts doctorOptions) error {
	manifests, err := doctorManifests(registry, opts.RecipeIDs)
	if err != nil {
		return err
	}
	checks := []doctorCheck{
		{ID: "root", OK: root != "", Message: root},
		{ID: "project-config", OK: true, Message: filepath.Join(root, ".fireharp", "harness.yaml")},
		{ID: "enabled-recipes", OK: true, Message: strings.Join(registry.EnabledIDs(), ", ")},
		{ID: "telemetry", OK: true, Message: telemetry.Path(root, cfg.Telemetry)},
	}
	if len(registry.EnabledIDs()) == 0 {
		checks[2].Message = "none"
	}
	for _, manifest := range manifests {
		checks = append(checks, doctorRecipe(ctx, root, cfg, manifest)...)
	}
	ok := true
	for _, check := range checks {
		ok = ok && check.OK
	}
	result := doctorResult{OK: ok, EnabledRecipes: registry.EnabledIDs(), Checks: checks}
	if opts.JSON {
		return writeJSON(stdout, result)
	}
	for _, check := range checks {
		status := "ok"
		if !check.OK {
			status = "fail"
		}
		fmt.Fprintf(stdout, "%s: %s (%s)\n", check.ID, status, check.Message)
		if check.Remediation != "" && !check.OK {
			fmt.Fprintf(stdout, "  fix: %s\n", check.Remediation)
		}
	}
	if !ok {
		return errors.New("doctor failed")
	}
	return nil
}

func doctorManifests(registry recipes.Registry, selected []string) ([]recipes.Manifest, error) {
	if len(selected) == 0 {
		return registry.EnabledManifests(), nil
	}
	var manifests []recipes.Manifest
	for _, id := range selected {
		manifest, ok := registry.Get(id)
		if !ok {
			return nil, fmt.Errorf("recipe %q is not loaded", id)
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func doctorRecipe(ctx context.Context, root string, cfg config.Config, manifest recipes.Manifest) []doctorCheck {
	var checks []doctorCheck
	switch manifest.ID {
	case recipes.CodexHooks:
		checks = append(checks, codexHookChecks(root)...)
	case recipes.Coherence:
		checks = append(checks, precommitChecks(root, manifest.ID)...)
	case recipes.SecretsGitleaks:
		checks = append(checks, precommitChecks(root, manifest.ID)...)
	case recipes.LineCount:
		checks = append(checks, doctorCheck{
			ID:          "line-limit",
			RecipeID:    manifest.ID,
			OK:          cfg.Limits.FileLineLimit > 0,
			Message:     fmt.Sprintf("%d", cfg.Limits.FileLineLimit),
			Remediation: "set limits.file_line_limit to a positive integer",
		})
	case recipes.AgentSteering:
		checks = append(checks, doctorCheck{
			ID:          "agent-steering-config",
			RecipeID:    manifest.ID,
			OK:          len(cfg.DangerousShell)+len(cfg.SensitivePaths)+len(cfg.SkillTriggers) > 0,
			Message:     fmt.Sprintf("dangerous_shell=%d sensitive_paths=%d skill_triggers=%d", len(cfg.DangerousShell), len(cfg.SensitivePaths), len(cfg.SkillTriggers)),
			Remediation: "configure at least one steering rule",
		})
	}
	for _, result := range recipes.RunCommands(ctx, root, manifest, "doctor") {
		checks = append(checks, doctorCheck{
			ID:          manifest.ID + "-command",
			RecipeID:    manifest.ID,
			OK:          result.OK,
			Message:     commandMessage(result),
			Remediation: installHint(manifest.ID),
		})
	}
	return checks
}

func codexHookChecks(root string) []doctorCheck {
	projectHooks := fileExists(filepath.Join(root, ".codex", "hooks.json"))
	userHooksPath := ""
	userHooks := false
	if home, err := os.UserHomeDir(); err == nil {
		userHooksPath = filepath.Join(home, ".codex", "hooks.json")
		userHooks = fileExists(userHooksPath)
	}
	return []doctorCheck{{
		ID:          "codex-hooks",
		RecipeID:    recipes.CodexHooks,
		OK:          projectHooks || userHooks,
		Message:     fmt.Sprintf("project=%t user=%t %s", projectHooks, userHooks, userHooksPath),
		Remediation: "run hookline init --recipe codex-hooks --scope project",
	}}
}

func precommitChecks(root, recipeID string) []doctorCheck {
	hookPath := filepath.Join(root, ".githooks", "pre-commit")
	hooksPath, _ := gitConfig(root, "core.hooksPath")
	return []doctorCheck{
		{
			ID:          recipeID + "-precommit-file",
			RecipeID:    recipeID,
			OK:          fileExists(hookPath),
			Message:     ".githooks/pre-commit",
			Remediation: "run hookline init --recipe " + recipeID,
		},
		{
			ID:          recipeID + "-hooks-path",
			RecipeID:    recipeID,
			OK:          hooksPath == ".githooks",
			Message:     hooksPath,
			Remediation: "run git config core.hooksPath .githooks",
		},
	}
}

func gitConfig(root, key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func commandMessage(result recipes.CommandResult) string {
	if result.OK {
		if result.Output != "" {
			return result.Output
		}
		return strings.Join(result.Args, " ")
	}
	if result.Error != "" {
		return result.Error
	}
	return strings.Join(result.Args, " ")
}

func installHint(recipeID string) string {
	switch recipeID {
	case recipes.Coherence:
		return "install coherence and rerun doctor"
	case recipes.SecretsGitleaks:
		return "install gitleaks, for example with brew install gitleaks"
	default:
		return ""
	}
}
