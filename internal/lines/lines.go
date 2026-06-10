package lines

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fireharp/hookline/internal/config"
)

type Result struct {
	Root       string      `json:"root"`
	Limit      int         `json:"limit"`
	Violations []Violation `json:"violations"`
}

type Violation struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
	Limit int    `json:"limit"`
}

func Scan(root string, limits config.LimitsConfig) (Result, error) {
	files, err := gitFiles(root)
	if err != nil {
		files, err = walkFiles(root)
		if err != nil {
			return Result{}, err
		}
	}
	result := Result{Root: root, Limit: limits.FileLineLimit}
	for _, rel := range files {
		if !scannable(rel) {
			continue
		}
		n, err := countLines(filepath.Join(root, rel))
		if err != nil {
			return Result{}, err
		}
		limit := limits.FileLineLimit
		if limit <= 0 {
			limit = 500
		}
		if n > limit {
			result.Violations = append(result.Violations, Violation{Path: rel, Lines: n, Limit: limit})
		}
	}
	return result, nil
}

func gitFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if text := strings.TrimSpace(scanner.Text()); text != "" {
			files = append(files, filepath.Clean(text))
		}
	}
	return files, scanner.Err()
}

func walkFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && skipDir(entry.Name()) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

func scannable(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rs", ".md", ".sh", ".yaml", ".yml", ".toml", ".json":
		return true
	default:
		return false
	}
}

func skipDir(name string) bool {
	switch name {
	case ".git", ".coherence", ".venv", ".cache", "bin", "build", "dist", "node_modules", "vendor":
		return true
	default:
		return false
	}
}
