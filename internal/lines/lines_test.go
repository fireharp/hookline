package lines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestScanReportsOversizedTrackedStyleFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "big.go")
	if err := os.WriteFile(path, []byte(strings.Repeat("package main\n", 4)), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root, config.LimitsConfig{FileLineLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Violations) != 1 {
		t.Fatalf("expected one violation, got %#v", result.Violations)
	}
}
