package infra_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// gitInit makes dir a repository, cloned from origin unless it is empty.
func gitInit(t *testing.T, dir, origin string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q", dir}, {"-C", dir, "remote", "add", "origin", origin}} {
		if origin == "" && args[0] == "-C" {
			continue
		}
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
}

func TestGitWorkspaceDetect(t *testing.T) {
	bareRepository(t) // isolates git from the user's config
	workspace := infra.NewGitWorkspace(io.Discard)

	detect := func(t *testing.T, root string) []string {
		t.Helper()
		projectRoot, err := domain.NewProjectRoot(root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		checkouts, err := workspace.Detect(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var found []string
		for _, c := range checkouts.List() {
			found = append(found, c.Dir()+" "+c.Remote())
		}
		return found
	}

	t.Run("finds a repository at the root, not its subdirectories", func(t *testing.T) {
		root := t.TempDir()
		gitInit(t, root, "git@host:app.git")
		mkdir(t, filepath.Join(root, "api"))

		if got, want := detect(t, root), []string{". git@host:app.git"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("finds repositories in the root's directories", func(t *testing.T) {
		root := t.TempDir()
		gitInit(t, filepath.Join(root, "back"), "git@host:back.git")
		gitInit(t, filepath.Join(root, "front"), "git@host:front.git")
		mkdir(t, filepath.Join(root, "notes"))

		want := []string{"back git@host:back.git", "front git@host:front.git"}
		if got := detect(t, root); !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("skips repositories without an origin to clone from", func(t *testing.T) {
		root := t.TempDir()
		gitInit(t, filepath.Join(root, "scratch"), "")

		if got := detect(t, root); len(got) != 0 {
			t.Fatalf("got %q, want none", got)
		}
	})

	t.Run("finds nothing under a root that doesn't exist yet", func(t *testing.T) {
		if got := detect(t, filepath.Join(t.TempDir(), "missing")); len(got) != 0 {
			t.Fatalf("got %q, want none", got)
		}
	})
}
