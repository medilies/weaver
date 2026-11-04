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

	for _, valid := range []string{"bicyclette", "my-app_2", "2fa"} {
		t.Run("accepts "+valid, func(t *testing.T) {
			if _, err := domain.NewProjectName(valid); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	invalid := map[string]string{
		"a path separator":           "../etc",
		"a dot tmux would rewrite":   "my.app",
		"a colon tmux would rewrite": "my:app",
		"a hash tmux would expand":   "my#app",
		"inner whitespace":           "my app",
		"a leading dash":             "-app",
		"a leading underscore":       "_app",
	}
	for reason, name := range invalid {
		t.Run("rejects "+reason, func(t *testing.T) {
			_, err := domain.NewProjectName(name)
			if !errors.Is(err, domain.ErrInvalidProjectName) {
				t.Fatalf("got %v, want ErrInvalidProjectName", err)
			}
		})
	}
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
