package infra_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// load writes doc as the config of project name in a fresh directory and
// reads it back through the registry.
func load(t *testing.T, name, doc string) (domain.Project, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name+".yml"), []byte(doc), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	projectName, err := domain.NewProjectName(name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return infra.NewFileProjectRegistry(dir).FindByName(projectName)
}

// loadFixture reads testdata/<name>.yml, which must be valid.
func loadFixture(t *testing.T, name string) domain.Project {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("testdata", name+".yml"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	project, err := load(t, name, string(doc))
	if err != nil {
		t.Fatalf("loading fixture: %v", err)
	}
	return project
}

// planDir is the absolute directory the pane starts in.
func planDir(t *testing.T, project domain.Project, window, pane string) string {
	t.Helper()
	for _, w := range project.Plan().Windows() {
		if w.Name().String() != window {
			continue
		}
		for _, p := range w.Panes() {
			if p.Name().String() == pane {
				return p.Dir()
			}
		}
	}
	t.Fatalf("no pane %q in window %q", pane, window)
	return ""
}

// ownDir is the pane's own dir, which it must have.
func ownDir(t *testing.T, pane domain.Pane) string {
	t.Helper()
	dir, set := pane.Dir()
	if !set {
		t.Fatalf("pane %q has no dir of its own", pane.Name())
	}
	return dir.String()
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

func TestFileProjectRegistryFindByName(t *testing.T) {
	t.Run("loads a config that only sets the root", func(t *testing.T) {
		project, err := load(t, "weaver", "root: /home/ilies/dev/weaver\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/home/ilies/dev/weaver"
		if got := project.Root().String(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("expands a leading ~ in the root to the home directory", func(t *testing.T) {
		t.Setenv("HOME", "/home/someone")
		project, err := load(t, "portable", "root: ~/dev/portable\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := project.Root().String(); got != "/home/someone/dev/portable" {
			t.Fatalf("got %q, want %q", got, "/home/someone/dev/portable")
		}
	})

	t.Run("expands a leading ~ in a pane's dir to the home directory", func(t *testing.T) {
		t.Setenv("HOME", "/home/someone")
		project, err := load(t, "portable", "root: /srv/portable\nwindows: [{name: main, panes: [{name: notes, dir: ~/notes}]}]\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := ownDir(t, paneNamed(t, project, "main", "notes")); got != "/home/someone/notes" {
			t.Fatalf("got %q, want %q", got, "/home/someone/notes")
		}
	})

	t.Run("reports a not-found error when the file is missing", func(t *testing.T) {
		name, err := domain.NewProjectName("missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = infra.NewFileProjectRegistry(t.TempDir()).FindByName(name)
		if !errors.Is(err, domain.ErrProjectNotFound) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})
}

func TestFileProjectRegistryReadsLayouts(t *testing.T) {
	nimbus := loadFixture(t, "nimbus")

	t.Run("reads windows in order", func(t *testing.T) {
		var names []string
		for _, w := range nimbus.Windows() {
			names = append(names, w.Name().String())
		}
		if want := []string{"api", "app", "repo"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})

	t.Run("reads panes in creation order", func(t *testing.T) {
		var names []string
		for _, p := range nimbus.Windows()[0].Panes() {
			names = append(names, p.Name().String())
		}
		if want := []string{"api shell", "task dev", "psql"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})

	t.Run("starts a pane without a dir in its window's", func(t *testing.T) {
		want := filepath.Join(nimbus.Root().String(), "api")
		if got := planDir(t, nimbus, "api", "api shell"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("reads . as a pane's dir, starting it in the root rather than its window's dir", func(t *testing.T) {
		if got, want := planDir(t, nimbus, "api", "psql"), nimbus.Root().String(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("reads a blank pane dir as its window's", func(t *testing.T) {
		project := mustLoad(t, "shop", "root: /srv/shop\nwindows: [{name: main, dir: api, panes: [{name: web, dir: \" \"}]}]\n")
		if got := planDir(t, project, "main", "web"); got != "/srv/shop/api" {
			t.Fatalf("got %q, want /srv/shop/api", got)
		}
	})

	t.Run("reads a pane's dir relative to the root, not to its window's dir", func(t *testing.T) {
		project := mustLoad(t, "shop", "root: /srv/shop\nwindows: [{name: main, dir: api, panes: [{name: web, dir: web}]}]\n")
		if got := planDir(t, project, "main", "web"); got != "/srv/shop/web" {
			t.Fatalf("got %q, want /srv/shop/web", got)
		}
	})

	t.Run("reads a split's direction and size", func(t *testing.T) {
		split, _ := paneNamed(t, nimbus, "api", "task dev").Split()
		if split.Direction() != domain.Right || split.Size().String() != "40%" {
			t.Fatalf("got %v %q, want right 40%%", split.Direction(), split.Size().String())
		}
	})

	t.Run("defaults a split's target to the previous pane", func(t *testing.T) {
		split, _ := paneNamed(t, nimbus, "api", "psql").Split()
		if split.Of() != 1 {
			t.Fatalf("got pane #%d, want #2 (task dev)", split.Of()+1)
		}
	})

	t.Run("reads a command to run", func(t *testing.T) {
		startup := paneNamed(t, nimbus, "api", "task dev").Startup()
		cmd, _ := startup.Command()
		if cmd.String() != "task dev" || !startup.Submits() {
			t.Fatalf("got %q (submits: %v), want task dev submitted", cmd.String(), startup.Submits())
		}
	})

	t.Run("reads a command to leave at the prompt", func(t *testing.T) {
		startup := paneNamed(t, nimbus, "app", "emulator").Startup()
		cmd, _ := startup.Command()
		if cmd.String() != "nimbus emulator" || startup.Submits() {
			t.Fatalf("got %q (submits: %v), want nimbus emulator not submitted", cmd.String(), startup.Submits())
		}
	})

	t.Run("gives a window without panes one shell in the root", func(t *testing.T) {
		panes := nimbus.Windows()[2].Panes()
		if len(panes) != 1 {
			t.Fatalf("got %d panes, want one", len(panes))
		}
		if _, set := panes[0].Dir(); set {
			t.Fatal("got a pane with its own dir, want one starting in the root")
		}
	})
}

func TestFileProjectRegistryReadsEnvironments(t *testing.T) {
	shop := loadFixture(t, "shop")

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

	t.Run("reads the commands to run first in every pane", func(t *testing.T) {
		if got := shop.Before(); len(got) != 1 || got[0].String() != "source ~/.shop_aliases" {
			t.Fatalf("got %v, want source ~/.shop_aliases", got)
		}
	})

	t.Run("reads the setup commands in order", func(t *testing.T) {
		got := lines(shop.Setup())
		if want := []string{"docker compose up -d --wait", "php artisan migrate"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("reads the teardown commands", func(t *testing.T) {
		if got := lines(shop.Teardown()); !slices.Equal(got, []string{"docker compose down"}) {
			t.Fatalf("got %q, want docker compose down", got)
		}
	})

	t.Run("reads the commands to run first in a window's panes", func(t *testing.T) {
		if got := shop.Windows()[0].Before(); len(got) != 1 || got[0].String() != "git fetch --quiet" {
			t.Fatalf("got %v, want git fetch --quiet", got)
		}
	})

	t.Run("reads the commands to run first in a pane", func(t *testing.T) {
		if got := paneNamed(t, shop, "shop", "back").Before(); len(got) != 1 || got[0].String() != "alias php=php8.4" {
			t.Fatalf("got %v, want alias php=php8.4", got)
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

func TestFileProjectRegistryResolvesSplitTargetsByName(t *testing.T) {
	project, err := load(t, "splits", splitOfDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	split, _ := paneNamed(t, project, "main", "under left").Split()
	if split.Of() != 0 {
		t.Fatalf("got pane #%d, want #1 (left)", split.Of()+1)
	}
}

func TestFileProjectRegistryLaysOutGrids(t *testing.T) {
	project, err := load(t, "grid", gridDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []string
	for _, pane := range project.Windows()[0].Panes() {
		desc := pane.Name().String()
		if split, ok := pane.Split(); ok {
			desc += fmt.Sprintf(" %s of #%d %s", split.Direction(), split.Of()+1, split.Size())
		}
		got = append(got, desc)
	}
	if want := []string{"editor", "server right of #1 40%", "go test below of #2 50%"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFileProjectRegistryRejectsInvalidConfigs(t *testing.T) {
	t.Run("names an empty setup command", func(t *testing.T) {
		_, err := load(t, "empty", "root: /srv/empty\nsetup: [\"  \"]\n")
		if !errors.Is(err, domain.ErrEmptyCommand) || !strings.Contains(err.Error(), "invalid config: setup: ") {
			t.Fatalf("got %v, want an empty command error under setup", err)
		}
	})

	t.Run("names an empty teardown command", func(t *testing.T) {
		_, err := load(t, "empty", "root: /srv/empty\nteardown: [\"\"]\n")
		if !errors.Is(err, domain.ErrEmptyCommand) || !strings.Contains(err.Error(), "invalid config: teardown: ") {
			t.Fatalf("got %v, want an empty command error under teardown", err)
		}
	})

	t.Run("rejects a window laid out by both a grid and splits", func(t *testing.T) {
		_, err := load(t, "mixed", `root: /srv/mixed
windows:
  - name: app
    grid:
      areas: [[app shell, flutter run]]
    panes:
      - name: app shell
      - name: flutter run
        split: { direction: right }
`)
		want := `window "app": pane "flutter run": a window laid out by a grid must not split its panes`
		if !errors.Is(err, domain.ErrSplitInGrid) || !strings.Contains(err.Error(), want) {
			t.Fatalf("got %v, want %q", err, want)
		}
	})

	t.Run("rejects a null grid rather than reading it as splits", func(t *testing.T) {
		_, err := load(t, "null", "root: /srv/null\nwindows: [{name: app, grid: null, panes: [{name: a}, {name: b, split: {direction: right}}]}]\n")
		if !errors.Is(err, domain.ErrEmptyGrid) || !strings.Contains(err.Error(), `window "app": grid: `) {
			t.Fatalf("got %v, want an empty grid error for the app window", err)
		}
	})

	t.Run("reports a grid's error under its window's grid", func(t *testing.T) {
		_, err := load(t, "sizes", "root: /srv/sizes\nwindows: [{name: app, grid: {columns: [60%], areas: [[a, b]]}, panes: [{name: a}, {name: b}]}]\n")
		if !errors.Is(err, domain.ErrTrackCount) || !strings.Contains(err.Error(), `window "app": grid: `) {
			t.Fatalf("got %v, want a size count error under the app window's grid", err)
		}
	})

	t.Run("rejects unknown keys, usually typos", func(t *testing.T) {
		_, err := load(t, "typo", "root: /srv/typo\nwindos: []\n")
		if !errors.Is(err, domain.ErrInvalidConfig) || !strings.Contains(err.Error(), "windos") {
			t.Fatalf("got %v, want an invalid config naming windos", err)
		}
	})

	t.Run("rejects repositories, which weaver no longer reads", func(t *testing.T) {
		_, err := load(t, "old", "root: /srv/old\nrepositories: {.: git@host:old.git}\n")
		if !errors.Is(err, domain.ErrInvalidConfig) || !strings.Contains(err.Error(), "repositories") {
			t.Fatalf("got %v, want an invalid config naming repositories", err)
		}
	})

	t.Run("names a field's place as the config does, not by its Go type", func(t *testing.T) {
		_, err := load(t, "typo", "root: /srv/typo\nwindows: [{name: a, panes: [{comand: ls}]}]\n")
		if err == nil || !strings.Contains(err.Error(), "not found in type pane") || strings.Contains(err.Error(), "infra.") {
			t.Fatalf("got %v, want comand reported as not found in a pane", err)
		}
	})

	t.Run("rejects a second document", func(t *testing.T) {
		_, err := load(t, "twice", "root: /srv/a\n---\nroot: /srv/b\n")
		if !errors.Is(err, infra.ErrTrailingData) {
			t.Fatalf("got %v, want ErrTrailingData", err)
		}
	})

	t.Run("reports the missing root of an empty config", func(t *testing.T) {
		_, err := load(t, "empty", "# nothing yet\n")
		if !errors.Is(err, domain.ErrEmptyProjectRoot) {
			t.Fatalf("got %v, want ErrEmptyProjectRoot", err)
		}
	})

	t.Run("rejects a relative root", func(t *testing.T) {
		_, err := load(t, "relative", "root: dev/relative\n")
		if !errors.Is(err, domain.ErrRelativeProjectRoot) {
			t.Fatalf("got %v, want ErrRelativeProjectRoot", err)
		}
	})

	t.Run("rejects a bare ~ as the root, which YAML reads as null", func(t *testing.T) {
		_, err := load(t, "home", "root: ~\n")
		if !errors.Is(err, infra.ErrUnquotedHome) {
			t.Fatalf("got %v, want ErrUnquotedHome", err)
		}
	})

	t.Run("rejects a bare ~ as a window's dir, naming the window", func(t *testing.T) {
		_, err := load(t, "dots", "root: /srv/dots\nwindows: [{name: main, dir: ~}]\n")
		if !errors.Is(err, infra.ErrUnquotedHome) || !strings.Contains(err.Error(), `window "main": dir:`) {
			t.Fatalf("got %v, want ErrUnquotedHome naming the window", err)
		}
	})

	t.Run("rejects a bare ~ as a pane's dir, naming the pane", func(t *testing.T) {
		_, err := load(t, "home", "root: /srv/home\nwindows: [{name: main, panes: [{name: dots, dir: ~}]}]\n")
		if !errors.Is(err, infra.ErrUnquotedHome) || !strings.Contains(err.Error(), `window "main": pane "dots": dir:`) {
			t.Fatalf("got %v, want ErrUnquotedHome naming the pane", err)
		}
	})

	t.Run("reads a quoted ~ as the home directory", func(t *testing.T) {
		project, err := load(t, "home", "root: \"~\"\n")
		home, _ := os.UserHomeDir()
		if err != nil || project.Root().String() != home {
			t.Fatalf("got root %v (err %v), want %q", project.Root(), err, home)
		}
	})

	t.Run("rejects a pane that both runs and prefills a command", func(t *testing.T) {
		_, err := load(t, "both", `root: /srv/both
windows:
  - name: main
    panes:
      - command: task dev
        prefill: task check
`)
		if !errors.Is(err, infra.ErrCommandAndPrefill) {
			t.Fatalf("got %v, want ErrCommandAndPrefill", err)
		}
	})

	t.Run("rejects a task without a command, naming it", func(t *testing.T) {
		_, err := load(t, "task", `root: /srv/task
windows:
  - name: main
    panes:
      - tasks:
          - {command: make}
          - {name: migrate}
`)
		if !errors.Is(err, domain.ErrEmptyCommand) || !strings.Contains(err.Error(), "tasks: #2") {
			t.Fatalf("got %v, want ErrEmptyCommand for task #2", err)
		}
	})

	t.Run("rejects two tasks of a pane with the same name", func(t *testing.T) {
		_, err := load(t, "task", `root: /srv/task
windows:
  - name: main
    panes:
      - tasks:
          - {name: seed, command: make seed}
          - {name: seed, command: make reseed}
`)
		if !errors.Is(err, domain.ErrDuplicateTask) {
			t.Fatalf("got %v, want ErrDuplicateTask", err)
		}
	})

	t.Run("rejects a split of a pane the window doesn't have", func(t *testing.T) {
		_, err := load(t, "ghost", `root: /srv/ghost
windows:
  - name: main
    panes:
      - name: left
      - name: right
        split: { of: middle, direction: right }
`)
		if !errors.Is(err, infra.ErrUnknownSplitTarget) {
			t.Fatalf("got %v, want ErrUnknownSplitTarget", err)
		}
	})

	t.Run("rejects a split of a pane created later", func(t *testing.T) {
		_, err := load(t, "early", `root: /srv/early
windows:
  - name: main
    panes:
      - name: left
      - name: right
        split: { of: bottom, direction: right }
      - name: bottom
        split: { direction: below }
`)
		if !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("says which window and pane is wrong", func(t *testing.T) {
		_, err := load(t, "where", `root: /srv/where
windows:
  - name: api
    panes:
      - name: shell
      - name: psql
        split: { direction: left }
`)
		if err == nil || !strings.Contains(err.Error(), `window "api": pane "psql": split:`) {
			t.Fatalf("got %v, want the window and pane named", err)
		}
	})
}

// writeConfigs writes each file, holding a minimal config, in a fresh
// directory and returns it.
func writeConfigs(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("root: /srv\n"), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
	}
	return dir
}

func TestFileProjectRegistryNames(t *testing.T) {
	t.Run("lists project configs sorted, skipping other files", func(t *testing.T) {
		dir := t.TempDir()
		for _, file := range []string{"shop.yml", "nimbus.yml", "a-b.yml", "a.yml", "weaver.schema.json", "notes.txt", "old.json", "blog.yaml", "show.yml"} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("root: /srv\n"), 0o600); err != nil {
				t.Fatalf("writing fixture: %v", err)
			}
		}
		if err := os.Mkdir(filepath.Join(dir, "folder.yml"), 0o755); err != nil {
			t.Fatalf("creating fixture: %v", err)
		}

		names, err := infra.NewFileProjectRegistry(dir).Names()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got []string
		for _, name := range names {
			got = append(got, name.String())
		}
		if want := []string{"a", "a-b", "nimbus", "shop"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("warns about a config skipped for its name", func(t *testing.T) {
		dir := writeConfigs(t, "shop.yml", "edit.yml", "my.shop.yml", ".#shop.yml")
		var warnings strings.Builder

		if _, err := infra.NewFileProjectRegistry(dir).WithWarnings(&warnings).Names(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "warning: skipping " + filepath.Join(dir, "edit.yml") +
			": project name is a weave command: \"edit\"; rename it to register it\n" +
			"warning: skipping " + filepath.Join(dir, "my.shop.yml") +
			": " + domain.ErrInvalidProjectName.Error() + "; rename it to register it\n"
		if got := warnings.String(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("warns once per skipped config", func(t *testing.T) {
		dir := writeConfigs(t, "edit.yml")
		var warnings strings.Builder
		configs := infra.NewFileProjectRegistry(dir).WithWarnings(&warnings)

		for range 2 {
			if _, err := configs.Names(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}

		if got := strings.Count(warnings.String(), "warning:"); got != 1 {
			t.Fatalf("got %d warnings, want 1", got)
		}
	})

	t.Run("has no projects when the directory is missing", func(t *testing.T) {
		names, err := infra.NewFileProjectRegistry(filepath.Join(t.TempDir(), "missing")).Names()
		if err != nil || len(names) != 0 {
			t.Fatalf("got %v (%v), want no projects", names, err)
		}
	})
}

func TestFileProjectRegistryDelete(t *testing.T) {
	name, _ := domain.NewProjectName("shop")

	t.Run("removes the project's config", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "shop.yml"), []byte("root: /srv/shop\n"), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		configs := infra.NewFileProjectRegistry(dir)
		if err := configs.Delete(name); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := configs.FindByName(name); !isNotFound(err) {
			t.Fatalf("got %v, want ErrProjectNotFound once deleted", err)
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		if err := infra.NewFileProjectRegistry(t.TempDir()).Delete(name); !isNotFound(err) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})
}

// lines is the commands' text, in order.
func lines(cmds []domain.Command) []string {
	out := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		out = append(out, cmd.String())
	}
	return out
}
