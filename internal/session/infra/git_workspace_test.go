package infra_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull) // also shuts out $XDG_CONFIG_HOME/git/config
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

func TestGitWorkspaceExecutable(t *testing.T) {
	workspace := infra.NewGitWorkspace(io.Discard)

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

func TestGitWorkspaceClone(t *testing.T) {
	remote := bareRepository(t)
	workspace := infra.NewGitWorkspace(io.Discard)

	t.Run("clones into a directory whose parents are missing", func(t *testing.T) {
		repo, err := domain.NewRepo("back", remote)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dir := filepath.Join(t.TempDir(), "dev", "shop", "back")

		if err := workspace.Clone(repo, dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Fatalf("got no repository in %s: %v", dir, err)
		}
	})

	t.Run("names the remote it failed to clone", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.git")
		repo, err := domain.NewRepo("back", missing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = workspace.Clone(repo, filepath.Join(t.TempDir(), "back"))
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
		repos, err := workspace.Detect(projectRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var found []string
		for _, r := range repos.List() {
			found = append(found, r.Dir()+" "+r.Remote())
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

	t.Run("drops the credentials of an https remote", func(t *testing.T) {
		root := t.TempDir()
		gitInit(t, filepath.Join(root, "back"), "https://ghp_token@github.com/me/back.git")
		gitInit(t, filepath.Join(root, "front"), "https://me:secret@github.com/me/front.git")

		want := []string{"back https://github.com/me/back.git", "front https://github.com/me/front.git"}
		if got := detect(t, root); !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps the user of an ssh remote", func(t *testing.T) {
		root := t.TempDir()
		gitInit(t, root, "ssh://git@host/app.git")

		if got, want := detect(t, root), []string{". ssh://git@host/app.git"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("skips a submodule, which its superproject brings", func(t *testing.T) {
		t.Setenv("GIT_AUTHOR_NAME", "test")
		t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
		t.Setenv("GIT_COMMITTER_NAME", "test")
		t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
		lib := t.TempDir()
		root := t.TempDir()
		gitInit(t, lib, "")
		gitInit(t, root, "git@host:app.git")
		for _, args := range [][]string{
			{"-C", lib, "commit", "-q", "--allow-empty", "-m", "lib"},
			{"-C", root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "lib"},
		} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
			}
		}

		if got, want := detect(t, root), []string{". git@host:app.git"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("finds a root that is itself a submodule", func(t *testing.T) {
		t.Setenv("GIT_AUTHOR_NAME", "test")
		t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
		t.Setenv("GIT_COMMITTER_NAME", "test")
		t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
		proj := t.TempDir()
		mono := t.TempDir()
		gitInit(t, proj, "")
		gitInit(t, mono, "git@host:mono.git")
		for _, args := range [][]string{
			{"-C", proj, "commit", "-q", "--allow-empty", "-m", "proj"},
			{"-C", mono, "-c", "protocol.file.allow=always", "submodule", "add", "-q", proj, "proj"},
		} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
			}
		}

		// The submodule's origin is the repository it was added from.
		if got, want := detect(t, filepath.Join(mono, "proj")), []string{". " + proj}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestGitWorkspaceWorkingTree(t *testing.T) {
	remote := bareRepository(t)
	workspace := infra.NewGitWorkspace(io.Discard)
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	// clone is a clone of remote on branch feature, pushed with one
	// commit then one commit ahead of it.
	clone := filepath.Join(t.TempDir(), "shop")
	for _, args := range [][]string{
		{"clone", "-q", remote, clone},
		{"-C", clone, "checkout", "-q", "-b", "feature"},
		{"-C", clone, "commit", "-q", "--allow-empty", "-m", "first"},
		{"-C", clone, "push", "-q", "-u", "origin", "feature"},
		{"-C", clone, "commit", "-q", "--allow-empty", "-m", "second"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}

	t.Run("reads the branch and how far it is ahead", func(t *testing.T) {
		tree, ok := workspace.WorkingTree(clone)
		if !ok || tree.Branch() != "feature" || tree.Ahead() != 1 || tree.Behind() != 0 || tree.Dirty() {
			t.Fatalf("got %+v (%v), want clean feature one ahead", tree, ok)
		}
	})

	t.Run("sees an untracked file as a change", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(clone, "notes"), nil, 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(clone, "notes")) })
		if tree, ok := workspace.WorkingTree(clone); !ok || !tree.Dirty() {
			t.Fatalf("got %+v (%v), want changes", tree, ok)
		}
	})

	t.Run("reads how far it is behind", func(t *testing.T) {
		behind := filepath.Join(t.TempDir(), "behind")
		for _, args := range [][]string{
			{"clone", "-q", "--branch", "feature", remote, behind},
			{"-C", behind, "commit", "-q", "--allow-empty", "-m", "pushed"},
			{"-C", behind, "push", "-q"},
			{"-C", behind, "reset", "-q", "--hard", "HEAD~1"},
		} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
			}
		}
		if tree, ok := workspace.WorkingTree(behind); !ok || tree.Behind() != 1 || tree.Ahead() != 0 {
			t.Fatalf("got %+v (%v), want one behind", tree, ok)
		}
	})

	t.Run("reads a detached head", func(t *testing.T) {
		detached := filepath.Join(t.TempDir(), "detached")
		for _, args := range [][]string{{"clone", "-q", clone, detached}, {"-C", detached, "checkout", "-q", "--detach"}} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
			}
		}
		if tree, ok := workspace.WorkingTree(detached); !ok || !tree.Detached() {
			t.Fatalf("got %+v (%v), want a detached head", tree, ok)
		}
	})

	t.Run("finds none outside a repository", func(t *testing.T) {
		if _, ok := workspace.WorkingTree(t.TempDir()); ok {
			t.Fatal("got a working tree, want none")
		}
	})

	t.Run("gives up on time while git's children still run", func(t *testing.T) {
		slow := t.TempDir()
		gitInit(t, slow, "")
		hook := filepath.Join(t.TempDir(), "fsmonitor")
		if err := os.WriteFile(hook, []byte("#!/bin/sh\nsleep 5\n"), 0o700); err != nil {
			t.Fatalf("writing hook: %v", err)
		}
		if out, err := exec.Command("git", "-C", slow, "config", "core.fsmonitor", hook).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v: %s", err, out)
		}

		start := time.Now()
		workspace.WorkingTree(slow)
		if took := time.Since(start); took > 3*time.Second {
			t.Fatalf("took %s, want about a second", took)
		}
	})
}
