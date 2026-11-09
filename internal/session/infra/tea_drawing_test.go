package infra

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/domain"
)

// firstWindow is the first window of a project rooted at /srv/shop with the
// given panes, written as in a config.
func firstWindow(t *testing.T, panes string) domain.WindowPlan {
	t.Helper()
	name, err := domain.NewProjectName("shop")
	if err != nil {
		t.Fatal(err)
	}
	doc := "root: /srv/shop\nwindows:\n  - name: main\n    panes:\n" + panes
	project, err := decodeProject(name, []byte(doc))
	if err != nil {
		t.Fatalf("decoding %q: %v", doc, err)
	}
	return project.Plan().Windows()[0]
}

func TestLayOut(t *testing.T) {
	t.Run("fills the window with its first pane", func(t *testing.T) {
		got := layOut(firstWindow(t, "      - name: only\n"), 81, 20)
		if want := []box{{0, 0, 81, 20}}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("splits in half by default, past a border", func(t *testing.T) {
		got := layOut(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
`), 81, 20)
		if want := []box{{0, 0, 40, 20}, {41, 0, 40, 20}}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("takes a percentage of the split pane, rounded down", func(t *testing.T) {
		got := layOut(firstWindow(t, `      - name: top
      - name: bottom
        split: { direction: below, size: 33% }
`), 81, 20)
		if want := []box{{0, 0, 81, 13}, {0, 14, 81, 6}}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("takes a number of cells of the split pane", func(t *testing.T) {
		got := layOut(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right, size: 20 }
`), 81, 20)
		if want := []box{{0, 0, 60, 20}, {61, 0, 20, 20}}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("cuts only the pane it splits", func(t *testing.T) {
		got := layOut(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
      - name: under left
        split: { of: left, direction: below }
`), 81, 20)
		if want := []box{{0, 0, 40, 10}, {41, 0, 40, 20}, {0, 11, 40, 9}}; !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestDrawing(t *testing.T) {
	t.Run("shares borders between panes, joining them", func(t *testing.T) {
		got := drawing(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
      - name: below
        split: { of: right, direction: below }
`), "/srv/shop", nil, -1, false, 81, 20, 21, 9)
		want := []string{
			"┌─ left ──┬─ right ─┐",
			"│ $ █     │ $ █     │",
			"│         │         │",
			"│         │         │",
			"│         ├─ below ─┤",
			"│         │ $ █     │",
			"│         │         │",
			"│         │         │",
			"└─────────┴─────────┘",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("puts the selected pane's name in brackets", func(t *testing.T) {
		got := drawing(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
`), "/srv/shop", nil, 1, false, 81, 20, 31, 4)[0]
		if want := "┌─ left ───────┬─[ right ]────┐"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("leaves out the brackets of a selected name with no room for them", func(t *testing.T) {
		got := drawing(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
`), "/srv/shop", nil, 1, false, 81, 20, 15, 4)[0]
		if want := "┌─ l… ─┬─ r… ─┐"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("draws the selected pane heavy, without brackets, when heavy", func(t *testing.T) {
		got := drawing(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
      - name: below
        split: { of: right, direction: below }
`), "/srv/shop", nil, 2, true, 81, 20, 21, 9)
		want := []string{
			"┌─ left ──┬─ right ─┐",
			"│ $ █     │ $ █     │",
			"│         │         │",
			"│         │         │",
			"│         ┢━ below ━┪",
			"│         ┃ $ █     ┃",
			"│         ┃         ┃",
			"│         ┃         ┃",
			"└─────────┺━━━━━━━━━┛",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("cuts a name that doesn't fit in its border", func(t *testing.T) {
		got := drawing(firstWindow(t, `      - name: a long name
      - name: b
        split: { direction: below }
`), "/srv/shop", nil, -1, false, 81, 20, 12, 5)[0]
		if want := "┌─ a lon… ─┐"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("cuts a prompt that doesn't fit in its box", func(t *testing.T) {
		got := drawing(firstWindow(t, "      - command: npm run dev\n"), "/srv/shop", nil, -1, false, 81, 20, 12, 3)[1]
		if want := "│ $ npm r… │"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("names no lone pane, as tmux draws no border around it", func(t *testing.T) {
		got := drawing(firstWindow(t, "      - name: only\n"), "/srv/shop", nil, -1, false, 81, 20, 12, 3)[0]
		if want := "┌──────────┐"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps a combining mark with its letter", func(t *testing.T) {
		got := drawing(firstWindow(t, "      - note: [\"cafe\u0301\"]\n"), "/srv/shop", nil, -1, false, 81, 20, 12, 4)[1]
		if want := "│ cafe\u0301     │"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestDrawingAlerts(t *testing.T) {
	sideBySide := `      - name: left
      - name: right
        split: { direction: right }
`

	t.Run("puts ⚠ in the top border of a pane with an alert", func(t *testing.T) {
		got := ansi.Strip(drawing(firstWindow(t, sideBySide), "/srv/shop", []alert{noAlert, programAlert}, -1, false, 81, 20, 21, 4)[0])
		if want := "┌─ left ──┬─ ⚠ ri… ─┐"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	stacked := `      - name: top
      - name: bottom
        split: { direction: below }
`

	t.Run("colours the border of a pane with an alert", func(t *testing.T) {
		row := drawing(firstWindow(t, sideBySide), "/srv/shop", []alert{noAlert, dirAlert}, -1, false, 81, 20, 21, 4)[1]
		if want := problemStyle.Render("│"); !strings.HasSuffix(row, want) {
			t.Fatalf("got %q, want it to end with %q", row, want)
		}
	})

	t.Run("leaves plain the border of a pane without one", func(t *testing.T) {
		row := drawing(firstWindow(t, sideBySide), "/srv/shop", []alert{noAlert, dirAlert}, -1, false, 81, 20, 21, 4)[1]
		if !strings.HasPrefix(row, "│ $ █") {
			t.Fatalf("got %q, want it to start plain", row)
		}
	})

	t.Run("leaves plain what a pane with an alert shows inside", func(t *testing.T) {
		row := drawing(firstWindow(t, sideBySide), "/srv/shop", []alert{noAlert, dirAlert}, -1, false, 81, 20, 21, 4)[1]
		if !strings.Contains(row, "│ $ █     "+problemStyle.Render("│")) {
			t.Fatalf("got %q, want the right pane's prompt plain", row)
		}
	})

	t.Run("leaves plain a neighbour's name on a shared border", func(t *testing.T) {
		row := drawing(firstWindow(t, stacked), "/srv/shop", []alert{dirAlert, noAlert}, -1, false, 81, 20, 21, 7)[3]
		if want := problemStyle.Render("─") + " bottom "; !strings.Contains(row, want) {
			t.Fatalf("got %q, want the border red and the name bottom plain: %q", row, want)
		}
	})

	t.Run("colours a shared border with the worse alert", func(t *testing.T) {
		row := drawing(firstWindow(t, sideBySide), "/srv/shop", []alert{dirAlert, programAlert}, -1, false, 81, 20, 21, 4)[1]
		if want := " $ █     " + problemStyle.Render("│") + " $ █"; !strings.Contains(row, want) {
			t.Fatalf("got %q, want the shared border red: %q", row, want)
		}
	})

	t.Run("marks a lone pane with an alert, though it has no name shown", func(t *testing.T) {
		got := ansi.Strip(drawing(firstWindow(t, "      - name: only\n"), "/srv/shop", []alert{programAlert}, -1, false, 81, 20, 12, 3)[0])
		if want := "┌─ ⚠ ──────┐"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestUndrawnPanes(t *testing.T) {
	t.Run("says how many panes don't fit", func(t *testing.T) {
		got := undrawnPanes(firstWindow(t, `      - name: a
      - name: b
        split: { direction: below }
      - name: c
        split: { direction: below }
`), "/srv/shop", nil, -1, 40, 2)
		if want := []string{"a · $ █", "… 2 more"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("puts the selected pane's name in brackets", func(t *testing.T) {
		got := undrawnPanes(firstWindow(t, `      - name: a
      - name: b
        split: { direction: below }
`), "/srv/shop", nil, 1, 40, 2)
		if want := []string{"a · $ █", "[ b ] · $ █"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestPaneLines(t *testing.T) {
	t.Run("shows a dir under the root relative to it", func(t *testing.T) {
		pane := firstWindow(t, "      - dir: api\n").Panes()[0]
		if got, want := paneLines(pane, "/srv/shop"), []string{"api/", "$ █"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("shows the note before the prompt", func(t *testing.T) {
		pane := firstWindow(t, "      - note: [hello]\n").Panes()[0]
		if got, want := paneLines(pane, "/srv/shop"), []string{"hello", "$ █"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("shows a command as run", func(t *testing.T) {
		pane := firstWindow(t, "      - command: task dev\n").Panes()[0]
		if got, want := paneLines(pane, "/srv/shop"), []string{"$ task dev"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("shows a prefill typed, not run", func(t *testing.T) {
		pane := firstWindow(t, "      - prefill: nimbus emulator\n").Panes()[0]
		if got, want := paneLines(pane, "/srv/shop"), []string{"$ nimbus emulator█"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("shows a dir outside the root in full", func(t *testing.T) {
		pane := firstWindow(t, "      - dir: /srv/shared\n").Panes()[0]
		if got, want := paneLines(pane, "/srv/shop"), []string{"/srv/shared/", "$ █"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestNeighbour(t *testing.T) {
	// left | right, right over below, each 10 rows high.
	boxes := layOut(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
      - name: below
        split: { of: right, direction: below }
`), 81, 21)

	t.Run("finds the pane across the border", func(t *testing.T) {
		if got, ok := neighbour(boxes, 1, 0, 1); !ok || got != 2 {
			t.Fatalf("got %d, %v, want 2, true", got, ok)
		}
	})

	t.Run("finds none at the window's edge", func(t *testing.T) {
		if got, ok := neighbour(boxes, 1, 1, 0); ok {
			t.Fatalf("got %d, want none", got)
		}
	})

	t.Run("picks the first of those sharing as much of the side", func(t *testing.T) {
		if got, ok := neighbour(boxes, 0, 1, 0); !ok || got != 1 {
			t.Fatalf("got %d, %v, want 1, true", got, ok)
		}
	})

	t.Run("picks the one sharing the most of the side", func(t *testing.T) {
		boxes := layOut(firstWindow(t, `      - name: left
      - name: right
        split: { direction: right }
      - name: below
        split: { of: right, direction: below, size: 80% }
`), 81, 20)
		if got, ok := neighbour(boxes, 0, 1, 0); !ok || got != 2 {
			t.Fatalf("got %d, %v, want 2, true", got, ok)
		}
	})
}

func TestBorderRune(t *testing.T) {
	sides := []int{joinsUp, joinsDown, joinsLeft, joinsRight}
	for joins := 1; joins < 16; joins++ {
		for heavy := 0; heavy < 16; heavy++ {
			if heavy&^joins != 0 {
				continue // only a side joined can be heavy
			}
			if got := borderRune(joins, heavy); got == "" {
				var weights [4]int
				for i, side := range sides {
					if joins&side != 0 {
						weights[i] = 1 + min(heavy&side, 1)
					}
				}
				t.Errorf("no character joins up, down, left, right as %v", weights)
			}
		}
	}
}
