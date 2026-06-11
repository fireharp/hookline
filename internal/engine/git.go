package engine

import (
	"bufio"
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var todoMarkerPattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(TO[D]O|FIX[M]E)([^A-Za-z0-9_]|$)`)

type diffStat struct {
	Path    string
	Added   int
	Removed int
}

func changedFiles(root string) []diffStat {
	stats := diffNumstat(root)
	seen := map[string]bool{}
	for _, stat := range stats {
		seen[stat.Path] = true
	}
	for _, path := range untracked(root) {
		if !seen[path] {
			stats = append(stats, diffStat{Path: path})
		}
	}
	return stats
}

func diffNumstat(root string) []diffStat {
	cmd := exec.Command("git", "diff", "--numstat", "HEAD", "--")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var stats []diffStat
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		removed, _ := strconv.Atoi(fields[1])
		stats = append(stats, diffStat{Path: fields[2], Added: added, Removed: removed})
	}
	return stats
}

func untracked(root string) []string {
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if text := strings.TrimSpace(scanner.Text()); text != "" {
			files = append(files, text)
		}
	}
	return files
}

func addedTodoLines(root string) bool {
	cmd := exec.Command("git", "diff", "--unified=0", "HEAD", "--")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	currentPath := ""
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ b/") {
			currentPath = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "diff --git ") {
			currentPath = ""
			continue
		}
		if strings.HasPrefix(line, "+++") || !strings.HasPrefix(line, "+") {
			continue
		}
		if todoCheckExemptPath(currentPath) {
			continue
		}
		if todoMarkerPattern.MatchString(line) {
			return true
		}
	}
	return false
}

func todoCheckExemptPath(path string) bool {
	return isTest(path) ||
		strings.HasPrefix(path, "docs/cases/") ||
		strings.HasPrefix(path, "internal/bench/")
}
