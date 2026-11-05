package domain_test

import (
	"errors"
	"slices"
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

	t.Run("rejects a weave command's name", func(t *testing.T) {
		_, err := domain.NewProjectName("show")
		if !errors.Is(err, domain.ErrReservedProjectName) {
			t.Fatalf("got %v, want ErrReservedProjectName", err)
		}
	})
}

func TestNewProjectRoot(t *testing.T) {
	t.Run("cleans the path", func(t *testing.T) {
		root, err := domain.NewProjectRoot(" /home/ilies/dev/../dev/weaver/ ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := root.String(); got != "/home/ilies/dev/weaver" {
			t.Fatalf("got %q, want %q", got, "/home/ilies/dev/weaver")
		}
	})

	t.Run("rejects blank paths", func(t *testing.T) {
		_, err := domain.NewProjectRoot("  ")
		if !errors.Is(err, domain.ErrEmptyProjectRoot) {
			t.Fatalf("got %v, want ErrEmptyProjectRoot", err)
		}
	})

	t.Run("rejects relative paths", func(t *testing.T) {
		_, err := domain.NewProjectRoot("dev/weaver")
		if !errors.Is(err, domain.ErrRelativeProjectRoot) {
			t.Fatalf("got %v, want ErrRelativeProjectRoot", err)
		}
	})
}

func TestProjectWindows(t *testing.T) {
	project := domain.NewProject(projectName(t, "didata"), projectRoot(t, "/home/ilies/dev/didata"))

	t.Run("opens one window named after the project by default", func(t *testing.T) {
		windows := project.Windows()
		if len(windows) != 1 || windows[0].Name().String() != "didata" {
			t.Fatalf("got %d windows, want one named didata", len(windows))
		}
	})

	t.Run("opens the windows it is given, in order", func(t *testing.T) {
		withWindows, err := project.WithWindows(window(t, "api"), window(t, "app"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var names []string
		for _, w := range withWindows.Windows() {
			names = append(names, w.Name().String())
		}
		if want := []string{"api", "app"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})

	t.Run("rejects two windows with the same name", func(t *testing.T) {
		_, err := project.WithWindows(window(t, "api"), window(t, "api"))
		if !errors.Is(err, domain.ErrDuplicateWindowName) {
			t.Fatalf("got %v, want ErrDuplicateWindowName", err)
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
