package infra_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// requireTool skips the test when name isn't installed, except in CI,
// where every test must run.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s is required in CI", name)
		}
		t.Skipf("%s is not installed", name)
	}
}

// bareRepository creates an empty repository to clone from, without the
// user's git config.
func bareRepository(t *testing.T) string {
	t.Helper()
	requireTool(t, "git")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return remote
}

func TestGitWorkspaceExists(t *testing.T) {
	workspace := infra.NewGitWorkspace(io.Discard)
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	for path, want := range map[string]bool{dir: true, filepath.Join(dir, "missing"): false, file: false} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			got, err := workspace.Exists(path)
			if err != nil || got != want {
				t.Fatalf("got %v (%v), want %v", got, err, want)
			}
		})
	}
}

func TestGitWorkspaceClone(t *testing.T) {
	remote := bareRepository(t)
	workspace := infra.NewGitWorkspace(io.Discard)

	t.Run("clones into a directory whose parents are missing", func(t *testing.T) {
		checkout, err := domain.NewCheckout("back", remote)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dir := filepath.Join(t.TempDir(), "dev", "shop", "back")

		if err := workspace.Clone(checkout, dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Fatalf("got no repository in %s: %v", dir, err)
		}
	})

	t.Run("names the remote it failed to clone", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.git")
		checkout, err := domain.NewCheckout("back", missing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = workspace.Clone(checkout, filepath.Join(t.TempDir(), "back"))
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("got %v, want an error naming %s", err, missing)
		}
	})
}
