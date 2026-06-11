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
	Root             string    `json:"root"`
	Limit            int       `json:"limit"`
	SplitReviewLimit int       `json:"split_review_limit,omitempty"`
	Findings         []Finding `json:"findings,omitempty"`
}

type Finding struct {
	Path     string `json:"path"`
	Lines    int    `json:"lines"`
	Limit    int    `json:"limit"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func Scan(root string, limits config.LimitsConfig) (Result, error) {
	files, err := gitFiles(root)
	if err != nil {
		files, err = walkFiles(root)
		if err != nil {
			return Result{}, err
		}
	}
	result := Result{Root: root, Limit: limits.FileLineLimit, SplitReviewLimit: limits.SplitReviewLineLimit}
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
			severity := "advisory"
			message := "File is over the soft line-count target; review whether to split or keep cohesive."
			if limits.SplitReviewLineLimit > 0 && n >= limits.SplitReviewLineLimit {
				severity = "split-review"
				message = "File is far over the soft line-count target; split it or explicitly justify keeping it cohesive."
			}
			result.Findings = append(result.Findings, Finding{
				Path:     rel,
				Lines:    n,
				Limit:    limit,
				Severity: severity,
				Message:  message,
			})
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
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rs", ".md", ".mdx", ".sh", ".yaml", ".yml", ".toml", ".json":
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
