package infra_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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

func TestLocalWorkspaceExists(t *testing.T) {
	workspace := infra.NewLocalWorkspace()
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

func TestLocalWorkspaceRealPath(t *testing.T) {
	workspace := infra.NewLocalWorkspace()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving the test directory: %v", err)
	}
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatalf("making fixture: %v", err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "link")); err != nil {
		t.Fatalf("linking fixture: %v", err)
	}

	t.Run("resolves a path through a symlink", func(t *testing.T) {
		if got := workspace.RealPath(filepath.Join(dir, "link")); got != real {
			t.Fatalf("got %q, want %q", got, real)
		}
	})

	t.Run("keeps a path that doesn't exist as is", func(t *testing.T) {
		missing := filepath.Join(dir, "link", "missing")
		if got := workspace.RealPath(missing); got != missing {
			t.Fatalf("got %q, want %q", got, missing)
		}
	})
}

func TestLocalWorkspaceExecutable(t *testing.T) {
	workspace := infra.NewLocalWorkspace()

	t.Run("finds a program on PATH", func(t *testing.T) {
		requireTool(t, "sh")
		if !workspace.Executable("sh", t.TempDir()) {
			t.Fatal("got sh not found, want it on PATH")
		}
	})

	t.Run("doesn't find a program missing from PATH", func(t *testing.T) {
		if workspace.Executable("weaver-no-such-program", t.TempDir()) {
			t.Fatal("got a missing program found")
		}
	})

	t.Run("finds a script by its path from the directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", "dev"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !workspace.Executable("./bin/dev", dir) || workspace.Executable("./bin/dev", t.TempDir()) {
			t.Fatal("want ./bin/dev found from its directory only")
		}
	})

	t.Run("doesn't run a file that isn't executable", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "notes"), []byte("todo\n"), 0o644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if workspace.Executable("./notes", dir) {
			t.Fatal("got a plain file executable")
		}
	})
}
