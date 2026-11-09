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

func TestProjectRootRelative(t *testing.T) {
	root := projectRoot(t, "/srv/shop")

	for raw, want := range map[string]string{
		"/srv/shop/back": "back",
		"/srv/shop":      "",
		"/srv/shopping":  "/srv/shopping",
		"back":           "back",
	} {
		t.Run(`makes "`+raw+`" relative`, func(t *testing.T) {
			if got := root.Relative(paneDir(t, raw)).String(); got != want {
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

func TestProjectPlanDirs(t *testing.T) {
	shell := domain.NewPane().Named(paneName(t, "shell"))
	web := domain.NewPane().Named(paneName(t, "web")).In(paneDir(t, "web")).Splitting(split(t, 0, domain.Right, ""))
	psql := domain.NewPane().Named(paneName(t, "psql")).In(paneDir(t, ".")).Splitting(split(t, 0, domain.Below, ""))
	project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
		WithWindows(window(t, "api", shell, web, psql).In(paneDir(t, "api")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	panes := project.Plan().Windows()[0].Panes()

	for i, want := range map[int]string{
		0: "/srv/shop/api", // its window's
		1: "/srv/shop/web", // its own, relative to the root
		2: "/srv/shop",     // the root, leaving its window's
	} {
		t.Run("starts "+panes[i].Name().String()+" in "+want, func(t *testing.T) {
			if got := panes[i].Dir(); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestProjectRootedAt(t *testing.T) {
	project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
		WithWindows(window(t, "api").In(paneDir(t, "api")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	moved := project.RootedAt(projectRoot(t, "/home/ilies/shop"))

	if got := moved.Plan().Windows()[0].Panes()[0].Dir(); got != "/home/ilies/shop/api" {
		t.Fatalf("got %q, want the window's dir under the new root", got)
	}
}

func TestPaneInWindowDir(t *testing.T) {
	pane := domain.NewPane().In(paneDir(t, "web")).InWindowDir()
	project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
		WithWindows(window(t, "api", pane).In(paneDir(t, "api")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := project.Plan().Windows()[0].Panes()[0].Dir(); got != "/srv/shop/api" {
		t.Fatalf("got %q, want its window's dir", got)
	}
}

// commands flattens commands to their text, in order.
func commands(cmds []domain.Command) []string {
	var out []string
	for _, cmd := range cmds {
		out = append(out, cmd.String())
	}
	return out
}

func TestPanePlanClears(t *testing.T) {
	nvm := command(t, "nvm use 20")
	clears := func(t *testing.T, project domain.Clearing, win domain.Clearing, pane domain.Pane) bool {
		t.Helper()
		p, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
			WithClearing(project).
			WithWindows(window(t, "shop", pane).WithClearing(win))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return p.Plan().Windows()[0].Panes()[0].Clears()
	}
	withBefore := domain.NewPane().RunningFirst(nvm)

	for _, tc := range []struct {
		name         string
		project, win domain.Clearing
		pane         domain.Pane
		want         bool
	}{
		{"not when no level sets it", domain.ClearingUnset, domain.ClearingUnset, withBefore, false},
		{"when the project sets it on", domain.ClearingOn, domain.ClearingUnset, withBefore, true},
		{"not when the window sets it off over the project", domain.ClearingOn, domain.ClearingOff, withBefore, false},
		{"when the pane sets it on over the window", domain.ClearingUnset, domain.ClearingOff, withBefore.WithClearing(domain.ClearingOn), true},
		{"not when the pane sets it off", domain.ClearingOn, domain.ClearingOn, withBefore.WithClearing(domain.ClearingOff), false},
		{"not without before commands to hide", domain.ClearingOn, domain.ClearingUnset, domain.NewPane(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clears(t, tc.project, tc.win, tc.pane); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
