package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func defaultHookCommand() string {
	if path, err := exec.LookPath("hookline"); err == nil {
		return shellQuote(path) + " hook codex"
	}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, "go", "bin", "hookline")
		if fileExists(path) {
			return shellQuote(path) + " hook codex"
		}
	}
	if root, ok := sourceCommandRoot(); ok {
		return "cd " + shellQuote(root) + " && go run ./cmd/hookline hook codex"
	}
	return "hookline hook codex"
}

func sourceCommandRoot() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	dir := filepath.Dir(file)
	if filepath.Base(dir) != "hookline" || filepath.Base(filepath.Dir(dir)) != "cmd" {
		return "", false
	}
	root := filepath.Dir(filepath.Dir(dir))
	if !fileExists(filepath.Join(root, "go.mod")) {
		return "", false
	}
	return root, true
}

func shellJoin(args []string) string {
	var parts []string
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}
