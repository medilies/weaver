package infra_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// load writes doc as the config of project name in a fresh directory,
// lists its root in the settings, naming the project name, and reads it
// back through the registry. The root is the one doc gives, moved out of
// the config, else /srv/<name>.
func load(t *testing.T, name, doc string) (domain.Project, error) {
	t.Helper()
	dir := t.TempDir()
	root, doc := movedRoot(t, doc)
	if root == "" {
		root = "/srv/" + name
	}
	config := filepath.Join(dir, name+".yml")
	write(t, config, doc)
	settings := filepath.Join(dir, "settings.yml")
	write(t, settings, fmt.Sprintf("projects:\n  - root: %s\n    config: %s\n    name: %s\n", root, config, name))
	configs := infra.NewFileProjectRegistry(settings)
	refs, err := configs.Registered()
	if err != nil || len(refs) != 1 {
		t.Fatalf("got %v (%v) registered, want %s", refs, err, root)
	}
	return configs.Find(refs[0].Root())
}

// rootLine is a config's root line.
var rootLine = regexp.MustCompile(`(?m)^root: *(.*)\n?`)

// movedRoot is the root doc gives, as YAML writes it, and doc without
// it: its root line taken out, or, in a doc written on one line, as
// JSON, its root field.
func movedRoot(t *testing.T, doc string) (string, string) {
	t.Helper()
	if found := rootLine.FindStringSubmatch(doc); found != nil {
		return found[1], strings.Replace(doc, found[0], "", 1)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(doc), &node); err != nil || len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		return "", doc
	}
	fields := node.Content[0]
	for i := 0; i+1 < len(fields.Content); i += 2 {
		if fields.Content[i].Value != "root" {
			continue
		}
		root := strconv.Quote(fields.Content[i+1].Value)
		fields.Content = slices.Delete(fields.Content, i, i+2)
		moved, err := yaml.Marshal(&node)
		if err != nil {
			t.Fatalf("taking the root out of %q: %v", doc, err)
		}
		return root, string(moved)
	}
	return "", doc
}

