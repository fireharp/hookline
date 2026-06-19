package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fireharp/hookline/internal/codexhooks"
	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
	"gopkg.in/yaml.v3"
)

type importOptions struct {
	From    string
	ID      string
	Title   string
	Enable  bool
	Install bool
	Force   bool
	JSON    bool
}

type importCLIResult struct {
	RecipeID string                     `json:"recipe_id,omitempty"`
	Action   string                     `json:"action"`
	Path     string                     `json:"path,omitempty"`
	Changed  bool                       `json:"changed"`
	Imported int                        `json:"imported,omitempty"`
	Skipped  []codexhooks.ImportWarning `json:"skipped,omitempty"`
	Message  string                     `json:"message,omitempty"`
}

func importCommand(args []string, stdout io.Writer, root string) error {
	if len(args) == 0 || args[0] != "codex-hooks" {
		return errors.New("usage: hookline import codex-hooks [--from <path-or-url>] [--id <id>] [--enable] [--install] [--force] [--json]")
	}
	opts, err := parseImportOptions(args[1:], root)
	if err != nil {
		return err
	}
	data, source, err := readImportSource(opts.From)
	if err != nil {
		return err
	}
	id := opts.ID
	if id == "" {
		id = codexhooks.DefaultID(source)
	}
	result, err := codexhooks.Import(data, id, opts.Title, source)
	if err != nil {
		return err
	}
	recipePath := filepath.Join(config.ProjectRecipesPath(root), result.Manifest.ID+".yaml")
	changed, err := writeImportedRecipe(recipePath, result.Manifest, opts.Force)
	if err != nil {
		return err
	}
	results := []importCLIResult{{
		RecipeID: result.Manifest.ID,
		Action:   "write-recipe",
		Path:     recipePath,
		Changed:  changed,
		Imported: result.Imported,
		Skipped:  result.Skipped,
	}}
	if opts.Install {
		opts.Enable = true
	}
	if opts.Enable {
		ids := []string{result.Manifest.ID}
		if opts.Install {
			ids = append(ids, recipes.CodexHooks)
		}
		configPath := config.ProjectConfigPath(root)
		changed, err := ensureRecipesEnabled(configPath, ids)
		if err != nil {
			return err
		}
		results = append(results, importCLIResult{Action: "enable-recipes", Path: configPath, Changed: changed, Message: strings.Join(ids, ", ")})
	}
	if opts.Install {
		path := filepath.Join(root, ".codex", "hooks.json")
		changed, err := writeHookFile(path, commandForScope(defaultHookCommand(), "project"), opts.Force)
		if err != nil {
			return err
		}
		results = append(results, importCLIResult{RecipeID: recipes.CodexHooks, Action: "write-codex-hooks", Path: path, Changed: changed})
	}
	if opts.JSON {
		return writeJSON(stdout, results)
	}
	for _, result := range results {
		action := "already current"
		if result.Changed {
			action = result.Action
		}
		fmt.Fprintf(stdout, "%s: %s\n", action, result.Path)
	}
	return nil
}

func parseImportOptions(args []string, root string) (importOptions, error) {
	opts := importOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--from" && i+1 < len(args):
			i++
			opts.From = args[i]
		case strings.HasPrefix(arg, "--from="):
			opts.From = strings.TrimPrefix(arg, "--from=")
		case arg == "--id" && i+1 < len(args):
			i++
			opts.ID = args[i]
		case strings.HasPrefix(arg, "--id="):
			opts.ID = strings.TrimPrefix(arg, "--id=")
		case arg == "--title" && i+1 < len(args):
			i++
			opts.Title = args[i]
		case strings.HasPrefix(arg, "--title="):
			opts.Title = strings.TrimPrefix(arg, "--title=")
		case arg == "--enable":
			opts.Enable = true
		case arg == "--install":
			opts.Install = true
		case arg == "--force":
			opts.Force = true
		case arg == "--json":
			opts.JSON = true
		default:
			return importOptions{}, fmt.Errorf("unknown import option %q", arg)
		}
	}
	if opts.From == "" {
		from, err := firstCodexHooksFile(root)
		if err != nil {
			return importOptions{}, err
		}
		opts.From = from
	}
	return opts, nil
}

func firstCodexHooksFile(root string) (string, error) {
	for _, name := range []string{"hooks.json", "hooks.yaml", "hooks.yml"} {
		path := filepath.Join(root, ".codex", name)
		if fileExists(path) {
			return path, nil
		}
	}
	return "", errors.New("no .codex/hooks.json, hooks.yaml, or hooks.yml found; pass --from")
}

func readImportSource(source string) ([]byte, string, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return readImportURL(source)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, "", err
	}
	abs, err := filepath.Abs(source)
	if err == nil {
		source = abs
	}
	return data, source, nil
}

func readImportURL(source string) ([]byte, string, error) {
	source = rawGitHubURL(source)
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(source)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("%s returned %s", source, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, "", err
	}
	return data, source, nil
}

func rawGitHubURL(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host != "github.com" {
		return source
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 5 || parts[2] != "blob" {
		return source
	}
	return "https://raw.githubusercontent.com/" + parts[0] + "/" + parts[1] + "/" + parts[3] + "/" + strings.Join(parts[4:], "/")
}

func writeImportedRecipe(path string, manifest recipes.Manifest, force bool) (bool, error) {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return false, err
	}
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
