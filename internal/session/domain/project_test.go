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

	for _, valid := range []string{"nimbus", "my-app_2", "2fa"} {
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

	t.Run("rejects control characters", func(t *testing.T) {
		_, err := domain.NewProjectRoot("/home/ilies/dev\nweaver")
		if !errors.Is(err, domain.ErrControlCharacters) {
			t.Fatalf("got %v, want ErrControlCharacters", err)
		}
	})
}

func TestProjectWindows(t *testing.T) {
	project := domain.NewProject(projectName(t, "globex"), projectRoot(t, "/home/ilies/dev/globex"))

	t.Run("opens one window named after the project by default", func(t *testing.T) {
		windows := project.Windows()
		if len(windows) != 1 || windows[0].Name().String() != "globex" {
			t.Fatalf("got %d windows, want one named globex", len(windows))
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

func TestChangingProjectWindows(t *testing.T) {
	shopProject := func(t *testing.T) domain.Project {
		t.Helper()
		p, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
			WithWindows(window(t, "api"), window(t, "app"), window(t, "db"))
		if err != nil {
			t.Fatalf("WithWindows: %v", err)
		}
		return p
	}
	names := func(p domain.Project) []string {
		var out []string
		for _, w := range p.Windows() {
			out = append(out, w.Name().String())
		}
		return out
	}

	t.Run("adds a window last", func(t *testing.T) {
		added, err := shopProject(t).AddingWindow(window(t, "repo"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := names(added), []string{"api", "app", "db", "repo"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("rejects adding a window named as another", func(t *testing.T) {
		if _, err := shopProject(t).AddingWindow(window(t, "db")); !errors.Is(err, domain.ErrDuplicateWindowName) {
			t.Fatalf("got %v, want ErrDuplicateWindowName", err)
		}
	})

	t.Run("replaces a window", func(t *testing.T) {
		replaced, err := shopProject(t).ReplacingWindow(1, window(t, "web"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := names(replaced), []string{"api", "web", "db"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("removes a window", func(t *testing.T) {
		removed, err := shopProject(t).WithoutWindow(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := names(removed), []string{"app", "db"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("rejects removing a project's only window", func(t *testing.T) {
		project := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop"))
		if _, err := project.WithoutWindow(0); !errors.Is(err, domain.ErrLastWindow) {
			t.Fatalf("got %v, want ErrLastWindow", err)
		}
	})

	t.Run("moves a window, the others shifting", func(t *testing.T) {
		moved, err := shopProject(t).MovingWindow(2, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := names(moved), []string{"db", "api", "app"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps the project's env and before commands", func(t *testing.T) {
		env, err := domain.NewEnv(map[string]string{"APP_ENV": "dev"})
		if err != nil {
			t.Fatalf("NewEnv: %v", err)
		}
		project := shopProject(t).WithEnv(env).RunningFirst(command(t, "nvm use"))
		removed, err := project.WithoutWindow(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(removed.Env().Vars()) != 1 || len(removed.Before()) != 1 {
			t.Fatalf("got %d variables and %d before commands, want 1 and 1", len(removed.Env().Vars()), len(removed.Before()))
		}
	})

	for name, change := range map[string]func(domain.Project) (domain.Project, error){
		"replacing": func(p domain.Project) (domain.Project, error) { return p.ReplacingWindow(3, window(t, "web")) },
		"removing":  func(p domain.Project) (domain.Project, error) { return p.WithoutWindow(-1) },
		"moving to": func(p domain.Project) (domain.Project, error) { return p.MovingWindow(0, 3) },
	} {
		t.Run("rejects "+name+" a window the project doesn't have", func(t *testing.T) {
			if _, err := change(shopProject(t)); !errors.Is(err, domain.ErrWindowNotFound) {
				t.Fatalf("got %v, want ErrWindowNotFound", err)
			}
		})
	}
}
