package app_test

import (
	"errors"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// inMemoryRepository is a fake domain.ProjectRepository, so these tests
// never touch disk.
type inMemoryRepository struct {
	projects map[string]domain.Project
}

func (r inMemoryRepository) FindByName(name domain.ProjectName) (domain.Project, error) {
	project, ok := r.projects[name.String()]
	if !ok {
		return domain.Project{}, domain.NewProjectNotFound(name)
	}
	return project, nil
}

func TestLoadProjectHandle(t *testing.T) {
	name, err := domain.NewProjectName("weaver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root, err := domain.NewProjectRoot("/home/ilies/dev/weaver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	seeded := domain.NewProject(name, root)

	repo := inMemoryRepository{projects: map[string]domain.Project{"weaver": seeded}}
	useCase := app.NewLoadProject(repo)

	t.Run("resolves a known project", func(t *testing.T) {
		got, err := useCase.Handle("weaver")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Describe() != seeded.Describe() {
			t.Fatalf("got %q, want %q", got.Describe(), seeded.Describe())
		}
	})

	t.Run("passes through a not-found error", func(t *testing.T) {
		_, err := useCase.Handle("missing")
		var notFound domain.ProjectNotFound
		if !errors.As(err, &notFound) {
			t.Fatalf("got %v, want ProjectNotFound", err)
		}
	})

	t.Run("rejects a blank name before touching the repository", func(t *testing.T) {
		_, err := useCase.Handle("   ")
		if !errors.Is(err, domain.ErrEmptyProjectName) {
			t.Fatalf("got %v, want ErrEmptyProjectName", err)
		}
	})
}
