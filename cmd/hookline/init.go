package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
)

const managedByHookline = "Managed by hookline"

type initOptions struct {
	Scope   string
	Command string
	Force   bool
	JSON    bool
	Recipes []string
}

type initResult struct {
	RecipeID string `json:"recipe_id,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Path     string `json:"path,omitempty"`
	Action   string `json:"action"`
	Changed  bool   `json:"changed"`
	Message  string `json:"message,omitempty"`
}

type codexHooksFile struct {
	Hooks map[string][]codexHookMatcher `json:"hooks"`
}

type codexHookMatcher struct {
	Matcher string             `json:"matcher,omitempty"`
	Hooks   []codexHookHandler `json:"hooks"`
}

type codexHookHandler struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

var coherenceInitializer = runCoherenceInit

func initRecipes(args []string, stdout io.Writer, root string, cfg config.Config, registry recipes.Registry) error {
	opts, err := parseInitOptions(args)
	if err != nil {
		return err
	}
	var selected []recipes.Manifest
	for _, id := range opts.Recipes {
		manifest, ok := registry.Get(id)
		if !ok {
			return fmt.Errorf("recipe %q is not loaded", id)
		}
		selected = append(selected, manifest)
	}
	var results []initResult
	for _, scope := range initScopes(opts.Scope) {
		path, err := configPath(scope, root)
		if err != nil {
			return err
		}
		changed, err := ensureRecipesEnabled(path, opts.Recipes)
		if err != nil {
			return err
		}
		results = append(results, initResult{Scope: scope, Path: path, Action: "enable-recipes", Changed: changed, Message: strings.Join(opts.Recipes, ", ")})
	}
	if containsRecipe(opts.Recipes, recipes.CodexHooks) {
		for _, scope := range initScopes(opts.Scope) {
			path, err := hooksPath(scope, root)
			if err != nil {
				return err
			}
			changed, err := writeHookFile(path, commandForScope(opts.Command, scope), opts.Force)
			if err != nil {
				return err
			}
			results = append(results, initResult{RecipeID: recipes.CodexHooks, Scope: scope, Path: path, Action: "write-codex-hooks", Changed: changed})
		}
	}
	if opts.Scope != "user" {
		precommitIDs := precommitRecipeIDs(selected)
		if len(precommitIDs) > 0 {
			path := filepath.Join(root, ".githooks", "pre-commit")
			data := buildPreCommit(precommitIDs, registry)
			changed, err := writeManagedFile(path, data, 0o755, opts.Force)
			if err != nil {
				return err
			}
			results = append(results, initResult{Scope: "project", Path: path, Action: "write-precommit", Changed: changed, Message: strings.Join(precommitIDs, ", ")})
			if err := setGitConfig(root, "core.hooksPath", ".githooks"); err != nil {
				return err
			}
			results = append(results, initResult{Scope: "project", Action: "set-core-hooks-path", Changed: true, Message: ".githooks"})
		}
		managed, err := applyManagedRecipeFiles(root, selected, opts.Force)
		if err != nil {
			return err
		}
		results = append(results, managed...)
		if containsRecipe(opts.Recipes, recipes.Coherence) {
			result, err := coherenceInitializer(root)
			if err != nil {
				return err
			}
			results = append(results, result)
		}
	}
	if opts.JSON {
		return writeJSON(stdout, results)
	}
	for _, result := range results {
		action := "already current"
		if result.Changed {
			action = result.Action
		}
		target := result.Path
		if target == "" {
			target = result.Message
		}
		fmt.Fprintf(stdout, "%s: %s\n", action, target)
	}
	return nil
}

func parseInitOptions(args []string) (initOptions, error) {
	opts := initOptions{Scope: "project", Command: defaultHookCommand()}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--user":
			opts.Scope = "user"
		case arg == "--project":
			opts.Scope = "project"
		case arg == "--force":
			opts.Force = true
		case arg == "--json":
			opts.JSON = true
		case arg == "--recipe" && i+1 < len(args):
			i++
			opts.Recipes = append(opts.Recipes, splitCSV(args[i])...)
		case strings.HasPrefix(arg, "--recipe="):
			opts.Recipes = append(opts.Recipes, splitCSV(strings.TrimPrefix(arg, "--recipe="))...)
		case arg == "--scope" && i+1 < len(args):
			i++
			opts.Scope = args[i]
		case strings.HasPrefix(arg, "--scope="):
			opts.Scope = strings.TrimPrefix(arg, "--scope=")
		case arg == "--command" && i+1 < len(args):
			i++
			opts.Command = args[i]
		case strings.HasPrefix(arg, "--command="):
			opts.Command = strings.TrimPrefix(arg, "--command=")
		default:
			return initOptions{}, fmt.Errorf("unknown init option %q", arg)
		}
	}
	if opts.Scope != "project" && opts.Scope != "user" && opts.Scope != "both" {
		return initOptions{}, errors.New("usage: hookline init [--recipe <id>...] [--scope project|user|both] [--command <cmd>] [--force] [--json]")
	}
	if len(opts.Recipes) == 0 {
		opts.Recipes = recipes.StandardRecipeIDs()
	}
	opts.Recipes = uniqueStrings(opts.Recipes)
	if strings.TrimSpace(opts.Command) == "" {
		return initOptions{}, errors.New("init command cannot be empty")
	}
	return opts, nil
}

func initScopes(scope string) []string {
	if scope == "both" {
		return []string{"project", "user"}
	}
	return []string{scope}
}

func configPath(scope, root string) (string, error) {
	if scope == "project" {
		return config.ProjectConfigPath(root), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return config.UserConfigPath(home), nil
}

func ensureRecipesEnabled(path string, ids []string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		var b strings.Builder
		b.WriteString("recipes:\n  enabled:\n")
		for _, id := range ids {
			b.WriteString("    - " + id + "\n")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
		return true, os.WriteFile(path, []byte(b.String()), 0o644)
	}
	if err != nil {
		return false, err
	}
	missing := missingRecipes(string(data), ids)
	if len(missing) == 0 {
		return false, nil
	}
	next := appendRecipesBlock(string(data), missing)
	return true, os.WriteFile(path, []byte(next), 0o644)
}

func missingRecipes(data string, ids []string) []string {
	var missing []string
	for _, id := range ids {
		if !strings.Contains(data, "- "+id) {
			missing = append(missing, id)
		}
	}
	return missing
}

func appendRecipesBlock(data string, ids []string) string {
	lines := strings.Split(strings.TrimRight(data, "\n"), "\n")
	recipesStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "recipes:" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			recipesStart = i
			break
		}
	}
	if recipesStart == -1 {
		var b strings.Builder
		b.WriteString(strings.TrimRight(data, "\n"))
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("recipes:\n  enabled:\n")
		for _, id := range ids {
			b.WriteString("    - " + id + "\n")
		}
		return b.String()
	}
	blockEnd := len(lines)
	for i := recipesStart + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			blockEnd = i
			break
		}
	}
	enabledStart := -1
	for i := recipesStart + 1; i < blockEnd; i++ {
		if strings.TrimSpace(lines[i]) == "enabled:" && strings.HasPrefix(lines[i], "  ") {
			enabledStart = i
			break
		}
	}
	insertAt := recipesStart + 1
	var insert []string
	if enabledStart == -1 {
		insert = append(insert, "  enabled:")
		for _, id := range ids {
			insert = append(insert, "    - "+id)
		}
	} else {
		insertAt = enabledStart + 1
		for insertAt < blockEnd && (strings.HasPrefix(lines[insertAt], "    ") || strings.TrimSpace(lines[insertAt]) == "") {
			insertAt++
		}
		for _, id := range ids {
			insert = append(insert, "    - "+id)
		}
	}
	lines = append(lines[:insertAt], append(insert, lines[insertAt:]...)...)
	return strings.Join(lines, "\n") + "\n"
}

func hooksPath(scope, root string) (string, error) {
	if scope == "project" {
		return filepath.Join(root, ".codex", "hooks.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "hooks.json"), nil
}

func writeHookFile(path, command string, force bool) (bool, error) {
	data, err := json.MarshalIndent(buildCodexHooks(command), "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, data) {
			return false, nil
		}
		if !force {
			return false, fmt.Errorf("%s already exists; rerun with --force to replace it", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, data, 0o644)
}

func commandForScope(command, scope string) string {
	if strings.Contains(command, "--source ") || strings.Contains(command, "--source=") {
		return command
	}
	return command + " --source " + scope
}

func buildCodexHooks(command string) codexHooksFile {
	handler := func(status string) []codexHookHandler {
		return []codexHookHandler{{
			Type:          "command",
			Command:       command,
			Timeout:       30,
			StatusMessage: status,
		}}
	}
	return codexHooksFile{Hooks: map[string][]codexHookMatcher{
		"SessionStart": {{
			Matcher: "startup|resume|clear|compact",
			Hooks:   handler("Hookline session start"),
		}},
		"PreToolUse": {{
			Matcher: "Bash|apply_patch|Edit|Write",
			Hooks:   handler("Hookline policy check"),
		}},
		"PermissionRequest": {{
			Matcher: "Bash|apply_patch|Edit|Write",
			Hooks:   handler("Hookline permission review"),
		}},
		"PostToolUse": {{
			Matcher: "Bash|apply_patch|Edit|Write",
			Hooks:   handler("Hookline post-tool review"),
		}},
		"UserPromptSubmit": {{
			Hooks: handler("Hookline prompt review"),
		}},
		"PreCompact": {{
			Matcher: "manual|auto",
			Hooks:   handler("Hookline pre-compact review"),
		}},
		"PostCompact": {{
			Matcher: "manual|auto",
			Hooks:   handler("Hookline post-compact review"),
		}},
		"SubagentStart": {{
			Hooks: handler("Hookline subagent start"),
		}},
		"SubagentStop": {{
			Hooks: handler("Hookline subagent stop"),
		}},
		"Stop": {{
			Hooks: handler("Hookline stop review"),
		}},
	}}
}

func precommitRecipeIDs(manifests []recipes.Manifest) []string {
	var ids []string
	for _, manifest := range manifests {
		if recipes.HasSurface(manifest, "precommit") {
			ids = append(ids, manifest.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func buildPreCommit(ids []string, registry recipes.Registry) []byte {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env sh\n")
	b.WriteString("# " + managedByHookline + ". Rerun hookline init to update.\n")
	b.WriteString("set -eu\n\n")
	b.WriteString("root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)\n")
	b.WriteString("cd \"$root\"\n\n")
	if containsRecipe(ids, recipes.SecretsGitleaks) {
		b.WriteString(`run_hookline() {
  if command -v hookline >/dev/null 2>&1; then
    hookline "$@"
  elif [ -f cmd/hookline/main.go ]; then
    go run ./cmd/hookline "$@"
  else
    echo "hookline binary not found; install hookline or run from the source repo" >&2
    return 1
  fi
}

`)
	}
	for _, id := range ids {
		switch id {
		case recipes.Coherence:
			b.WriteString(`if [ "${COHERENCE_OFF:-0}" != "1" ]; then
  if ! command -v coherence >/dev/null 2>&1 && [ -x "$HOME/go/bin/coherence" ]; then
    PATH="$HOME/go/bin:$PATH"
    export PATH
  fi
  if command -v coherence >/dev/null 2>&1; then
    coherence scan --staged
  else
    echo "coherence: binary not found on PATH; skipping pre-commit checks" >&2
  fi
fi

`)
		case recipes.SecretsGitleaks:
			b.WriteString("run_hookline scan secrets\n\n")
		default:
			if manifest, ok := registry.Get(id); ok {
				for _, command := range manifest.Commands["precommit"] {
					b.WriteString(shellJoin(command.Args) + "\n")
				}
				if len(manifest.Commands["precommit"]) > 0 {
					b.WriteString("\n")
				}
			}
		}
	}
	return []byte(b.String())
}

func writeManagedFile(path string, data []byte, mode os.FileMode, force bool) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, data) {
			return false, nil
		}
		if !force && !bytes.Contains(existing, []byte(managedByHookline)) {
			return false, fmt.Errorf("%s already exists and is not managed by hookline; rerun with --force to replace it", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return false, err
	}
	return true, os.Chmod(path, mode)
}

func setGitConfig(root, key, value string) error {
	cmd := exec.Command("git", "config", key, value)
	cmd.Dir = root
	return cmd.Run()
}

func runCoherenceInit(root string) (initResult, error) {
	result := initResult{
		RecipeID: recipes.Coherence,
		Scope:    "project",
		Action:   "run-coherence-init",
		Message:  "coherence init --skill-install=auto",
	}
	path, ok := findExecutable("coherence")
	if !ok {
		result.Message = "coherence binary not found; run coherence init --skill-install=auto"
		return result, nil
	}
	cmd := exec.Command(path, "init", "--skill-install=auto")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("coherence init failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	output := string(out)
	result.Changed = strings.Contains(output, "created") || strings.Contains(output, "updated")
	return result, nil
}

func findExecutable(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, "go", "bin", name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, true
		}
	}
	return "", false
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func containsRecipe(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
