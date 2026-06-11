package recipes

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fireharp/hookline/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	Coherence       = "coherence"
	SecretsGitleaks = "secrets-gitleaks"
	LineCount       = "line-count"
	CodexHooks      = "codex-hooks"
	AgentSteering   = "agent-steering"
)

//go:embed bundled/*.yaml
var bundled embed.FS

type Manifest struct {
	ID          string               `yaml:"id" json:"id"`
	Title       string               `yaml:"title" json:"title,omitempty"`
	Description string               `yaml:"description" json:"description,omitempty"`
	Surfaces    []string             `yaml:"surfaces" json:"surfaces,omitempty"`
	Commands    map[string][]Command `yaml:"commands" json:"commands,omitempty"`
	Source      string               `yaml:"-" json:"source,omitempty"`
}

type Command struct {
	Args []string `yaml:"args" json:"args"`
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
	if err := loadBundled(&reg); err != nil {
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

func loadBundled(reg *Registry) error {
	return fs.WalkDir(bundled, "bundled", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		data, err := bundled.ReadFile(path)
		if err != nil {
			return err
		}
		return reg.loadBytes(data, path)
	})
}

func defaultRecipePaths(root string) []string {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".fireharp", "hookline", "recipes"))
	}
	paths = append(paths, filepath.Join(root, ".fireharp", "hookline", "recipes"))
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
