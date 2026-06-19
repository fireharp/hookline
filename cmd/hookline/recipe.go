package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
)

type recipeLifecycleOptions struct {
	Scope        string
	Force        bool
	PruneManaged bool
	JSON         bool
	Recipes      []string
}

func recipeCommand(args []string, stdout io.Writer, root string, cfg config.Config, registry recipes.Registry) error {
	if len(args) == 0 {
		return errors.New("usage: hookline recipe list|enable|disable")
	}
	switch args[0] {
	case "list":
		jsonOut := hasFlag(args, "--json")
		list := registry.List()
		if jsonOut {
			return writeJSON(stdout, list)
		}
		for _, item := range list {
			status := "available"
			if item.Enabled {
				status = "enabled"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", item.ID, status, item.Title)
		}
		return nil
	case "enable":
		opts, err := parseRecipeLifecycleOptions(args[1:], false)
		if err != nil {
			return err
		}
		return enableRecipes(stdout, root, opts, registry)
	case "disable":
		opts, err := parseRecipeLifecycleOptions(args[1:], true)
		if err != nil {
			return err
		}
		return disableRecipes(stdout, root, opts, registry)
	default:
		return errors.New("usage: hookline recipe list|enable|disable")
	}
}

func parseRecipeLifecycleOptions(args []string, allowPrune bool) (recipeLifecycleOptions, error) {
	opts := recipeLifecycleOptions{Scope: "project"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--scope" && i+1 < len(args):
			i++
			opts.Scope = args[i]
		case strings.HasPrefix(arg, "--scope="):
			opts.Scope = strings.TrimPrefix(arg, "--scope=")
		case arg == "--recipe" && i+1 < len(args):
			i++
			opts.Recipes = append(opts.Recipes, splitCSV(args[i])...)
		case strings.HasPrefix(arg, "--recipe="):
			opts.Recipes = append(opts.Recipes, splitCSV(strings.TrimPrefix(arg, "--recipe="))...)
		case arg == "--force":
			opts.Force = true
		case arg == "--json":
			opts.JSON = true
		case arg == "--prune-managed" && allowPrune:
			opts.PruneManaged = true
		case strings.HasPrefix(arg, "--"):
			return recipeLifecycleOptions{}, fmt.Errorf("unknown recipe option %q", arg)
		default:
			opts.Recipes = append(opts.Recipes, splitCSV(arg)...)
		}
	}
	if opts.Scope != "project" && opts.Scope != "user" && opts.Scope != "both" {
		return recipeLifecycleOptions{}, errors.New("usage: hookline recipe enable|disable <id>... [--scope project|user|both]")
	}
	opts.Recipes = uniqueStrings(opts.Recipes)
	if len(opts.Recipes) == 0 {
		return recipeLifecycleOptions{}, errors.New("recipe command requires at least one recipe id")
	}
	return opts, nil
}

func enableRecipes(stdout io.Writer, root string, opts recipeLifecycleOptions, registry recipes.Registry) error {
	manifests, err := recipeManifests(opts.Recipes, registry)
	if err != nil {
		return err
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
	if opts.Scope != "user" {
		applied, err := applyManagedRecipeFiles(root, manifests, opts.Force)
		if err != nil {
			return err
		}
		results = append(results, applied...)
	}
	return writeRecipeLifecycleResults(stdout, opts.JSON, results)
}

func disableRecipes(stdout io.Writer, root string, opts recipeLifecycleOptions, registry recipes.Registry) error {
	manifests, err := recipeManifests(opts.Recipes, registry)
	if err != nil {
		return err
	}
	var results []initResult
	for _, scope := range initScopes(opts.Scope) {
		path, err := configPath(scope, root)
		if err != nil {
			return err
		}
		changed, err := removeRecipesFromConfig(path, opts.Recipes)
		if err != nil {
			return err
		}
		results = append(results, initResult{Scope: scope, Path: path, Action: "disable-recipes", Changed: changed, Message: strings.Join(opts.Recipes, ", ")})
	}
	if opts.PruneManaged && opts.Scope != "user" {
		pruned, err := pruneManagedRecipeFiles(root, manifests)
		if err != nil {
			return err
		}
		results = append(results, pruned...)
	}
	return writeRecipeLifecycleResults(stdout, opts.JSON, results)
}

func recipeManifests(ids []string, registry recipes.Registry) ([]recipes.Manifest, error) {
	var manifests []recipes.Manifest
	for _, id := range ids {
		manifest, ok := registry.Get(id)
		if !ok {
			return nil, fmt.Errorf("recipe %q is not loaded", id)
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func applyManagedRecipeFiles(root string, manifests []recipes.Manifest, force bool) ([]initResult, error) {
	var results []initResult
	for _, manifest := range manifests {
		for _, file := range manifest.ManagedFiles {
			path := filepath.Join(root, file.Path)
			mode, err := managedFileMode(file)
			if err != nil {
				return nil, err
			}
			changed, err := writeManagedFile(path, []byte(file.Content), mode, force)
			if err != nil {
				return nil, err
			}
			action := file.Action
			if action == "" {
				action = "write-managed-file"
			}
			results = append(results, initResult{RecipeID: manifest.ID, Scope: "project", Path: path, Action: action, Changed: changed})
		}
	}
	return results, nil
}

func pruneManagedRecipeFiles(root string, manifests []recipes.Manifest) ([]initResult, error) {
	var results []initResult
	for _, manifest := range manifests {
		for _, file := range manifest.ManagedFiles {
			result, err := removeManagedRecipeFile(root, manifest.ID, file.Path, "prune-managed-file")
			if err != nil {
				return nil, err
			}
			results = append(results, result)
			parent := filepath.Dir(filepath.Join(root, file.Path))
			if parent != root {
				_ = os.Remove(parent)
			}
		}
	}
	return results, nil
}

func managedFileMode(file recipes.ManagedFile) (os.FileMode, error) {
	if strings.TrimSpace(file.Mode) == "" {
		return 0o644, nil
	}
	var mode uint64
	if n, err := fmt.Sscanf(file.Mode, "%o", &mode); err != nil || n != 1 {
		return 0, fmt.Errorf("managed file %s has invalid mode %q", file.Path, file.Mode)
	}
	return os.FileMode(mode), nil
}

func removeManagedRecipeFile(root, recipeID, rel, action string) (initResult, error) {
	path := filepath.Join(root, rel)
	result := initResult{RecipeID: recipeID, Scope: "project", Path: path, Action: action}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !strings.Contains(string(data), managedByHookline) {
		return result, fmt.Errorf("%s exists and is not managed by hookline; refusing to remove", path)
	}
	if err := os.Remove(path); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}

func removeRecipesFromConfig(path string, ids []string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var kept []string
	changed := false
	for _, line := range lines {
		if recipeLineMatches(line, ids) {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	if !changed {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}

func recipeLineMatches(line string, ids []string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "- ") {
		return false
	}
	value := strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`)
	for _, id := range ids {
		if value == id {
			return true
		}
	}
	return false
}

func writeRecipeLifecycleResults(stdout io.Writer, jsonOut bool, results []initResult) error {
	if jsonOut {
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
