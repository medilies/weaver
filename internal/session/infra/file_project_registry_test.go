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

	t.Run("reads pane directories", func(t *testing.T) {
		if got := paneNamed(t, nimbus, "api", "api shell").Dir().String(); got != "api" {
			t.Fatalf("got %q, want %q", got, "api")
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
		if len(panes) != 1 || !panes[0].Dir().IsRoot() {
			t.Fatalf("got %d panes, want one shell in the root", len(panes))
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
	project, err := load(t, "grid", gridDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	split, _ := paneNamed(t, project, "main", "under left").Split()
	if split.Of() != 0 {
		t.Fatalf("got pane #%d, want #1 (left)", split.Of()+1)
	}
}

func TestFileProjectRegistryRejectsInvalidConfigs(t *testing.T) {
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
