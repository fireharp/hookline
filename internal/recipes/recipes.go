package recipes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fireharp/hookline/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	Coherence        = "coherence"
	SecretsGitleaks  = "secrets-gitleaks"
	LineCount        = "line-count"
	CodexHooks       = "codex-hooks"
	AgentSteering    = "agent-steering"
	RTKExplicitProxy = "rtk-explicit-proxy"
)

type Manifest struct {
	ID           string               `yaml:"id" json:"id"`
	Title        string               `yaml:"title" json:"title,omitempty"`
	Description  string               `yaml:"description" json:"description,omitempty"`
	Surfaces     []string             `yaml:"surfaces" json:"surfaces,omitempty"`
	Commands     map[string][]Command `yaml:"commands" json:"commands,omitempty"`
	CodexHooks   []CodexHook          `yaml:"codex_hooks" json:"codex_hooks,omitempty"`
	ManagedFiles []ManagedFile        `yaml:"managed_files" json:"managed_files,omitempty"`
	Source       string               `yaml:"-" json:"source,omitempty"`
}

type Command struct {
	Args []string `yaml:"args" json:"args"`
}

type CodexHook struct {
	Event          string `yaml:"event" json:"event"`
	Matcher        string `yaml:"matcher,omitempty" json:"matcher,omitempty"`
	Type           string `yaml:"type" json:"type"`
	Command        string `yaml:"command" json:"command"`
	CommandWindows string `yaml:"command_windows,omitempty" json:"command_windows,omitempty"`
	Timeout        int    `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	StatusMessage  string `yaml:"status_message,omitempty" json:"status_message,omitempty"`
}

type ManagedFile struct {
	Path    string `yaml:"path" json:"path"`
	Mode    string `yaml:"mode" json:"mode,omitempty"`
	Action  string `yaml:"action" json:"action,omitempty"`
	Content string `yaml:"content" json:"-"`
}

type Registry struct {
	byID       map[string]Manifest
	order      []string
	enabled    map[string]bool
	enabledIDs []string
}

type Listing struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Surfaces    []string `json:"surfaces,omitempty"`
	Enabled     bool     `json:"enabled"`
	Source      string   `json:"source,omitempty"`
}

type CommandResult struct {
	RecipeID string   `json:"recipe_id"`
	Surface  string   `json:"surface"`
	Args     []string `json:"args"`
	OK       bool     `json:"ok"`
	Output   string   `json:"output,omitempty"`
	Error    string   `json:"error,omitempty"`
}

func StandardRecipeIDs() []string {
	return []string{CodexHooks, Coherence, SecretsGitleaks, LineCount, AgentSteering}
}

func Load(root string, cfg config.Config) (Registry, error) {
	reg := Registry{byID: map[string]Manifest{}, enabled: map[string]bool{}}
	if err := reg.loadStandard(); err != nil {
		return Registry{}, err
	}
	for _, path := range defaultRecipePaths(root) {
		if err := reg.loadPath(path); err != nil {
			return Registry{}, err
		}
	}
	for _, path := range cfg.Recipes.Paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if err := reg.loadPath(path); err != nil {
			return Registry{}, err
		}
	}
	for _, id := range unique(cfg.Recipes.Enabled) {
		if _, ok := reg.byID[id]; !ok {
			return Registry{}, fmt.Errorf("enabled recipe %q is not loaded", id)
		}
		reg.enabled[id] = true
		reg.enabledIDs = append(reg.enabledIDs, id)
	}
	return reg, nil
}

func (r Registry) Get(id string) (Manifest, bool) {
	m, ok := r.byID[id]
	return m, ok
}

func (r Registry) Enabled(id string) bool {
	return r.enabled[id]
}

func (r Registry) EnabledIDs() []string {
	return append([]string(nil), r.enabledIDs...)
}

func (r Registry) AnyEnabled() bool {
	return len(r.enabledIDs) > 0
}

func (r Registry) EnabledManifests() []Manifest {
	var manifests []Manifest
	for _, id := range r.enabledIDs {
		if m, ok := r.byID[id]; ok {
			manifests = append(manifests, m)
		}
	}
	return manifests
}

func (r Registry) HasEnabledSurface(surface string) bool {
	for _, m := range r.EnabledManifests() {
		if HasSurface(m, surface) {
			return true
		}
	}
	return false
}

func (r Registry) List() []Listing {
	var out []Listing
	for _, id := range r.order {
		m := r.byID[id]
		out = append(out, Listing{
			ID:          m.ID,
			Title:       m.Title,
			Description: m.Description,
			Surfaces:    append([]string(nil), m.Surfaces...),
			Enabled:     r.enabled[m.ID],
			Source:      m.Source,
		})
	}
	return out
}

func HasSurface(m Manifest, surface string) bool {
	for _, s := range m.Surfaces {
		if s == surface {
			return true
		}
	}
	if len(m.Commands[surface]) > 0 {
		return true
	}
	return false
}

func RunCommands(ctx context.Context, root string, m Manifest, surface string) []CommandResult {
	var results []CommandResult
	for _, command := range m.Commands[surface] {
		result := CommandResult{RecipeID: m.ID, Surface: surface, Args: append([]string(nil), command.Args...)}
		if len(command.Args) == 0 {
			result.Error = "empty command"
			results = append(results, result)
			continue
		}
		cmd := exec.CommandContext(ctx, command.Args[0], command.Args[1:]...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		result.Output = strings.TrimSpace(string(out))
		if err != nil {
			result.Error = err.Error()
			if result.Output != "" {
				result.Error = result.Output
			}
		} else {
			result.OK = true
		}
		results = append(results, result)
	}
	return results
}

func defaultRecipePaths(root string) []string {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, config.UserRecipesPath(home))
	}
	paths = append(paths, config.ProjectRecipesPath(root))
	return paths
}

func (r *Registry) loadPath(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() || !isYAML(entry.Name()) {
				continue
			}
			if err := r.loadFile(filepath.Join(path, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !isYAML(path) {
		return nil
	}
	return r.loadFile(path)
}

func (r *Registry) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return r.loadBytes(data, path)
}

func (r *Registry) loadStandard() error {
	for _, manifest := range standardRecipeManifests {
		if err := r.loadBytes([]byte(manifest), "builtin"); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) loadBytes(data []byte, source string) error {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("%s: recipe id is required", source)
	}
	for surface, commands := range m.Commands {
		for i, command := range commands {
			if len(command.Args) == 0 {
				return fmt.Errorf("%s: command %s[%d] has no args", source, surface, i)
			}
		}
		if !contains(m.Surfaces, surface) {
			m.Surfaces = append(m.Surfaces, surface)
		}
	}
	for i, hook := range m.CodexHooks {
		if strings.TrimSpace(hook.Event) == "" {
			return fmt.Errorf("%s: codex_hooks[%d] event is required", source, i)
		}
		if strings.TrimSpace(hook.Command) == "" {
			return fmt.Errorf("%s: codex_hooks[%d] command is required", source, i)
		}
		if strings.TrimSpace(hook.Type) == "" {
			hook.Type = "command"
		}
		if hook.Type != "command" {
			return fmt.Errorf("%s: codex_hooks[%d] type %q is not supported", source, i, hook.Type)
		}
		if hook.Timeout < 0 {
			return fmt.Errorf("%s: codex_hooks[%d] timeout must be non-negative", source, i)
		}
		m.CodexHooks[i] = hook
	}
	if len(m.CodexHooks) > 0 && !contains(m.Surfaces, "hook") {
		m.Surfaces = append(m.Surfaces, "hook")
	}
	for i, file := range m.ManagedFiles {
		if strings.TrimSpace(file.Path) == "" {
			return fmt.Errorf("%s: managed_files[%d] path is required", source, i)
		}
		if filepath.IsAbs(file.Path) {
			return fmt.Errorf("%s: managed_files[%d] path must be relative", source, i)
		}
		clean := filepath.Clean(file.Path)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("%s: managed_files[%d] path must stay inside the project", source, i)
		}
		m.ManagedFiles[i].Path = clean
	}
	sort.Strings(m.Surfaces)
	m.Source = source
	if _, exists := r.byID[m.ID]; !exists {
		r.order = append(r.order, m.ID)
	}
	r.byID[m.ID] = m
	return nil
}

func isYAML(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}

func unique(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
