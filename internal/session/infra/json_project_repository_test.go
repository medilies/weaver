package infra_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

func TestJSONProjectRepositoryFindByName(t *testing.T) {
	dir := t.TempDir()
	repo := infra.NewJSONProjectRepository(dir)

	t.Run("loads a project from its JSON file", func(t *testing.T) {
		if err := os.WriteFile(
			filepath.Join(dir, "weaver.json"),
			[]byte(`{"root": "/home/ilies/dev/weaver"}`),
			0o600,
		); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}

		name, err := domain.NewProjectName("weaver")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		project, err := repo.FindByName(name)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "weaver (/home/ilies/dev/weaver)"
		if got := project.Describe(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("reports a not-found error when the file is missing", func(t *testing.T) {
		name, err := domain.NewProjectName("missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = repo.FindByName(name)
		var notFound domain.ProjectNotFound
		if !errors.As(err, &notFound) {
			t.Fatalf("got %v, want ProjectNotFound", err)
		}
	})
}
