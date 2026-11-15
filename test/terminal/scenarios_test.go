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

// Scenario 3.
func TestOpenRunsSetupAndLandsOnFocus(t *testing.T) {
	h := start(t)
	h.promptIn("term")

	h.typed("clear; weave shop")

	h.eventually("the client moves to shop", func() bool { return h.clientOn() == "shop" })
	if landed := h.tmux("display", "-p", "-t", "=shop:", "#{window_name} #{@label}"); landed != "api back" {
		h.fatal("the client lands in %q, not api back", landed)
	}
	logs, back := h.logsIdle(), h.paneOf("shop:api", "back")
	for _, pane := range []string{logs, back} {
		h.tmux("send-keys", "-t", pane, `echo "env=$APP_ENV $LOG"`, "Enter")
	}
	h.paneShows(logs, "env=staging info")
	h.paneShows(back, "env=dev info")
	h.paneShows("=term:", "setting up shop")
}

// Scenario 4.
func TestExitedCommandWarns(t *testing.T) {
	h := start(t)
	h.started("shop")
	h.logsIdle()
	h.browse("shop")

	h.keys("Right")

	h.sees(`⚠ window "api", pane "logs": command exited: sleep 3`)
	h.seesLoosely("APP_ENV=staging window api, over project's local")
	h.sees("r rerun")
	h.keys("Tab")
	h.gone("r rerun")
}

// Scenario 7.
func TestTaskRunStayingLeavesYouInBrowser(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	h.browse("shop")

	h.keys("t", "r")

	h.sees("● wait")
	h.waitRuns(logs, "sleep", 5*time.Second)
	if on := h.clientOn(); on != "term" {
		h.fatal("r left the browser for %s", on)
	}
	h.tmux("send-keys", "-t", logs, "C-c")
	h.gone("● wait")
	h.sees("  wait")
}

// Scenario 8.
func TestConfigShowsAsStored(t *testing.T) {
	h := start(t)
	h.started("shop")
	h.browse("shop")

	h.keys("y")

	h.sees("setup: [echo setting up shop]")
	h.keys("y")
	h.sees("shop.yml · preview")
	h.gone("setup: [echo setting up shop]")
}

// Scenario 13.
func TestGridColumnInCells(t *testing.T) {
	h := start(t)
	h.promptIn("term")

	h.typed("clear; weave notes")

	h.eventually("the client moves to notes", func() bool { return h.clientOn() == "notes" })
	if window := h.tmux("display", "-p", "-t", "=notes:", "#{window_name}"); window != "code" {
		h.fatal("the window is %q, not code", window)
	}
	if size := h.tmux("display", "-p", "-t", "=notes:code", "#{window_width}x#{window_height}"); size != "160x44" {
		h.fatal("the window is %s, not 160x44", size)
	}
	widths := strings.ReplaceAll(h.tmux("list-panes", "-t", "=notes:code", "-F", "#{@label}=#{pane_width}"), "\n", " ")
	if widths != "files=30 edit=129" {
		h.fatal("the widths are %s, not files=30 edit=129", widths)
	}
}

// Scenario 16.
func TestBadSettingsStopOnlyTheBrowser(t *testing.T) {
	h := start(t)
	h.settings("keys: { remove: u }")
	before := h.config("notes")
	h.promptIn("term")

	h.typed("clear; weave; echo exit=$?")

	h.sees("Error: " + h.dir + `/cfg/weaver/settings.yml: keys.remove: key taken: "u" already does undo`)
	h.paneHasLine("=term:", "exit=1")
	if n, _ := h.at("q quit").Count(); n > 0 {
		h.fatal("the browser opened")
	}
	if after := h.config("notes"); after != before {
		t.Fatalf("the config changed:\n%s", after)
	}
	h.typed("weave --version >/dev/null && weave completion bash >/dev/null && echo both-ran")
	h.paneHasLine("=term:", "both-ran")
	h.started("notes")
}

// Scenario 22.
func TestPreviewAndDrawingShareSelectedPane(t *testing.T) {
	h := start(t)
	h.started("notes")
	h.browse("notes")
	h.sees("notes.yml · preview")
	h.sees("[ files ]")

	h.keys("Tab")
	h.sees("[ edit ]")
	h.keys("p")
	h.gone("· preview")
	h.sees("[ edit ]")
	h.keys("Tab")
	h.sees("[ files ]")
	h.keys("p")
	h.sees("· preview")
	h.sees("[ files ]")

	h.keys("t")
	h.sees("enter run, go")
	h.keys("Down")
	h.sees("[ edit ]")
	h.keys("Up")
	h.sees("[ files ]")
}

// Scenario 23.
func TestKillUnzoomsFirst(t *testing.T) {
	h := start(t)
	h.started("notes")
	h.zoom("notes:code", "edit")

	h.typed("clear; weave kill notes")

	h.eventually("notes ends", func() bool { return !h.running("notes") })
	if !h.running("term") {
		t.Fatal("term went with notes: the server crashed")
	}
}

// Scenario 25.
func TestTaskOfBrowsersOwnPaneRunsApart(t *testing.T) {
	h := start(t)
	h.started("notes")
	h.switchTo("notes", "code")
	h.zoom("notes:code", "files")
	h.browse("notes")
	h.keys("t")
	h.sees("enter run, go")

	h.keys("r")
	h.sees("✗ files runs this browser: w runs ls in a throwaway window")
	h.keys("w")

	h.sees("Running ls in a throwaway window.")
	h.eventually("a window ls opens", func() bool {
		_, err := h.tmuxErr("display", "-p", "-t", "=notes:ls", "")
		return err == nil
	})
	if window := h.tmux("display", "-p", "-t", "=notes:", "#{window_name}"); window != "code" {
		h.fatal("the current window is %q, not code", window)
	}
	h.keys("Down", "r")
	h.sees("Running echo saved in code › edit.")
	if pane := h.activePane("notes"); pane != "files" {
		h.fatal("the current pane is %q, not files", pane)
	}
}

// Scenario 26.
func TestBrowserAsksAboutBusyPaneInPlace(t *testing.T) {
	h := start(t)
	h.started("notes")
	edit := h.paneOf("notes:code", "edit")
	h.tmux("send-keys", "-t", edit, "sleep 300", "Enter")
	h.waitRuns(edit, "sleep", 5*time.Second)
	h.browse("notes")

	h.keys("t", "Down", "r")

	h.sees("edit is busy: run echo saved in a throwaway window? (y/n)")
	h.sees("notes.yml")
	h.keys("y")
	h.sees("Running echo saved in a throwaway window.")
	if on := h.clientOn(); on != "term" {
		h.fatal("r left the browser for %s", on)
	}
}
