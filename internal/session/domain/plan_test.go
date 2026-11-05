package domain_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

// installed is a fake domain.RuntimeResolver: "kind version" → directory.
type installed map[string]string

func (i installed) Resolve(runtime domain.Runtime) (string, error) {
	if dir, ok := i[runtime.String()]; ok {
		return dir, nil
	}
	return "", fmt.Errorf("%w: %s", domain.ErrRuntimeNotInstalled, runtime)
}

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
		WithRuntimes(runtimes(t, "node", "20")).
		Running(command(t, "npm run dev")).
		WithNote("Usual commands:", "  npm i")
	project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
		WithEnv(env(t, map[string]string{"APP_ENV": "local", "TZ": "UTC"})).
		WithRuntimes(runtimes(t, "php", "8.4", "node", "22")).
		WithWindows(window(t, "shop", back, front))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resolver := installed{"php 8.4": "/cache/php/8.4", "node 20": "/nvm/v20/bin", "node 22": "/nvm/v22/bin"}

	plan, err := project.Plan(resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	panes := plan.Windows()[0].Panes()
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

	t.Run("puts the project's runtimes first on PATH", func(t *testing.T) {
		if got, want := backPlan.Path(), []string{"/nvm/v22/bin", "/cache/php/8.4"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("lets a pane pin another version than its project", func(t *testing.T) {
		if got, want := frontPlan.Path(), []string{"/nvm/v20/bin", "/cache/php/8.4"}; !slices.Equal(got, want) {
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

	t.Run("fails naming the window and pane when a version isn't installed", func(t *testing.T) {
		_, err := project.Plan(installed{})
		if !errors.Is(err, domain.ErrRuntimeNotInstalled) || !strings.Contains(err.Error(), `window "shop": pane "back"`) {
			t.Fatalf("got %v, want ErrRuntimeNotInstalled naming the window and pane", err)
		}
	})
}
