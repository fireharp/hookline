package config

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Limits         LimitsConfig  `yaml:"limits"`
	Secrets        SecretsConfig `yaml:"secrets"`
	DangerousShell []string      `yaml:"dangerous_shell"`
	SensitivePaths []string      `yaml:"sensitive_paths"`
	SkillTriggers  []SkillRule   `yaml:"skill_triggers"`
}

type LimitsConfig struct {
	FileLineLimit    int `yaml:"file_line_limit"`
	NewFileLineLimit int `yaml:"new_file_line_limit"`
	LargeDiffAdded   int `yaml:"large_diff_added"`
}

type SecretsConfig struct {
	EnvFile         string   `yaml:"env_file"`
	MinValueLength  int      `yaml:"min_value_length"`
	AllowedEnvKeys  []string `yaml:"allowed_env_keys"`
	RunGitleaks     *bool    `yaml:"run_gitleaks"`
	GitleaksCommand []string `yaml:"gitleaks_command"`
}

type SkillRule struct {
	ID       string   `yaml:"id"`
	Regex    string   `yaml:"regex"`
	Skills   []string `yaml:"skills"`
	Message  string   `yaml:"message"`
	Severity string   `yaml:"severity"`
}

func Default() Config {
	runGitleaks := true
	return Config{
		Limits: LimitsConfig{
			FileLineLimit:    500,
			NewFileLineLimit: 300,
			LargeDiffAdded:   700,
		},
		Secrets: SecretsConfig{
			EnvFile:        ".env",
			MinValueLength: 6,
			RunGitleaks:    &runGitleaks,
			GitleaksCommand: []string{
				"gitleaks", "git", "--pre-commit", "--staged", "--redact", "--no-banner", ".",
			},
		},
		DangerousShell: []string{
			`rm\s+-rf\s+(/|\$HOME|~|\*)`,
			`chmod\s+777`,
			`curl\b.*\|\s*(sh|bash|zsh)`,
			`wget\b.*\|\s*(sh|bash|zsh)`,
			`\bsudo\b`,
		},
		SensitivePaths: []string{
			"**/auth/**",
			"**/billing/**",
			"**/chargeback/**",
			"**/ledger/**",
			"**/payments/**",
			"**/security/**",
			"**/secrets/**",
		},
		SkillTriggers: []SkillRule{
			{
				ID:       "temporal-workflow-skill",
				Regex:    `(?i)(temporal|workflow|long-running|saga|compensation|durable execution)`,
				Skills:   []string{"temporal-workflow-design", "idempotency-and-compensation"},
				Severity: "info",
				Message:  "Relevant skills: temporal-workflow-design, idempotency-and-compensation. Read them before changing workflow logic.",
			},
		},
	}
}

func Load(root string) (Config, error) {
	cfg := Default()
	if home, err := os.UserHomeDir(); err == nil {
		_ = mergeFile(&cfg, filepath.Join(home, ".fireharp", "hookline.yaml"))
	}
	_ = mergeFile(&cfg, filepath.Join(root, ".fireharp", "harness.yaml"))
	return cfg, nil
}

func FindRoot(start string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = start
	out, err := cmd.Output()
	if err == nil {
		return filepath.Clean(stringTrimSpace(out)), nil
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var next Config
	if err := yaml.Unmarshal(data, &next); err != nil {
		return err
	}
	merge(cfg, next)
	return nil
}

func merge(dst *Config, src Config) {
	if src.Limits.FileLineLimit != 0 {
		dst.Limits.FileLineLimit = src.Limits.FileLineLimit
	}
	if src.Limits.NewFileLineLimit != 0 {
		dst.Limits.NewFileLineLimit = src.Limits.NewFileLineLimit
	}
	if src.Limits.LargeDiffAdded != 0 {
		dst.Limits.LargeDiffAdded = src.Limits.LargeDiffAdded
	}
	if src.Secrets.EnvFile != "" {
		dst.Secrets.EnvFile = src.Secrets.EnvFile
	}
	if src.Secrets.MinValueLength != 0 {
		dst.Secrets.MinValueLength = src.Secrets.MinValueLength
	}
	if src.Secrets.RunGitleaks != nil {
		dst.Secrets.RunGitleaks = src.Secrets.RunGitleaks
	}
	if len(src.Secrets.GitleaksCommand) > 0 {
		dst.Secrets.GitleaksCommand = src.Secrets.GitleaksCommand
	}
	if len(src.Secrets.AllowedEnvKeys) > 0 {
		dst.Secrets.AllowedEnvKeys = src.Secrets.AllowedEnvKeys
	}
	if len(src.DangerousShell) > 0 {
		dst.DangerousShell = src.DangerousShell
	}
	if len(src.SensitivePaths) > 0 {
		dst.SensitivePaths = src.SensitivePaths
	}
	if len(src.SkillTriggers) > 0 {
		dst.SkillTriggers = src.SkillTriggers
	}
}

func stringTrimSpace(data []byte) string {
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r' || data[len(data)-1] == '\t' || data[len(data)-1] == ' ') {
		data = data[:len(data)-1]
	}
	return string(data)
}
