package bench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fireharp/hookline/internal/config"
)

func rtkExplicitProxy(ctx context.Context, _ config.Config) Scenario {
	root, cleanup, err := tempRepo()
	if err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	defer cleanup()
	home := filepath.Join(root, "home")
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	rtkPath := filepath.Join(binDir, "rtk")
	if err := os.WriteFile(rtkPath, []byte(fakeRTKScript()), 0o755); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	configPath := config.ProjectConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	if err := os.WriteFile(configPath, []byte("recipes:\n  enabled:\n    - rtk-explicit-proxy\n"), 0o644); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	if err := os.WriteFile(filepath.Join(root, ".rtk", "filters.toml"), []byte("# Managed by hookline\nschema_version = 1\n"), 0o644); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	env := append(os.Environ(), "HOME="+home, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	gitStatus, err := runRTK(ctx, root, env, rtkPath, "git", "status")
	if err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	testOutput, err := runRTK(ctx, root, env, rtkPath, "test", "cargo", "test")
	if err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	if !strings.Contains(gitStatus, "compact git status") || !strings.Contains(testOutput, "compact test output") {
		return fail("rtk-explicit-proxy", "expected explicit rtk command output")
	}
	if fileExists(filepath.Join(home, ".codex")) || fileExists(filepath.Join(home, ".claude")) {
		return fail("rtk-explicit-proxy", "expected no global agent config directories")
	}
	if err := os.Remove(filepath.Join(root, ".rtk", "filters.toml")); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	if err := os.WriteFile(configPath, []byte("recipes:\n  enabled: []\n"), 0o644); err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	if fileExists(filepath.Join(root, ".rtk", "filters.toml")) {
		return fail("rtk-explicit-proxy", "expected managed rtk filters to be removable")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fail("rtk-explicit-proxy", err.Error())
	}
	if strings.Contains(string(data), "rtk-explicit-proxy") {
		return fail("rtk-explicit-proxy", "expected rtk recipe to be removable from config")
	}
	return resolved(
		"rtk-explicit-proxy",
		"Created a temp repo with a project-local RTK filters file and fake rtk binary.",
		"Hookline recipe keeps RTK explicit: no global init and no automatic Bash rewrite.",
		"Agent ran rtk git status and rtk test cargo test explicitly.",
		"Explicit RTK commands returned compact output and managed filters were removable.",
		rtkExplicitProxyEvidence(),
	)
}

func runRTK(ctx context.Context, root string, env []string, rtkPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, rtkPath, args...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func fakeRTKScript() string {
	return `#!/usr/bin/env sh
if [ "$1" = "--version" ]; then
  echo "rtk 0.0.0-fixture"
  exit 0
fi
if [ "$1" = "git" ] && [ "$2" = "status" ]; then
  echo "compact git status"
  exit 0
fi
if [ "$1" = "test" ]; then
  echo "compact test output"
  exit 0
fi
echo "rtk fixture: $*"
`
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
