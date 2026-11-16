package infra_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// registeredShop is a registry listing the root shop, holding config as
// weaver.yml, and the root.
func registeredShop(t *testing.T, config string) (infra.FileProjectRegistry, domain.ProjectRoot) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "shop")
	write(t, filepath.Join(root, "weaver.yml"), config)
	settings := filepath.Join(dir, "settings.yml")
	write(t, settings, "projects:\n  - "+root+"\n")
	return infra.NewFileProjectRegistry(settings), mustRoot(t, root)
}

// write writes content to path, making its directory.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("making %s's directory: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// read is what path holds.
func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(content)
}

func mustRoot(t *testing.T, raw string) domain.ProjectRoot {
	t.Helper()
	root, err := domain.NewProjectRoot(raw)
	if err != nil {
		t.Fatalf("NewProjectRoot(%q): %v", raw, err)
	}
	return root
}

func TestFileProjectRegistryDocuments(t *testing.T) {
	const stored = "env: {A: 1}\n"

	t.Run("reads a config as stored, with its file", func(t *testing.T) {
		configs, root := registeredShop(t, stored)
		got, err := configs.Document(root)
		if want := filepath.Join(root.String(), "weaver.yml"); err != nil || got.File != want || string(got.Content) != stored {
			t.Fatalf("got %q %q (%v), want %s %q", got.File, got.Content, err, want, stored)
		}
	})

	t.Run("reports a root that isn't registered", func(t *testing.T) {
		configs, _ := registeredShop(t, stored)
		if _, err := configs.Document(mustRoot(t, "/srv/blog")); !errors.Is(err, domain.ErrProjectNotFound) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})

	t.Run("reports a registered root without a config", func(t *testing.T) {
		configs, root := registeredShop(t, stored)
		if err := os.Remove(filepath.Join(root.String(), "weaver.yml")); err != nil {
			t.Fatal(err)
		}
		if _, err := configs.Document(root); !errors.Is(err, domain.ErrMissingConfig) {
			t.Fatalf("got %v, want ErrMissingConfig", err)
		}
	})

	for config, want := range map[string]bool{
		"env: {A: 1}\n": false,
		"# yaml-language-server: $schema=https://example.com/weaver.schema.json\nenv: {A: 1}\n": false,
		"# shop's config\nenv: {A: 1}\n":                      true,
		"env: {A: 1}  # moved\n":                              true,
		"windows:\n  - name: api\n    # later: a db window\n": true,
	} {
		t.Run(fmt.Sprintf("tells whether %q is commented", config), func(t *testing.T) {
			configs, root := registeredShop(t, config)
			if got, err := configs.Commented(root); err != nil || got != want {
				t.Fatalf("got %v (%v), want %v", got, err, want)
			}
		})
	}

	t.Run("stores a valid config as written", func(t *testing.T) {
		configs, root := registeredShop(t, stored)
		edited := "# yaml-language-server: $schema=https://example.com/weaver.schema.json\nenv: {A: 2}  # kept\n"
		if err := configs.ReplaceDocument(root, []byte(edited)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(root.String(), "weaver.yml")); got != edited {
			t.Fatalf("got %q, want the edit as written", got)
		}
	})

	t.Run("stores through a link to the config, keeping the link", func(t *testing.T) {
		configs, root := registeredShop(t, stored)
		link := filepath.Join(root.String(), "weaver.yml")
		target := filepath.Join(t.TempDir(), "dotfiles-shop.yml")
		if err := os.Rename(link, target); err != nil {
			t.Fatalf("moving fixture: %v", err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("linking fixture: %v", err)
		}
		edited := "env: {A: 2}\n"

		if err := configs.ReplaceDocument(root, []byte(edited)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, target); got != edited {
			t.Fatalf("got the link's target holding %q, want the edit", got)
		}
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("got %v (%v), want weaver.yml still a link", info, err)
		}
	})

	t.Run("keeps the stored config when the new one is invalid", func(t *testing.T) {
		configs, root := registeredShop(t, stored)
		err := configs.ReplaceDocument(root, []byte("root: /srv/shop\n"))
		if !errors.Is(err, domain.ErrInvalidConfig) || !errors.Is(err, infra.ErrRootInConfig) {
			t.Fatalf("got %v, want ErrInvalidConfig for the root", err)
		}
		if got := read(t, filepath.Join(root.String(), "weaver.yml")); got != stored {
			t.Fatalf("got %q, want the stored config kept", got)
		}
	})

	t.Run("marks an unreadable stored config as invalid", func(t *testing.T) {
		configs, root := registeredShop(t, "env: [1]\n")
		if _, err := configs.Find(root); !errors.Is(err, domain.ErrInvalidConfig) {
			t.Fatalf("got %v, want ErrInvalidConfig", err)
		}
	})
}

func TestFileProjectRegistryConfigAt(t *testing.T) {
	t.Run("reads the config in a root that isn't registered", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, ".weaver.yaml"), "env: {A: 1}\n")
		configs := infra.NewFileProjectRegistry(filepath.Join(t.TempDir(), "settings.yml"))
		project, document, err := configs.ConfigAt(mustRoot(t, root), "")
		if err != nil || len(project.Env().Vars()) != 1 || document.File != filepath.Join(root, ".weaver.yaml") {
			t.Fatalf("got %v from %s (%v), want .weaver.yaml's project", project.Env().Vars(), document.File, err)
		}
	})

	t.Run("reads the config named, relative to the root", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "weaver.yml"), "{}\n")
		write(t, filepath.Join(root, "tmux", "dev.yml"), "env: {A: 1}\n")
		configs := infra.NewFileProjectRegistry(filepath.Join(t.TempDir(), "settings.yml"))
		project, _, err := configs.ConfigAt(mustRoot(t, root), "tmux/dev.yml")
		if err != nil || len(project.Env().Vars()) != 1 {
			t.Fatalf("got %v (%v), want tmux/dev.yml's project", project.Env().Vars(), err)
		}
	})

	t.Run("reports a root without a config", func(t *testing.T) {
		configs := infra.NewFileProjectRegistry(filepath.Join(t.TempDir(), "settings.yml"))
		if _, _, err := configs.ConfigAt(mustRoot(t, t.TempDir()), ""); !errors.Is(err, domain.ErrMissingConfig) {
			t.Fatalf("got %v, want ErrMissingConfig", err)
		}
	})
}
