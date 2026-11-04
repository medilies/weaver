package domain_test

import (
	"errors"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestNewProjectName(t *testing.T) {
	t.Run("trims surrounding whitespace", func(t *testing.T) {
		name, err := domain.NewProjectName("  weaver  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := name.String(); got != "weaver" {
			t.Fatalf("got %q, want %q", got, "weaver")
		}
	})

	t.Run("rejects blank names", func(t *testing.T) {
		_, err := domain.NewProjectName("   ")
		if !errors.Is(err, domain.ErrEmptyProjectName) {
			t.Fatalf("got %v, want ErrEmptyProjectName", err)
		}
	})
}

func TestProjectDescribe(t *testing.T) {
	name, err := domain.NewProjectName("weaver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root, err := domain.NewProjectRoot("/home/ilies/dev/weaver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	project := domain.NewProject(name, root)

	want := "weaver (/home/ilies/dev/weaver)"
	if got := project.Describe(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
