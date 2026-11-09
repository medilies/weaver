package infra

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/domain"
)

// shop is a project rooted at /srv/shop, the rest of its config written
// after its root.
func shop(t *testing.T, config string) domain.Project {
	t.Helper()
	name, err := domain.NewProjectName("shop")
	if err != nil {
		t.Fatal(err)
	}
	doc := "root: /srv/shop\n" + config
	project, err := decodeProject(name, []byte(doc))
	if err != nil {
		t.Fatalf("decoding %q: %v", doc, err)
	}
	return project
}

// levelRows are the levels of the pane at index pane of project's window
// at index window, wide enough for any, each line's cells split on runs
// of two spaces or more.
func levelRows(project domain.Project, window, pane int) [][]string {
	var rows [][]string
	for _, line := range levels(project, window, pane, 200) {
		var cells []string
		for cell := range strings.SplitSeq(ansi.Strip(line), "  ") {
			if cell = strings.TrimSpace(cell); cell != "" {
				cells = append(cells, cell)
			}
		}
		rows = append(rows, cells)
	}
	return rows
}

func TestLevels(t *testing.T) {
	t.Run("puts a pane with no dir of its own or its window's in the project's root", func(t *testing.T) {
		got := levelRows(shop(t, "windows:\n  - name: main\n"), 0, 0)
		if want := [][]string{{"/srv/shop", "project"}}; !slices.EqualFunc(got, want, slices.Equal) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("takes a pane's dir from its window", func(t *testing.T) {
		got := levelRows(shop(t, "windows:\n  - name: main\n    dir: api\n"), 0, 0)
		if want := []string{"/srv/shop/api", "window main"}; !slices.Equal(got[0], want) {
			t.Fatalf("got %q, want %q", got[0], want)
		}
	})

	t.Run("says what a pane's own dir is over", func(t *testing.T) {
		got := levelRows(shop(t, `windows:
  - name: main
    dir: api
    panes:
      - name: back
      - name: front
        dir: front
        split: { direction: right }
`), 0, 1)
		if want := []string{"front", "/srv/shop/front", "pane, over window's api"}; !slices.Equal(got[0], want) {
			t.Fatalf("got %q, want %q", got[0], want)
		}
	})

	t.Run("says a pane's dir . is over its window's", func(t *testing.T) {
		got := levelRows(shop(t, `windows:
  - name: main
    dir: api
    panes:
      - name: back
      - name: front
        dir: .
        split: { direction: right }
`), 0, 1)
		if want := []string{"front", "/srv/shop", "pane, over window's api"}; !slices.Equal(got[0], want) {
			t.Fatalf("got %q, want %q", got[0], want)
		}
	})

	t.Run("takes a pane's own dir from the pane in a window at the root", func(t *testing.T) {
		got := levelRows(shop(t, "windows:\n  - name: main\n    panes:\n      - dir: api\n"), 0, 0)
		if want := []string{"/srv/shop/api", "pane"}; !slices.Equal(got[0], want) {
			t.Fatalf("got %q, want %q", got[0], want)
		}
	})

	t.Run("follows a grid's panes in the order they are created", func(t *testing.T) {
		got := levelRows(shop(t, `windows:
  - name: main
    grid:
      areas: [[left, right]]
    panes:
      - name: right
        dir: front
      - name: left
`), 0, 1)
		if want := []string{"right", "/srv/shop/front", "pane"}; !slices.Equal(got[0], want) {
			t.Fatalf("got %q, want %q", got[0], want)
		}
	})

	t.Run("lists before commands in the order they run, by level", func(t *testing.T) {
		got := levelRows(shop(t, `before: [source ~/.aliases]
windows:
  - name: main
    before: [git fetch]
    panes:
      - before: [nvm use 20]
`), 0, 0)
		want := [][]string{
			{"before", "source ~/.aliases", "project"},
			{"git fetch", "window main"},
			{"nvm use 20", "pane"},
		}
		if !slices.EqualFunc(got[1:], want, slices.Equal) {
			t.Fatalf("got %q, want %q", got[1:], want)
		}
	})

	t.Run("says which project variable a pane's overrides", func(t *testing.T) {
		got := levelRows(shop(t, `env: { APP_ENV: local, LOG: info }
windows:
  - name: main
    panes:
      - env: { APP_ENV: dev }
`), 0, 0)
		want := [][]string{
			{"env", "APP_ENV=dev", "pane, over project's local"},
			{"LOG=info", "project"},
		}
		if !slices.EqualFunc(got[1:], want, slices.Equal) {
			t.Fatalf("got %q, want %q", got[1:], want)
		}
	})

	t.Run("says which window variable a pane's overrides, and which project one the window's", func(t *testing.T) {
		got := levelRows(shop(t, `env: { APP_ENV: local, LOG: info }
windows:
  - name: main
    env: { APP_ENV: staging, LOG: debug }
    panes:
      - env: { APP_ENV: dev }
`), 0, 0)
		want := [][]string{
			{"env", "APP_ENV=dev", "pane, over window main's staging"},
			{"LOG=debug", "window main, over project's info"},
		}
		if !slices.EqualFunc(got[1:], want, slices.Equal) {
			t.Fatalf("got %q, want %q", got[1:], want)
		}
	})

	t.Run("takes a variable only a pane sets from the pane", func(t *testing.T) {
		got := levelRows(shop(t, "windows:\n  - name: main\n    panes:\n      - env: { DEBUG: \"1\" }\n"), 0, 0)
		if want := []string{"env", "DEBUG=1", "pane"}; !slices.Equal(got[1], want) {
			t.Fatalf("got %q, want %q", got[1], want)
		}
	})

	t.Run("quotes an empty value", func(t *testing.T) {
		got := levelRows(shop(t, `env: { APP_ENV: "" }
windows:
  - name: main
    panes:
      - env: { APP_ENV: dev }
`), 0, 0)
		if want := []string{"env", "APP_ENV=dev", `pane, over project's ""`}; !slices.Equal(got[1], want) {
			t.Fatalf("got %q, want %q", got[1], want)
		}
	})

	t.Run("quotes a value ending with a space, which wouldn't show", func(t *testing.T) {
		got := levelRows(shop(t, "env: { PS1: \"$ \" }\nwindows:\n  - name: main\n"), 0, 0)
		if want := []string{"env", `PS1="$ "`, "project"}; !slices.Equal(got[1], want) {
			t.Fatalf("got %q, want %q", got[1], want)
		}
	})

	t.Run("quotes a value holding control characters, keeping it on its line", func(t *testing.T) {
		got := levelRows(shop(t, "env: { MOTD: \"hi\\nthere\" }\nwindows:\n  - name: main\n"), 0, 0)
		if want := []string{"env", `MOTD="hi\nthere"`, "project"}; !slices.Equal(got[1], want) {
			t.Fatalf("got %q, want %q", got[1], want)
		}
	})

	t.Run("cuts a long value to keep its level in view", func(t *testing.T) {
		project := shop(t, "windows:\n  - name: main\n    panes:\n      - before: [\""+strings.Repeat("x", 60)+"\"]\n")
		got := ansi.Strip(levels(project, 0, 0, 40)[1])
		if !strings.HasSuffix(got, "  pane") || ansi.StringWidth(got) > 40 {
			t.Fatalf("got %q, want it within 40 cells and ending with its level", got)
		}
	})

	t.Run("shows a pane's clearing with the level setting it, over a higher one", func(t *testing.T) {
		project := shop(t, "clear: true\nwindows:\n  - name: main\n    panes:\n      - before: [nvm use 20]\n        clear: false\n")
		got := levelRows(project, 0, 0)
		if want := []string{"clear", "no", "pane, over project's yes"}; !slices.Equal(got[len(got)-1], want) {
			t.Fatalf("got %q, want the last row %q", got, want)
		}
	})

	t.Run("shows no clearing for a pane without before commands", func(t *testing.T) {
		got := levelRows(shop(t, "clear: true\nwindows:\n  - name: main\n"), 0, 0)
		for _, row := range got {
			if row[0] == "clear" {
				t.Fatalf("got %q, want no clear row", got)
			}
		}
	})
}
