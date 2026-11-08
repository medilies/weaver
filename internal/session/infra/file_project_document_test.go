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

func TestFileProjectRegistryDocuments(t *testing.T) {
	const stored = "root: /srv/shop\n"
	setup := func(t *testing.T) (infra.FileProjectRegistry, string, domain.ProjectName) {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "shop.yml"), []byte(stored), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		name, _ := domain.NewProjectName("shop")
		return infra.NewFileProjectRegistry(dir), dir, name
	}
	read := func(t *testing.T, path string) string {
		t.Helper()
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		return string(content)
	}

	t.Run("reads a config as stored, with its file name", func(t *testing.T) {
		configs, _, name := setup(t)
		got, err := configs.Document(name)
		if err != nil || got.File != "shop.yml" || string(got.Content) != stored {
			t.Fatalf("got %q %q (%v), want shop.yml %q", got.File, got.Content, err, stored)
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		configs, _, _ := setup(t)
		name, _ := domain.NewProjectName("blog")
		if _, err := configs.Document(name); !isNotFound(err) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})

	for config, want := range map[string]bool{
		"root: /srv/shop\n": false,
		"# yaml-language-server: $schema=./weaver.schema.json\nroot: /srv/shop\n": false,
		"# shop's config\nroot: /srv/shop\n":                                      true,
		"root: /srv/shop  # moved\n":                                              true,
		"root: /srv/shop\nwindows:\n  - name: api\n    # later: a db window\n":    true,
	} {
		t.Run(fmt.Sprintf("tells whether %q is commented", config), func(t *testing.T) {
			configs, dir, name := setup(t)
			if err := os.WriteFile(filepath.Join(dir, "shop.yml"), []byte(config), 0o600); err != nil {
				t.Fatalf("writing fixture: %v", err)
			}
			if got, err := configs.Commented(name); err != nil || got != want {
				t.Fatalf("got %v (%v), want %v", got, err, want)
			}
		})
	}

	t.Run("stores a valid config as written", func(t *testing.T) {
		configs, dir, name := setup(t)
		edited := "# yaml-language-server: $schema=./weaver.schema.json\nroot: /srv/shop\nenv: {A: 1}  # kept\n"
		if err := configs.ReplaceDocument(name, []byte(edited)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(dir, "shop.yml")); got != edited {
			t.Fatalf("got %q, want the edit as written", got)
		}
	})

	t.Run("stores through a link to the config, keeping the link", func(t *testing.T) {
		configs, dir, name := setup(t)
		target := filepath.Join(t.TempDir(), "dotfiles-shop.yml")
		if err := os.Rename(filepath.Join(dir, "shop.yml"), target); err != nil {
			t.Fatalf("moving fixture: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "shop.yml")); err != nil {
			t.Fatalf("linking fixture: %v", err)
		}
		edited := "root: /srv/shop\nenv: {A: 1}\n"

		if err := configs.ReplaceDocument(name, []byte(edited)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, target); got != edited {
			t.Fatalf("got the link's target holding %q, want the edit", got)
		}
		if info, err := os.Lstat(filepath.Join(dir, "shop.yml")); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("got %v (%v), want shop.yml still a link", info, err)
		}
	})

	t.Run("keeps the stored config when the new one is invalid", func(t *testing.T) {
		configs, dir, name := setup(t)
		err := configs.ReplaceDocument(name, []byte("root: relative/shop\n"))
		if !errors.Is(err, domain.ErrInvalidConfig) {
			t.Fatalf("got %v, want ErrInvalidConfig", err)
		}
		if got := read(t, filepath.Join(dir, "shop.yml")); got != stored {
			t.Fatalf("got %q, want the stored config kept", got)
		}
	})

	t.Run("saves the JSON Schema next to a stored config", func(t *testing.T) {
		configs, dir, name := setup(t)
		if err := configs.ReplaceDocument(name, []byte(stored)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "weaver.schema.json")); err != nil {
			t.Fatalf("got no schema: %v", err)
		}
	})

	t.Run("marks an unreadable stored config as invalid", func(t *testing.T) {
		configs, dir, name := setup(t)
		if err := os.WriteFile(filepath.Join(dir, "shop.yml"), []byte("root: [1]\n"), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		if _, err := configs.FindByName(name); !errors.Is(err, domain.ErrInvalidConfig) {
			t.Fatalf("got %v, want ErrInvalidConfig", err)
		}
	})
}

func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrProjectNotFound)
}
