package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type hookOptions struct {
	Source string
}

func parseHookOptions(args []string) (hookOptions, error) {
	if len(args) == 0 || args[0] != "codex" {
		return hookOptions{}, errors.New("usage: hookline hook codex [--source user|project|auto]")
	}
	opts := hookOptions{Source: "auto"}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--source" && i+1 < len(args):
			i++
			opts.Source = args[i]
		case strings.HasPrefix(arg, "--source="):
			opts.Source = strings.TrimPrefix(arg, "--source=")
		default:
			return hookOptions{}, fmt.Errorf("unknown hook option %q", arg)
		}
	}
	if opts.Source != "user" && opts.Source != "project" && opts.Source != "auto" {
		return hookOptions{}, errors.New("hook source must be user, project, or auto")
	}
	return opts, nil
}

func projectHookOverridesUser(root, source string) bool {
	return source == "user" && projectHookHasTaggedSource(root)
}

func projectHookHasTaggedSource(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, ".codex", "hooks.json"))
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, "hook codex") && strings.Contains(text, "--source project")
}
