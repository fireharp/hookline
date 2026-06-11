package lines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestScanReportsSoftFindingForLargeFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "big.go")
	if err := os.WriteFile(path, []byte(strings.Repeat("package main\n", 4)), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root, config.LimitsConfig{FileLineLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected one advisory finding, got %#v", result.Findings)
	}
	if result.Findings[0].Severity != "advisory" {
		t.Fatalf("expected advisory severity, got %#v", result.Findings[0])
	}
}

func TestScanReportsSplitReviewForVeryLargeFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "huge.ts")
	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 6)), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root, config.LimitsConfig{FileLineLimit: 3, SplitReviewLineLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected one finding, got %#v", result.Findings)
	}
	if result.Findings[0].Severity != "split-review" {
		t.Fatalf("expected split-review severity, got %#v", result.Findings[0])
	}
}

func TestScanIncludesMDXFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "case.mdx")
	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 4)), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root, config.LimitsConfig{FileLineLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Path != "case.mdx" {
		t.Fatalf("expected MDX finding, got %#v", result.Findings)
	}
}
