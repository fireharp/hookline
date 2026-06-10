package secrets

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fireharp/hookline/internal/config"
)

type Result struct {
	EnvLeaks      []EnvLeak `json:"env_leaks"`
	GitleaksRun   bool      `json:"gitleaks_run"`
	GitleaksError string    `json:"gitleaks_error,omitempty"`
}

type EnvLeak struct {
	Key      string `json:"key"`
	Redacted string `json:"redacted"`
	Count    int    `json:"count"`
}

func ScanStaged(ctx context.Context, root string, cfg config.SecretsConfig) (Result, error) {
	leaks, err := EnvLeaks(root, cfg)
	if err != nil {
		return Result{}, err
	}
	result := Result{EnvLeaks: leaks}
	if len(leaks) > 0 {
		return result, nil
	}
	if cfg.RunGitleaks == nil || *cfg.RunGitleaks {
		result.GitleaksRun = true
		if err := runGitleaks(ctx, root, cfg.GitleaksCommand); err != nil {
			result.GitleaksError = err.Error()
			return result, err
		}
	}
	return result, nil
}

func EnvLeaks(root string, cfg config.SecretsConfig) ([]EnvLeak, error) {
	envFile := cfg.EnvFile
	if envFile == "" {
		envFile = ".env"
	}
	minLen := cfg.MinValueLength
	if minLen <= 0 {
		minLen = 6
	}
	values, err := readEnvValues(filepath.Join(root, envFile), minLen, cfg.AllowedEnvKeys)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	diff, err := stagedDiff(root)
	if err != nil {
		return nil, err
	}
	if diff == "" {
		return nil, nil
	}
	added := addedLines(diff)
	var leaks []EnvLeak
	for _, value := range values {
		count := 0
		for _, line := range added {
			if strings.Contains(line, value.Value) {
				count++
			}
		}
		if count > 0 {
			leaks = append(leaks, EnvLeak{Key: value.Key, Redacted: redact(value.Value), Count: count})
		}
	}
	return leaks, nil
}

type envValue struct {
	Key   string
	Value string
}

func readEnvValues(path string, minLen int, allowed []string) ([]envValue, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	allowedKeys := map[string]bool{}
	for _, key := range allowed {
		allowedKeys[key] = true
	}
	var values []envValue
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimSuffix(key, "?"))
		if allowedKeys[key] {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if len(value) >= minLen {
			values = append(values, envValue{Key: key, Value: value})
		}
	}
	return values, scanner.Err()
}

func stagedDiff(root string) (string, error) {
	args := []string{
		"diff", "--cached", "--unified=0", "--",
		":(exclude).env", ":(exclude).env.*", ":(exclude).env.example",
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func addedLines(diff string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++") || !strings.HasPrefix(line, "+") {
			continue
		}
		lines = append(lines, strings.TrimPrefix(line, "+"))
	}
	return lines
}

func redact(value string) string {
	if len(value) <= 8 {
		return fmt.Sprintf("<redacted:%d chars>", len(value))
	}
	return fmt.Sprintf("%s...%s (%d chars)", value[:3], value[len(value)-3:], len(value))
}

func runGitleaks(ctx context.Context, root string, args []string) error {
	if len(args) == 0 {
		args = []string{"gitleaks", "git", "--pre-commit", "--staged", "--redact", "--no-banner", "."}
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}
