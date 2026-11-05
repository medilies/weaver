package domain_test

import (
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestProjectRootResolve(t *testing.T) {
	root := projectRoot(t, "/srv/shop")

	for raw, want := range map[string]string{"": "/srv/shop", "back": "/srv/shop/back", "/opt/tools": "/opt/tools"} {
		t.Run(`resolves "`+raw+`"`, func(t *testing.T) {
			if got := root.Resolve(paneDir(t, raw)); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestProjectRootContains(t *testing.T) {
	root := projectRoot(t, "/srv/shop")

	for path, want := range map[string]bool{
		"/srv/shop":         true,
		"/srv/shop/back/":   true,
		"/srv/shopping":     false,
		"/srv":              false,
		"/srv/shop/../blog": false,
	} {
		t.Run(path, func(t *testing.T) {
			if got := root.Contains(path); got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func TestProjectPlan(t *testing.T) {
	back := domain.NewPane().Named(paneName(t, "back")).In(paneDir(t, "back"))
	front := domain.NewPane().
		Named(paneName(t, "front")).
		In(paneDir(t, "front")).
		Splitting(split(t, 0, domain.Right, "30%")).
		WithEnv(env(t, map[string]string{"APP_ENV": "dev"})).
		RunningFirst(command(t, "nvm use 20")).
		Running(command(t, "npm run dev")).
		WithNote("Usual commands:", "  npm i")
	project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
		WithEnv(env(t, map[string]string{"APP_ENV": "local", "TZ": "UTC"})).
		RunningFirst(command(t, "source .env")).
		WithWindows(window(t, "shop", back, front).RunningFirst(command(t, "git fetch")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	panes := project.Plan().Windows()[0].Panes()
	backPlan, frontPlan := panes[0], panes[1]

	t.Run("makes pane directories absolute", func(t *testing.T) {
		if got := backPlan.Dir(); got != "/srv/shop/back" {
			t.Fatalf("got %q, want %q", got, "/srv/shop/back")
		}
	})

	t.Run("gives panes their project's env", func(t *testing.T) {
		if got, want := render(backPlan.Env()), []string{"APP_ENV=local", "TZ=UTC"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("lets a pane's env override its project's", func(t *testing.T) {
		if got, want := render(frontPlan.Env()), []string{"APP_ENV=dev", "TZ=UTC"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("runs the project's commands, then the window's, first in every pane", func(t *testing.T) {
		if got, want := commands(backPlan.Before()), []string{"source .env", "git fetch"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("runs a pane's own commands after its project's and window's", func(t *testing.T) {
		want := []string{"source .env", "git fetch", "nvm use 20"}
		if got := commands(frontPlan.Before()); !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps a pane's split", func(t *testing.T) {
		s, splits := frontPlan.Split()
		if !splits || s.Of() != 0 || s.Direction() != domain.Right || s.Size().String() != "30%" {
			t.Fatalf("got %v %v %v %q, want a 30%% split right of pane #1", splits, s.Of(), s.Direction(), s.Size().String())
		}
	})

	t.Run("keeps a pane's command", func(t *testing.T) {
		cmd, _ := frontPlan.Startup().Command()
		if cmd.String() != "npm run dev" || !frontPlan.Startup().Submits() {
			t.Fatalf("got %q, want npm run dev submitted", cmd.String())
		}
	})

	t.Run("keeps a pane's note", func(t *testing.T) {
		if got, want := frontPlan.Note(), []string{"Usual commands:", "  npm i"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// commands flattens commands to their text, in order.
func commands(cmds []domain.Command) []string {
	var out []string
	for _, cmd := range cmds {
		out = append(out, cmd.String())
	}
	return out
}
