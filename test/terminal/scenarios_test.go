//go:build terminal

package terminal

import (
	"regexp"
	"strings"
	"testing"

	tuitest "github.com/microsoft/tui-test/bindings/go"
)

// Scenario 1.
func TestBrowserDraws(t *testing.T) {
	h := start(t)

	h.browse("")

	for _, want := range []string{"─ projects ", "─ tasks ", "─ blog.yml ", "  notes", "  shop", "q quit"} {
		h.sees(want)
	}
	h.laidOut()
	if colour := h.colourOf("╭─ projects"); colour != "6" {
		h.fatal("the project list, holding the cursor, is drawn in %q, not cyan (6)", colour)
	}
}

func TestClickPicksProject(t *testing.T) {
	h := start(t)
	h.browse("")

	h.clicks("notes")

	h.sees("─ notes.yml ")
}

func TestClickPicksPane(t *testing.T) {
	h := start(t)
	h.browse("blog")

	// The pane list, under the drawing, which names shell too.
	if err := h.at("shell").Last().Click(tuitest.LocatorClickOptions{}); err != nil {
		h.fatal("clicking shell in the pane list: %v", err)
	}

	h.sees("[ shell ]")
}

// Scenario 2.
func TestOpenGrid(t *testing.T) {
	h := start(t)
	h.browse("blog")

	h.keys("Enter")

	h.eventually("the client switches to blog", func() bool { return h.clientOn() == "blog" })
	if active := h.tmux("display", "-p", "-t", "=blog:", "#{@label}"); active != "preview" {
		h.fatal("the active pane is %q, not preview", active)
	}
	// 160x44, less a column of borders and the pane names' top borders:
	// 60/40 of the columns, 70/30 of the rows.
	sizes := strings.ReplaceAll(h.tmux("list-panes", "-t", "=blog:write", "-F", "#{@label}=#{pane_width}x#{pane_height}"), "\n", " ")
	if want := "editor=95x43 preview=64x29 shell=64x13"; sizes != want {
		h.fatal("the grid is %s, not %s", sizes, want)
	}
}

// Scenario 12.
func TestKillFromInsideTearsDownFirst(t *testing.T) {
	h := start(t)
	h.started("shop")
	h.switchTo("shop", "shell")
	h.browse("shop")

	h.keys("x")
	h.sees("Kill")
	h.keys("y")

	h.sees("tearing down")
	h.sees("teardown: false: command failed")
	h.sees("shop's session will be killed anyway, with this pane.")
	h.sees("Press Enter to go on.")
	h.keys("Enter")
	h.eventually("shop ends", func() bool { return !h.running("shop") })
	if !h.running("term") {
		t.Fatal("term ended with shop")
	}
}

// Scenario 14.
func TestMiniaturesNeedLines(t *testing.T) {
	h := start(t)
	h.browse("notes")
	h.sees("┌──┬─ edit")

	if err := h.screen.Resize(cols, 20); err != nil {
		t.Fatal(err)
	}

	// Both on one screen: a resize draws a few half-made ones first.
	names := regexp.MustCompile(` code +shell `)
	h.eventually("the window names alone, at 20 lines", func() bool {
		screen, err := h.screen.Text(tuitest.TextOptions{})
		return err == nil && !strings.Contains(screen, "┌──┬─ edit") && names.MatchString(screen)
	})
	h.laidOut()
}
