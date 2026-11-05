package infra_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// load writes doc as the config of project name in a fresh directory and
// reads it back through the repository.
func load(t *testing.T, name, doc string) (domain.Project, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(doc), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	projectName, err := domain.NewProjectName(name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return infra.NewJSONProjectRepository(dir).FindByName(projectName)
}

// loadFixture reads testdata/<name>.json, which must be valid.
func loadFixture(t *testing.T, name string) domain.Project {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	project, err := load(t, name, string(doc))
	if err != nil {
		t.Fatalf("loading fixture: %v", err)
	}
	return project
}

func paneNamed(t *testing.T, project domain.Project, window, pane string) domain.Pane {
	t.Helper()
	for _, w := range project.Windows() {
		if w.Name().String() != window {
			continue
		}
		for _, p := range w.Panes() {
			if p.Name().String() == pane {
				return p
			}
		}
	}
	t.Fatalf("no pane %q in window %q", pane, window)
	return domain.Pane{}
}

func TestJSONProjectRepositoryFindByName(t *testing.T) {
	t.Run("loads a config that only sets the root", func(t *testing.T) {
		project, err := load(t, "weaver", `{"root": "/home/ilies/dev/weaver"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "weaver (/home/ilies/dev/weaver)"
		if got := project.Describe(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("expands a leading ~ in the root to the home directory", func(t *testing.T) {
		t.Setenv("HOME", "/home/someone")
		project, err := load(t, "portable", `{"root": "~/dev/portable"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := project.Root().String(); got != "/home/someone/dev/portable" {
			t.Fatalf("got %q, want %q", got, "/home/someone/dev/portable")
		}
	})

	t.Run("reports a not-found error when the file is missing", func(t *testing.T) {
		name, err := domain.NewProjectName("missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = infra.NewJSONProjectRepository(t.TempDir()).FindByName(name)
		if _, ok := errors.AsType[domain.ProjectNotFound](err); !ok {
			t.Fatalf("got %v, want ProjectNotFound", err)
		}
	})
}

func TestJSONProjectRepositoryReadsLayouts(t *testing.T) {
	bicyclette := loadFixture(t, "bicyclette")

	t.Run("reads windows in order", func(t *testing.T) {
		var names []string
		for _, w := range bicyclette.Windows() {
			names = append(names, w.Name().String())
		}
		if want := []string{"api", "app", "repo"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})

	t.Run("reads panes in creation order", func(t *testing.T) {
		var names []string
		for _, p := range bicyclette.Windows()[0].Panes() {
			names = append(names, p.Name().String())
		}
		if want := []string{"api shell", "task dev", "psql"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})

	t.Run("reads pane directories", func(t *testing.T) {
		if got := paneNamed(t, bicyclette, "api", "api shell").Dir().String(); got != "api" {
			t.Fatalf("got %q, want %q", got, "api")
		}
	})

	t.Run("reads a split's direction and size", func(t *testing.T) {
		split, _ := paneNamed(t, bicyclette, "api", "task dev").Split()
		if split.Direction() != domain.Right || split.Size().String() != "40%" {
			t.Fatalf("got %v %q, want right 40%%", split.Direction(), split.Size().String())
		}
	})

	t.Run("defaults a split's target to the previous pane", func(t *testing.T) {
		split, _ := paneNamed(t, bicyclette, "api", "psql").Split()
		if split.Of() != 1 {
			t.Fatalf("got pane #%d, want #2 (task dev)", split.Of()+1)
		}
	})

	t.Run("reads a command to run", func(t *testing.T) {
		startup := paneNamed(t, bicyclette, "api", "task dev").Startup()
		cmd, _ := startup.Command()
		if cmd.String() != "task dev" || !startup.Submits() {
			t.Fatalf("got %q (submits: %v), want task dev submitted", cmd.String(), startup.Submits())
		}
	})

	t.Run("reads a command to leave at the prompt", func(t *testing.T) {
		startup := paneNamed(t, bicyclette, "app", "emulator").Startup()
		cmd, _ := startup.Command()
		if cmd.String() != "bicyclette emulator" || startup.Submits() {
			t.Fatalf("got %q (submits: %v), want bicyclette emulator not submitted", cmd.String(), startup.Submits())
		}
	})

	t.Run("gives a window without panes one shell in the root", func(t *testing.T) {
		panes := bicyclette.Windows()[2].Panes()
		if len(panes) != 1 || !panes[0].Dir().IsRoot() {
			t.Fatalf("got %d panes, want one shell in the root", len(panes))
		}
	})
}

func TestJSONProjectRepositoryReadsEnvironments(t *testing.T) {
	shop := loadFixture(t, "shop")

	t.Run("reads repositories by directory", func(t *testing.T) {
		var got []string
		for _, c := range shop.Checkouts().List() {
			got = append(got, c.Dir()+" "+c.Remote())
		}
		want := []string{"back git@bitbucket.org:acme/shop-back.git", "front git@bitbucket.org:acme/shop-front.git"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("reads project env", func(t *testing.T) {
		vars := shop.Env().Vars()
		if len(vars) != 1 || vars[0].Name() != "APP_ENV" || vars[0].Value() != "local" {
			t.Fatalf("got %v, want APP_ENV=local", vars)
		}
	})

	t.Run("reads pane env", func(t *testing.T) {
		vars := paneNamed(t, shop, "shop", "front").Env().Vars()
		if len(vars) != 1 || vars[0].Value() != "dev" {
			t.Fatalf("got %v, want APP_ENV=dev", vars)
		}
	})

	t.Run("reads pane runtimes", func(t *testing.T) {
		runtimes := paneNamed(t, shop, "shop", "back").Runtimes().List()
		if len(runtimes) != 1 || runtimes[0].String() != "php 8.4" {
			t.Fatalf("got %v, want php 8.4", runtimes)
		}
	})

	t.Run("reads a note line by line", func(t *testing.T) {
		got := paneNamed(t, shop, "shop", "front").Note()
		want := []string{"", "Usual commands:", "", "  npm i", "  npm run dev", ""}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestJSONProjectRepositoryResolvesSplitTargetsByName(t *testing.T) {
	project, err := load(t, "grid", `{"root": "/srv/grid", "windows": [{"name": "main", "panes": [
		{"name": "left"},
		{"name": "right", "split": {"direction": "right"}},
		{"name": "under left", "split": {"of": "left", "direction": "below"}}
	]}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	split, _ := paneNamed(t, project, "main", "under left").Split()
	if split.Of() != 0 {
		t.Fatalf("got pane #%d, want #1 (left)", split.Of()+1)
	}
}

func TestJSONProjectRepositoryRejectsInvalidConfigs(t *testing.T) {
	t.Run("rejects unknown fields, usually typos", func(t *testing.T) {
		_, err := load(t, "typo", `{"root": "/srv/typo", "windos": []}`)
		if err == nil || !strings.Contains(err.Error(), `unknown field "windos"`) {
			t.Fatalf("got %v, want an unknown field error", err)
		}
	})

	t.Run("rejects data after the config object", func(t *testing.T) {
		_, err := load(t, "twice", `{"root": "/srv/twice"} {"root": "/srv/again"}`)
		if !errors.Is(err, infra.ErrTrailingData) {
			t.Fatalf("got %v, want ErrTrailingData", err)
		}
	})

	t.Run("rejects a relative root", func(t *testing.T) {
		_, err := load(t, "relative", `{"root": "dev/relative"}`)
		if !errors.Is(err, domain.ErrRelativeProjectRoot) {
			t.Fatalf("got %v, want ErrRelativeProjectRoot", err)
		}
	})

	t.Run("rejects a pane that both runs and prefills a command", func(t *testing.T) {
		_, err := load(t, "both", `{"root": "/srv/both", "windows": [{"name": "main", "panes": [
			{"command": "task dev", "prefill": "task check"}
		]}]}`)
		if !errors.Is(err, infra.ErrCommandAndPrefill) {
			t.Fatalf("got %v, want ErrCommandAndPrefill", err)
		}
	})

	t.Run("rejects a split of a pane the window doesn't have", func(t *testing.T) {
		_, err := load(t, "ghost", `{"root": "/srv/ghost", "windows": [{"name": "main", "panes": [
			{"name": "left"},
			{"name": "right", "split": {"of": "middle", "direction": "right"}}
		]}]}`)
		if !errors.Is(err, infra.ErrUnknownSplitTarget) {
			t.Fatalf("got %v, want ErrUnknownSplitTarget", err)
		}
	})

	t.Run("rejects a split of a pane created later", func(t *testing.T) {
		_, err := load(t, "early", `{"root": "/srv/early", "windows": [{"name": "main", "panes": [
			{"name": "left"},
			{"name": "right", "split": {"of": "bottom", "direction": "right"}},
			{"name": "bottom", "split": {"direction": "below"}}
		]}]}`)
		if !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("says which window and pane is wrong", func(t *testing.T) {
		_, err := load(t, "where", `{"root": "/srv/where", "windows": [{"name": "api", "panes": [
			{"name": "shell"},
			{"name": "psql", "split": {"direction": "left"}}
		]}]}`)
		if err == nil || !strings.Contains(err.Error(), `window "api": pane "psql": split:`) {
			t.Fatalf("got %v, want the window and pane named", err)
		}
	})
}

func TestJSONProjectRepositoryNames(t *testing.T) {
	t.Run("lists project configs sorted, skipping other files", func(t *testing.T) {
		dir := t.TempDir()
		for _, file := range []string{"shop.json", "bicyclette.json", "weaver.schema.json", "notes.txt", "show.json"} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(`{}`), 0o600); err != nil {
				t.Fatalf("writing fixture: %v", err)
			}
		}
		if err := os.Mkdir(filepath.Join(dir, "folder.json"), 0o755); err != nil {
			t.Fatalf("creating fixture: %v", err)
		}

		names, err := infra.NewJSONProjectRepository(dir).Names()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got []string
		for _, name := range names {
			got = append(got, name.String())
		}
		if want := []string{"bicyclette", "shop"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("has no projects when the directory is missing", func(t *testing.T) {
		names, err := infra.NewJSONProjectRepository(filepath.Join(t.TempDir(), "missing")).Names()
		if err != nil || len(names) != 0 {
			t.Fatalf("got %v (%v), want no projects", names, err)
		}
	})
}
