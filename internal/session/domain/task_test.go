package domain_test

import (
	"errors"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func task(t *testing.T, name, raw string) domain.Task {
	t.Helper()
	task, err := domain.NewTask(name, command(t, raw))
	if err != nil {
		t.Fatalf("NewTask(%q, %q): %v", name, raw, err)
	}
	return task
}

func TestNewTask(t *testing.T) {
	t.Run("shows by its name", func(t *testing.T) {
		if got := task(t, " migrate ", "php artisan migrate").Label(); got != "migrate" {
			t.Fatalf("got %q, want %q", got, "migrate")
		}
	})

	t.Run("shows by its command when unnamed", func(t *testing.T) {
		if got := task(t, "", "php artisan db:seed").Label(); got != "php artisan db:seed" {
			t.Fatalf("got %q, want the command", got)
		}
	})

	t.Run("rejects a name with control characters", func(t *testing.T) {
		if _, err := domain.NewTask("mi\tgrate", command(t, "make")); !errors.Is(err, domain.ErrControlCharacters) {
			t.Fatalf("got %v, want ErrControlCharacters", err)
		}
	})

	t.Run("rejects a task without a command", func(t *testing.T) {
		if _, err := domain.NewTask("migrate", domain.Command{}); !errors.Is(err, domain.ErrEmptyCommand) {
			t.Fatalf("got %v, want ErrEmptyCommand", err)
		}
	})
}

func TestTaskThrowawayWindow(t *testing.T) {
	cases := []struct{ name, task, command, want string }{
		{"is named after the task", "migrate", "php artisan migrate", "migrate"},
		{"is named after the program of an unnamed task", "", "NODE_ENV=dev npm run build", "npm"},
		{"is named after the command when the shell decides what runs", "", "cd back && make", "cd back && make"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := task(t, c.task, c.command).ThrowawayWindow().String(); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestPaneTasks(t *testing.T) {
	t.Run("rejects two tasks of a pane with the same name", func(t *testing.T) {
		pane := domain.NewPane().Offering(task(t, "seed", "make seed"), task(t, "seed", "make reseed"))
		if _, err := domain.NewWindow(windowName(t, "api"), pane); !errors.Is(err, domain.ErrDuplicateTask) {
			t.Fatalf("got %v, want ErrDuplicateTask", err)
		}
	})

	t.Run("rejects two unnamed tasks of a pane with the same command", func(t *testing.T) {
		pane := domain.NewPane().Offering(task(t, "", "make seed"), task(t, "", "make seed"))
		if _, err := domain.NewWindow(windowName(t, "api"), pane); !errors.Is(err, domain.ErrDuplicateTask) {
			t.Fatalf("got %v, want ErrDuplicateTask", err)
		}
	})

	t.Run("tells a named task from an unnamed one with its command", func(t *testing.T) {
		pane := domain.NewPane().Offering(task(t, "seed", "make seed"), task(t, "", "make seed"))
		if _, err := domain.NewWindow(windowName(t, "api"), pane); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("lets panes of a window offer the same task", func(t *testing.T) {
		seed := task(t, "seed", "make seed")
		_, err := domain.NewWindow(windowName(t, "api"),
			domain.NewPane().Named(paneName(t, "back")).Offering(seed),
			domain.NewPane().Named(paneName(t, "front")).Splitting(split(t, 0, domain.Right, "")).Offering(seed))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("are planned in order with their pane", func(t *testing.T) {
		pane := domain.NewPane().Offering(task(t, "migrate", "make migrate"), task(t, "", "make seed"))
		project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
			WithWindows(window(t, "api", pane))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tasks := project.Plan().Windows()[0].Panes()[0].Tasks()
		if len(tasks) != 2 || tasks[0].Label() != "migrate" || tasks[1].Label() != "make seed" {
			t.Fatalf("got %v, want migrate then make seed", tasks)
		}
	})
}
