package infra_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

func TestJSONProjectRepositoryDocuments(t *testing.T) {
	const stored = "{\n  \"root\": \"/srv/shop\"\n}\n"
	setup := func(t *testing.T) (infra.JSONProjectRepository, string, domain.ProjectName) {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "shop.json"), []byte(stored), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		name, _ := domain.NewProjectName("shop")
		return infra.NewJSONProjectRepository(dir), dir, name
	}
	read := func(t *testing.T, path string) string {
		t.Helper()
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		return string(content)
	}

	t.Run("reads a config as stored", func(t *testing.T) {
		repo, _, name := setup(t)
		if got, err := repo.Document(name); err != nil || string(got) != stored {
			t.Fatalf("got %q (%v), want %q", got, err, stored)
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		repo, _, _ := setup(t)
		name, _ := domain.NewProjectName("blog")
		if _, err := repo.Document(name); !isNotFound(err) {
			t.Fatalf("got %v, want ProjectNotFound", err)
		}
	})

	t.Run("stores a valid config as written", func(t *testing.T) {
		repo, dir, name := setup(t)
		edited := "{ \"root\": \"/srv/shop\",   \"env\": {\"A\": \"1\"} }\n"
		if err := repo.ReplaceDocument(name, []byte(edited)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(dir, "shop.json")); got != edited {
			t.Fatalf("got %q, want the edit as written", got)
		}
	})

	t.Run("keeps the stored config when the new one is invalid", func(t *testing.T) {
		repo, dir, name := setup(t)
		err := repo.ReplaceDocument(name, []byte(`{"root": "relative/shop"}`))
		if !errors.Is(err, domain.ErrInvalidConfig) {
			t.Fatalf("got %v, want ErrInvalidConfig", err)
		}
		if got := read(t, filepath.Join(dir, "shop.json")); got != stored {
			t.Fatalf("got %q, want the stored config kept", got)
		}
	})

	t.Run("saves the JSON Schema next to a stored config", func(t *testing.T) {
		repo, dir, name := setup(t)
		if err := repo.ReplaceDocument(name, []byte(stored)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "weaver.schema.json")); err != nil {
			t.Fatalf("got no schema: %v", err)
		}
	})

	t.Run("marks an unreadable stored config as invalid", func(t *testing.T) {
		repo, dir, name := setup(t)
		if err := os.WriteFile(filepath.Join(dir, "shop.json"), []byte(`{"root": 1}`), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		if _, err := repo.FindByName(name); !errors.Is(err, domain.ErrInvalidConfig) {
			t.Fatalf("got %v, want ErrInvalidConfig", err)
		}
	})
}

func isNotFound(err error) bool {
	_, ok := errors.AsType[domain.ProjectNotFound](err)
	return ok
}
