package secrets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestEnvLeaksRedactsLocalValues(t *testing.T) {
	root := t.TempDir()
	mustGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("LOCAL_VALUE=abcdef123456\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("value=abcdef123456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "README.md")
	leaks, err := EnvLeaks(root, config.Default().Secrets)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaks) != 1 {
		t.Fatalf("expected one leak, got %#v", leaks)
	}
	if strings.Contains(leaks[0].Redacted, "abcdef123456") {
		t.Fatalf("redacted output leaked value: %s", leaks[0].Redacted)
	}
}

func mustGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