// loadFixture reads testdata/<name>.yml, which must be valid, rooted at
// ~/dev/<name>.
func loadFixture(t *testing.T, name string) domain.Project {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("testdata", name+".yml"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	project, err := load(t, name, "root: ~/dev/"+name+"\n"+string(doc))
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

func TestFileProjectRegistryFind(t *testing.T) {
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

	t.Run("names the project as its config does", func(t *testing.T) {
		configs, root := registeredShop(t, "name: store\n")
		if project, err := configs.Find(root); err != nil || project.Name().String() != "store" {
			t.Fatalf("got %v (%v), want store", project.Name(), err)
		}
	})

	t.Run("names the project after its root's directory, whatever it holds", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "weaver.nvim")
		write(t, filepath.Join(root, "weaver.yml"), "{}\n")
		configs, _ := listing(t, "  - "+root+"\n")
		if project, err := configs.Find(mustRoot(t, root)); err != nil || project.Name().String() != "weaver.nvim" {
			t.Fatalf("got %v (%v), want weaver.nvim", project.Name(), err)
		}
	})

	t.Run("reports a root that isn't registered", func(t *testing.T) {
		configs := infra.NewFileProjectRegistry(filepath.Join(t.TempDir(), "settings.yml"))
		if _, err := configs.Find(mustRoot(t, "/srv/missing")); !errors.Is(err, domain.ErrProjectNotFound) {
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

func TestFileProjectRegistryReadsFocus(t *testing.T) {
	nimbus := loadFixture(t, "nimbus")

	t.Run("reads the focused window", func(t *testing.T) {
		if got := nimbus.FocusedWindow(); got != 1 {
			t.Fatalf("got window #%d, want #2, app", got+1)
		}
	})

	t.Run("reads a window's focused pane", func(t *testing.T) {
		if got := nimbus.Windows()[1].FocusedPane(); got != 1 {
			t.Fatalf("got pane #%d, want #2, flutter run", got+1)
		}
	})

	t.Run("rejects two focused windows", func(t *testing.T) {
		_, err := load(t, "shop", "root: /srv/shop\nwindows: [{name: a, focus: true}, {name: b, focus: true}]\n")
		if !errors.Is(err, domain.ErrFocusedWindowTwice) {
			t.Fatalf("got %v, want ErrFocusedWindowTwice", err)
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

	t.Run("reads window env", func(t *testing.T) {
		vars := shop.Windows()[0].Env().Vars()
		if len(vars) != 1 || vars[0].Name() != "LOG_LEVEL" || vars[0].Value() != "debug" {
			t.Fatalf("got %v, want LOG_LEVEL=debug", vars)
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
		_, err := load(t, "twice", "env: {A: a}\n---\nenv: {A: b}\n")
		if !errors.Is(err, infra.ErrTrailingData) {
			t.Fatalf("got %v, want ErrTrailingData", err)
		}
	})

	t.Run("loads an empty config", func(t *testing.T) {
		if _, err := load(t, "empty", "# nothing yet\n"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("refuses a root written in the config, which the settings hold", func(t *testing.T) {
		configs, root := registeredShop(t, "root: /srv/shop\n")
		if _, err := configs.Find(root); !errors.Is(err, infra.ErrRootInConfig) || !errors.Is(err, domain.ErrInvalidConfig) {
			t.Fatalf("got %v, want ErrRootInConfig", err)
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

// listing writes the settings listing projects, a YAML list's items,
// in a fresh directory, and returns its registry and the directory.
func listing(t *testing.T, projects string) (infra.FileProjectRegistry, string) {
	t.Helper()
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.yml")
	write(t, settings, "projects:\n"+projects)
	return infra.NewFileProjectRegistry(settings), dir
}

// described is refs as "name root".
func described(refs []domain.ProjectRef) []string {
	var got []string
	for _, ref := range refs {
		got = append(got, ref.Name().String()+" "+ref.Root().String())
	}
	return got
}

func TestFileProjectRegistryRegistered(t *testing.T) {
	t.Run("lists the settings' roots in order, named after their directories", func(t *testing.T) {
		configs, _ := listing(t, "  - /srv/shop\n  - root: /srv/blog\n  - /srv/api\n")
		refs, err := configs.Registered()
		if want := []string{"shop /srv/shop", "blog /srv/blog", "api /srv/api"}; err != nil || !slices.Equal(described(refs), want) {
			t.Fatalf("got %q (%v), want %q", described(refs), err, want)
		}
	})

	t.Run("names a project by its entry, else by its config", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "shop", "weaver.yml"), "name: store\n")
		write(t, filepath.Join(dir, "blog", "weaver.yml"), "name: journal\n")
		configs, _ := listing(t, "  - "+filepath.Join(dir, "shop")+"\n  - root: "+filepath.Join(dir, "blog")+"\n    name: notes\n")
		refs, err := configs.Registered()
		if err != nil || len(refs) != 2 || refs[0].Name().String() != "store" || refs[1].Name().String() != "notes" {
			t.Fatalf("got %q (%v), want store and notes", described(refs), err)
		}
	})

	t.Run("lets two projects share a name", func(t *testing.T) {
		configs, _ := listing(t, "  - /srv/a/api\n  - /srv/b/api\n")
		if refs, err := configs.Registered(); err != nil || len(refs) != 2 {
			t.Fatalf("got %q (%v), want both apis", described(refs), err)
		}
	})

	t.Run("lists a root listed twice once, which it can't find", func(t *testing.T) {
		configs, _ := listing(t, "  - /srv/shop\n  - root: /srv/shop/\n    config: other.yml\n")
		refs, err := configs.Registered()
		if err != nil || len(refs) != 1 {
			t.Fatalf("got %q (%v), want shop once", described(refs), err)
		}
		if _, err := configs.Find(refs[0].Root()); !errors.Is(err, domain.ErrRootListedTwice) {
			t.Fatalf("got %v, want ErrRootListedTwice", err)
		}
	})

	t.Run("skips an entry it can't read, warning once", func(t *testing.T) {
		configs, _ := listing(t, "  - dev/relative\n  - root: /srv/shop\n    colour: red\n  - /srv/blog\n")
		var warnings bytes.Buffer
		configs = configs.WithWarnings(&warnings, "")
		refs, err := configs.Registered()
		if err != nil || !slices.Equal(described(refs), []string{"blog /srv/blog"}) {
			t.Fatalf("got %q (%v), want blog alone", described(refs), err)
		}
		_, _ = configs.Registered()
		if got := warnings.String(); strings.Count(got, "warning: skipping") != 2 ||
			!strings.Contains(got, "projects[0]") || !strings.Contains(got, "unknown field colour") {
			t.Fatalf("got warnings %q, want each entry warned about once", got)
		}
	})

	t.Run("has no projects without settings", func(t *testing.T) {
		configs := infra.NewFileProjectRegistry(filepath.Join(t.TempDir(), "settings.yml"))
		if refs, err := configs.Registered(); err != nil || len(refs) != 0 {
			t.Fatalf("got %q (%v), want none", described(refs), err)
		}
	})

	t.Run("warns once that the former config directory still holds configs", func(t *testing.T) {
		configs, dir := listing(t, "  - /srv/shop\n")
		former := filepath.Join(dir, "projects")
		write(t, filepath.Join(former, "shop.yml"), "root: /srv/shop\n")
		var warnings bytes.Buffer
		configs = configs.WithWarnings(&warnings, former)
		_, _ = configs.Registered()
		_, _ = configs.Registered()
		if got := warnings.String(); strings.Count(got, former) != 1 || !strings.Contains(got, "aren't read anymore") {
			t.Fatalf("got warnings %q, want %s warned about once", got, former)
		}
	})
}

func TestFileProjectRegistryFindsConfigs(t *testing.T) {
	t.Run("prefers weaver.yml to the other names", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, ".weaver.yml"), "name: hidden\n")
		write(t, filepath.Join(root, "weaver.yml"), "name: shown\n")
		configs, _ := listing(t, "  - "+root+"\n")
		if project, err := configs.Find(mustRoot(t, root)); err != nil || project.Name().String() != "shown" {
			t.Fatalf("got %v (%v), want weaver.yml's project", project.Name(), err)
		}
	})

	t.Run("reads the config an entry names, relative to the root", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "tmux", "dev.yml"), "name: dev\n")
		configs, _ := listing(t, "  - root: "+root+"\n    config: tmux/dev.yml\n")
		if project, err := configs.Find(mustRoot(t, root)); err != nil || project.Name().String() != "dev" {
			t.Fatalf("got %v (%v), want tmux/dev.yml's project", project.Name(), err)
		}
	})

	t.Run("reads the config an entry names outside the root", func(t *testing.T) {
		config := filepath.Join(t.TempDir(), "client-api.yml")
		write(t, config, "env: {A: 1}\n")
		configs, _ := listing(t, "  - root: /srv/client-api\n    config: "+config+"\n")
		if project, err := configs.Find(mustRoot(t, "/srv/client-api")); err != nil || len(project.Env().Vars()) != 1 {
			t.Fatalf("got %v (%v), want the outside config's project", project.Env().Vars(), err)
		}
	})

	t.Run("reports a root without a config", func(t *testing.T) {
		configs, _ := listing(t, "  - "+t.TempDir()+"\n")
		refs, _ := configs.Registered()
		if _, err := configs.Find(refs[0].Root()); !errors.Is(err, domain.ErrMissingConfig) {
			t.Fatalf("got %v, want ErrMissingConfig", err)
		}
	})
}

func TestFileProjectRegistryRegisters(t *testing.T) {
	t.Setenv("HOME", "/home/someone")

	t.Run("writes weaver.yml in a new root and lists the root first", func(t *testing.T) {
		configs, dir := listing(t, "  - /srv/old\n")
		root := filepath.Join(dir, "shop")
		if err := configs.Save(domain.NewProject(mustName(t, "shop"), mustRoot(t, root)), ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "weaver.yml")); err != nil {
			t.Fatalf("got no weaver.yml: %v", err)
		}
		refs, err := configs.Registered()
		if want := []string{"shop " + root, "old /srv/old"}; err != nil || !slices.Equal(described(refs), want) {
			t.Fatalf("got %q (%v), want %q", described(refs), err, want)
		}
	})

	t.Run("lists a root under the home directory with ~", func(t *testing.T) {
		configs, dir := listing(t, "")
		config := filepath.Join(dir, "shop.yml")
		if err := configs.Save(domain.NewProject(mustName(t, "shop"), mustRoot(t, "/home/someone/dev/shop")), config); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(dir, "settings.yml")); !strings.Contains(got, "  - root: ~/dev/shop\n    config: "+config+"\n") {
			t.Fatalf("got settings:\n%s\nwant ~/dev/shop listed with its config", got)
		}
	})

	t.Run("keeps the settings' comments and other settings", func(t *testing.T) {
		configs, dir := listing(t, "  - /srv/old # the old one\n")
		settings := filepath.Join(dir, "settings.yml")
		write(t, settings, "# mine\npopup: true\nprojects:\n  - /srv/old # the old one\n")
		if err := configs.Save(domain.NewProject(mustName(t, "shop"), mustRoot(t, filepath.Join(dir, "shop"))), ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := read(t, settings)
		for _, kept := range []string{"# mine", "popup: true", "# the old one"} {
			if !strings.Contains(got, kept) {
				t.Fatalf("got settings:\n%s\nwant %q kept", got, kept)
			}
		}
	})

	t.Run("writes a registered project to its config, listing it once", func(t *testing.T) {
		configs, root := registeredShop(t, "env: {A: 1}\n")
		if err := configs.Save(domain.NewProject(mustName(t, "shop"), root), ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(root.String(), "weaver.yml")); strings.Contains(got, "env") {
			t.Fatalf("got %q, want the config replaced", got)
		}
		if refs, _ := configs.Registered(); len(refs) != 1 {
			t.Fatalf("got %q, want shop listed once", described(refs))
		}
	})

	t.Run("points a registered root at the config given", func(t *testing.T) {
		configs, root := registeredShop(t, "{}\n")
		if err := configs.Save(domain.NewProject(mustName(t, "shop"), root), "tmux/dev.yml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		document, err := configs.Document(root)
		if want := filepath.Join(root.String(), "tmux", "dev.yml"); err != nil || document.File != want {
			t.Fatalf("got %s (%v), want %s", document.File, err, want)
		}
	})

	t.Run("names a project in its config only when named otherwise than its root", func(t *testing.T) {
		configs, root := registeredShop(t, "{}\n")
		if err := configs.Save(domain.NewProject(mustName(t, "store"), root), ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(root.String(), "weaver.yml")); !strings.Contains(got, "name: store\n") {
			t.Fatalf("got %q, want the name written", got)
		}
		if err := configs.Save(domain.NewProject(mustName(t, "shop"), root), ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(root.String(), "weaver.yml")); strings.Contains(got, "name:") {
			t.Fatalf("got %q, want no name", got)
		}
	})

	t.Run("keeps the config's name when the settings name the project", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "weaver.yml"), "name: theirs\n")
		configs, _ := listing(t, "  - root: "+root+"\n    name: mine\n")
		project, err := configs.Find(mustRoot(t, root))
		if err != nil || project.Name().String() != "mine" {
			t.Fatalf("got %v (%v), want mine", project.Name(), err)
		}
		if err := configs.Save(project, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(root, "weaver.yml")); !strings.Contains(got, "name: theirs\n") {
			t.Fatalf("got %q, want the config's name kept", got)
		}
	})
}

func TestFileProjectRegistryEditsTheSettings(t *testing.T) {
	t.Run("fills an empty projects key, rather than adding another", func(t *testing.T) {
		dir := t.TempDir()
		settings := filepath.Join(dir, "settings.yml")
		write(t, settings, "projects:\n")
		configs := infra.NewFileProjectRegistry(settings)
		root := filepath.Join(dir, "shop")
		if err := configs.Save(domain.NewProject(mustName(t, "shop"), mustRoot(t, root)), ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, settings); strings.Count(got, "projects:") != 1 {
			t.Fatalf("got settings:\n%s\nwant one projects key", got)
		}
		if _, err := infra.LoadSettings(dir, io.Discard); err != nil {
			t.Fatalf("the settings no longer load: %v", err)
		}
	})

	t.Run("renames a project its entry names in the entry", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "weaver.yml"), "name: theirs\n")
		configs, dir := listing(t, "  - root: "+root+"\n    name: mine\n")
		if err := configs.Save(domain.NewProject(mustName(t, "ours"), mustRoot(t, root)), ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if refs, err := configs.Registered(); err != nil || len(refs) != 1 || refs[0].Name().String() != "ours" {
			t.Fatalf("got %q (%v), want ours", described(refs), err)
		}
		if got := read(t, filepath.Join(root, "weaver.yml")); !strings.Contains(got, "name: theirs\n") {
			t.Fatalf("got %q, want the config's name kept", got)
		}
		if got := read(t, filepath.Join(dir, "settings.yml")); !strings.Contains(got, "name: ours") {
			t.Fatalf("got settings:\n%s\nwant name: ours", got)
		}
	})

	t.Run("carries the config's name to the config a named project is pointed at", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "weaver.yml"), "name: theirs\n")
		configs, _ := listing(t, "  - root: "+root+"\n    name: mine\n")
		if err := configs.Save(domain.NewProject(mustName(t, "mine"), mustRoot(t, root)), "tmux/dev.yml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := read(t, filepath.Join(root, "tmux", "dev.yml")); !strings.Contains(got, "name: theirs\n") {
			t.Fatalf("got %q, want the config's name carried over", got)
		}
	})

	t.Run("keeps the settings file's permissions", func(t *testing.T) {
		configs, dir := listing(t, "  - /srv/old\n")
		settings := filepath.Join(dir, "settings.yml")
		if err := os.Chmod(settings, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := configs.Delete(mustRoot(t, "/srv/old")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info, err := os.Stat(settings); err != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("got %v (%v), want 0644 kept", info.Mode().Perm(), err)
		}
	})
}

func TestFileProjectRegistryDelete(t *testing.T) {
	t.Run("unlists the root, keeping its config", func(t *testing.T) {
		configs, root := registeredShop(t, "{}\n")
		if err := configs.Delete(root); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if refs, err := configs.Registered(); err != nil || len(refs) != 0 {
			t.Fatalf("got %q (%v), want nothing registered", described(refs), err)
		}
		if _, err := os.Stat(filepath.Join(root.String(), "weaver.yml")); err != nil {
			t.Fatalf("got the config gone: %v", err)
		}
	})

	t.Run("reports a root that isn't registered", func(t *testing.T) {
		configs, _ := registeredShop(t, "{}\n")
		if err := configs.Delete(mustRoot(t, "/srv/blog")); !errors.Is(err, domain.ErrProjectNotFound) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})
}

func mustName(t *testing.T, raw string) domain.ProjectName {
	t.Helper()
	name, err := domain.NewProjectName(raw)
	if err != nil {
		t.Fatalf("NewProjectName(%q): %v", raw, err)
	}
	return name
}

// lines is the commands' text, in order.
func lines(cmds []domain.Command) []string {
	out := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		out = append(out, cmd.String())
	}
	return out
}
