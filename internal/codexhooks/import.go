package codexhooks

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fireharp/hookline/internal/recipes"
	"gopkg.in/yaml.v3"
)

type File struct {
	Hooks map[string][]MatcherGroup `yaml:"hooks"`
}

type MatcherGroup struct {
	Matcher string    `yaml:"matcher"`
	Hooks   []Handler `yaml:"hooks"`
}

type Handler struct {
	Type                string `yaml:"type"`
	Command             string `yaml:"command"`
	CommandWindows      string `yaml:"commandWindows"`
	CommandWindowsSnake string `yaml:"command_windows"`
	Timeout             int    `yaml:"timeout"`
	StatusMessage       string `yaml:"statusMessage"`
	StatusMessageSnake  string `yaml:"status_message"`
	Async               bool   `yaml:"async"`
}

type ImportWarning struct {
	Event   string `json:"event"`
	Index   int    `json:"index"`
	Reason  string `json:"reason"`
	Matcher string `json:"matcher,omitempty"`
}

type ImportResult struct {
	Manifest recipes.Manifest `json:"manifest"`
	Imported int              `json:"imported"`
	Skipped  []ImportWarning  `json:"skipped,omitempty"`
}

func Parse(data []byte) (File, error) {
	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return File{}, err
	}
	if len(file.Hooks) == 0 {
		return File{}, fmt.Errorf("hooks map is required")
	}
	return file, nil
}

func Import(data []byte, id, title, source string) (ImportResult, error) {
	file, err := Parse(data)
	if err != nil {
		return ImportResult{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = "imported-codex-hooks"
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Imported Codex Hooks"
	}
	manifest := recipes.Manifest{
		ID:          id,
		Title:       title,
		Description: "Imported from " + source + ".",
		Surfaces:    []string{"hook"},
	}
	var skipped []ImportWarning
	for _, event := range sortedEvents(file.Hooks) {
		for _, group := range file.Hooks[event] {
			for i, handler := range group.Hooks {
				hook, ok, reason := importHandler(event, group.Matcher, handler)
				if !ok {
					skipped = append(skipped, ImportWarning{Event: event, Index: i, Matcher: group.Matcher, Reason: reason})
					continue
				}
				manifest.CodexHooks = append(manifest.CodexHooks, hook)
			}
		}
	}
	if len(manifest.CodexHooks) == 0 {
		return ImportResult{}, fmt.Errorf("no supported command hooks found")
	}
	return ImportResult{Manifest: manifest, Imported: len(manifest.CodexHooks), Skipped: skipped}, nil
}

func importHandler(event, matcher string, handler Handler) (recipes.CodexHook, bool, string) {
	if handler.Async {
		return recipes.CodexHook{}, false, "async hooks are parsed by Codex but not supported"
	}
	typ := strings.TrimSpace(handler.Type)
	if typ == "" && strings.TrimSpace(handler.Command) != "" {
		typ = "command"
	}
	if typ != "command" {
		return recipes.CodexHook{}, false, "only command hooks are supported"
	}
	command := strings.TrimSpace(handler.Command)
	if command == "" {
		return recipes.CodexHook{}, false, "command is required"
	}
	status := handler.StatusMessage
	if status == "" {
		status = handler.StatusMessageSnake
	}
	commandWindows := handler.CommandWindows
	if commandWindows == "" {
		commandWindows = handler.CommandWindowsSnake
	}
	return recipes.CodexHook{
		Event:          event,
		Matcher:        matcher,
		Type:           "command",
		Command:        command,
		CommandWindows: strings.TrimSpace(commandWindows),
		Timeout:        handler.Timeout,
		StatusMessage:  strings.TrimSpace(status),
	}, true, ""
}

func sortedEvents(hooks map[string][]MatcherGroup) []string {
	events := make([]string, 0, len(hooks))
	for event := range hooks {
		events = append(events, event)
	}
	sort.Strings(events)
	return events
}

func DefaultID(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(dir)
	if base == ".codex" {
		base = filepath.Base(filepath.Dir(dir))
	}
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "imported-codex"
	}
	base = strings.ToLower(base)
	base = strings.NewReplacer(" ", "-", "_", "-", ".", "-").Replace(base)
	base = strings.Trim(base, "-")
	if base == "" || base == "codex" {
		base = "imported-codex"
	}
	return base + "-hooks"
}
