package infra_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// touch creates an empty executable at path, with its parent directories.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
}

func runtimeOf(t *testing.T, kind, version string) domain.Runtime {
	t.Helper()
	runtime, err := domain.NewRuntime(kind, version)
	if err != nil {
		t.Fatalf("NewRuntime(%q, %q): %v", kind, version, err)
	}
	return runtime
}

func TestInstalledRuntimesResolvesPHP(t *testing.T) {
	bin, shims := t.TempDir(), t.TempDir()
	for _, name := range []string{"php", "php8.2", "php8.4", "php-config8.5", "phpize8.5", "phpabtpl"} {
		touch(t, filepath.Join(bin, name))
	}
	runtimes := infra.NewInstalledRuntimes(bin, t.TempDir(), shims)

	t.Run("links php to the pinned version in a directory for PATH", func(t *testing.T) {
		dir, err := runtimes.Resolve(runtimeOf(t, "php", "8.4"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		target, err := os.Readlink(filepath.Join(dir, "php"))
		if err != nil || target != filepath.Join(bin, "php8.4") {
			t.Fatalf("got %q (%v), want a link to php8.4", target, err)
		}
	})

	t.Run("picks the newest version matching a major", func(t *testing.T) {
		dir, err := runtimes.Resolve(runtimeOf(t, "php", "8"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if filepath.Base(dir) != "8.4" {
			t.Fatalf("got %q, want the 8.4 directory", dir)
		}
	})

	t.Run("ignores binaries that aren't a versioned php", func(t *testing.T) {
		_, err := runtimes.Resolve(runtimeOf(t, "php", "8.5"))
		if !errors.Is(err, domain.ErrRuntimeNotInstalled) {
			t.Fatalf("got %v, want ErrRuntimeNotInstalled", err)
		}
	})

	t.Run("lists the installed versions when the pinned one is missing", func(t *testing.T) {
		_, err := runtimes.Resolve(runtimeOf(t, "php", "8.1"))
		if err == nil || !strings.Contains(err.Error(), "php 8.1 (installed: 8.2, 8.4)") {
			t.Fatalf("got %v, want the installed versions listed", err)
		}
	})
}

func TestInstalledRuntimesResolvesNode(t *testing.T) {
	nvm := t.TempDir()
	for _, version := range []string{"v2.0.0", "v20.1.0", "v20.20.2", "v22.18.0"} {
		touch(t, filepath.Join(nvm, "versions", "node", version, "bin", "node"))
	}
	runtimes := infra.NewInstalledRuntimes(t.TempDir(), nvm, t.TempDir())

	t.Run("picks the newest install of the pinned major", func(t *testing.T) {
		dir, err := runtimes.Resolve(runtimeOf(t, "node", "20"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join(nvm, "versions", "node", "v20.20.2", "bin"); dir != want {
			t.Fatalf("got %q, want %q", dir, want)
		}
	})

	t.Run("matches versions at a dot boundary", func(t *testing.T) {
		dir, err := runtimes.Resolve(runtimeOf(t, "node", "2"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join(nvm, "versions", "node", "v2.0.0", "bin"); dir != want {
			t.Fatalf("got %q, want %q", dir, want)
		}
	})

	t.Run("reports a version nvm hasn't installed", func(t *testing.T) {
		_, err := runtimes.Resolve(runtimeOf(t, "node", "18"))
		if !errors.Is(err, domain.ErrRuntimeNotInstalled) {
			t.Fatalf("got %v, want ErrRuntimeNotInstalled", err)
		}
	})
}

func TestInstalledRuntimesAvailable(t *testing.T) {
	bin, nvm := t.TempDir(), t.TempDir()
	for _, name := range []string{"php8.4", "php8.2", "php-config8.5"} {
		touch(t, filepath.Join(bin, name))
	}
	for _, version := range []string{"v20.1.0", "v20.20.2", "v22.18.0"} {
		touch(t, filepath.Join(nvm, "versions", "node", version, "bin", "node"))
	}

	available, err := infra.NewInstalledRuntimes(bin, nvm, t.TempDir()).Available()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []string
	for _, runtime := range available {
		got = append(got, runtime.String())
	}
	if want := []string{"node 20", "node 22", "php 8.2", "php 8.4"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
