//go:build terminal

package terminal

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

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

// Scenario 5.
func TestTaskRunsInItsPane(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	h.browse("shop")

	h.keys("t", "Enter")

	h.eventually("the client moves to shop", func() bool { return h.clientOn() == "shop" })
	if active := h.activePane("shop"); active != "logs" {
		h.fatal("the active pane is %q, not logs", active)
	}
	h.waitRuns(logs, "sleep", 5*time.Second)
	h.switchTo("term", "0")
	h.browse("shop")
	h.sees("● wait")
}

// Scenario 6.
func TestTaskOfBusyPaneRunsInThrowawayWindow(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	h.tmux("send-keys", "-t", logs, "sleep 300", "Enter")
	h.waitRuns(logs, "sleep", 5*time.Second)
	h.browse("shop")

	h.keys("t", "Down", "Enter")
	h.sees("logs is busy")
	h.keys("y")

	h.eventually("the client moves to shop", func() bool { return h.clientOn() == "shop" })
	h.eventually("a window echo opens", func() bool {
		_, err := h.tmuxErr("display", "-p", "-t", "=shop:echo", "")
		return err == nil
	})
	if options := h.tmux("display", "-p", "-t", "=shop:echo", "#{@weaver_throwaway} #{@weaver_task_of}"); options != "1 "+logs {
		h.fatal("echo's options are %q, not %q", options, "1 "+logs)
	}
	h.eventually("echo prints hi", func() bool {
		out, _ := h.tmuxErr("capture-pane", "-p", "-t", "=shop:echo")
		return slices.Contains(strings.Split(out, "\n"), "hi")
	})
}

// Scenario 9.
func TestRerunTypesExitedCommandAgain(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	h.browse("shop")
	h.keys("Right")
	h.sees("r rerun")

	h.keys("r")

	h.eventually("the client moves to shop", func() bool { return h.clientOn() == "shop" })
	if active := h.activePane("shop"); active != "logs" {
		h.fatal("the active pane is %q, not logs", active)
	}
	h.waitRuns(logs, "sleep", 5*time.Second)
}

// Scenario 19.
func TestPreviewShowsSessionAsItIs(t *testing.T) {
	h := start(t)
	h.started("notes")
	h.browse("notes")
	h.sees("notes.yml · preview")

	h.tmux("send-keys", "-t", "=notes:code.0", `printf '\033[32mgreen\033[0m\n'`, "Enter")
	h.tmux("split-window", "-d", "-t", "=notes:code")

	h.sees("green")
	if colour := h.colourOf("green"); colour != "2" && colour != "green" {
		h.fatal("green is drawn in %q, not green", colour)
	}
	h.keys("Tab", "Tab")
	h.sees("not in the config")
	h.keys("p")
	h.gone("· preview")
	h.gone("not in the config")
	h.keys("p")
	h.sees("· preview")
	h.tmux("kill-session", "-t", "=notes")
	h.sees("preview: notes's session ended")
}

// Scenario 20.
func TestPreviewOfOwnPaneShowsNoBrowserInside(t *testing.T) {
	h := start(t)
	h.promptIn("term")
	h.typed("clear; weave notes")
	h.eventually("the client moves to notes", func() bool { return h.clientOn() == "notes" })
	h.tmux("resize-pane", "-Z", "-t", "=notes:code")

	h.browse("notes")

	h.sees("this browser")
}

// Scenario 21.
func TestPreviewOfZoomedWindowShowsZoomedPane(t *testing.T) {
	h := start(t)
	h.started("notes")
	h.browse("notes")
	h.sees("notes.yml · preview")

	h.tmux("resize-pane", "-Z", "-t", h.paneOf("notes:code", "edit"))

	h.gone("[ files ]")
	h.sees("edit  " + h.dir + "/notes")
	alone := regexp.MustCompile(`(?m)│  edit +│$`)
	h.screenShows("the preview shows the zoomed pane alone", alone.MatchString)
	h.keys("p")
	h.gone("· preview")
	h.sees("[ edit ]")
	h.keys("p")
	h.sees("· preview")
	h.tmux("resize-pane", "-Z", "-t", "=notes:code")
	both := regexp.MustCompile(`(?m)│  files +edit +│$`)
	h.screenShows("the preview of both panes, edit selected", func(screen string) bool {
		return strings.Contains(screen, "notes.yml · preview") && strings.Contains(screen, "[ edit ]") && both.MatchString(screen)
	})
}
