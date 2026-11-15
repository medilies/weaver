//go:build terminal

package terminal

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tuitest "github.com/microsoft/tui-test/bindings/go"
)

// TestBrowserDraws: `weave`.
//
// The project list, task list, details (`blog.yml`) and key line drawn.
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

// TestClickPicksProject: `weave`, click `notes`.
//
// The details are `notes.yml`'s.
func TestClickPicksProject(t *testing.T) {
	h := start(t)
	h.browse("")

	h.clicks("notes")

	h.sees("─ notes.yml ")
}

// TestMouseSettingOff: settings `mouse: false`: `weave`, click `notes`,
// then `y`.
//
// The click selects nothing: `y` shows `blog.yml`, the first project's
// config.
func TestMouseSettingOff(t *testing.T) {
	h := start(t)
	h.settings("mouse: false")
	h.browse("")
	h.sees("─ blog.yml ")

	h.clicks("notes")
	h.keys("y")

	h.sees("root: ")
	h.sees("─ blog.yml ")
}

// TestEditorSetting: settings `editor: sed -i s/info/debug/`: `weave`,
// select `shop`, `E`.
//
// The settings' editor edits the config: `Saved shop.`, and `shop.yml`
// sets `LOG: debug`.
func TestEditorSetting(t *testing.T) {
	h := start(t)
	h.settings("editor: sed -i s/info/debug/")
	h.browse("shop")

	h.keys("E")

	h.sees("Saved shop.")
	if config := h.config("shop"); !strings.Contains(config, "LOG: debug") {
		h.fatal("shop.yml is %q, want LOG: debug", config)
	}
}

// TestBrowserKeySetting: settings `keys: {config: Y}`: `weave`, select
// `shop`, `y`, then `Y`.
//
// The keys line reads `Y config`; `y` does nothing, `Y` shows the config.
func TestBrowserKeySetting(t *testing.T) {
	h := start(t)
	h.settings("keys:", "  config: Y")
	h.browse("shop")
	h.sees("Y config")

	h.keys("y", "Y")

	h.sees("root: ")
	h.sees("Y drawing")
}

// TestPopupSetting: settings `popup: true`: `weave` in the pane `term`,
// select `notes`, enter.
//
// The browser shows in a popup, over `term`, which doesn't hold it; enter
// opens `notes` from there, the popup closing and the client switching.
func TestPopupSetting(t *testing.T) {
	h := start(t)
	h.settings("popup: true")
	term := h.here()
	h.browse("notes")

	if shown := h.tmux("capture-pane", "-p", "-t", term); strings.Contains(shown, "enter open") {
		h.fatal("the browser runs in the pane term, not a popup")
	}
	h.keys("Enter")

	h.eventually("the client moves to notes", func() bool { return h.clientOn() == "notes" })
	h.gone("enter open")
}

// TestClickPicksPane: `weave`, select `blog`, click `shell` in the pane
// list.
//
// `shell` is the selected pane in the drawing: `[ shell ]`.
func TestClickPicksPane(t *testing.T) {
	h := start(t)
	h.browse("blog")

	// The pane list, under the drawing, which names shell too.
	if err := h.at("shell").Last().Click(tuitest.LocatorClickOptions{}); err != nil {
		h.fatal("clicking shell in the pane list: %v", err)
	}

	h.sees("[ shell ]")
}

// TestOpenGrid: `weave`, select `blog`, Enter.
//
// The client switches to `blog`, `preview`, focused, is the active pane,
// and the grid is 60/40 and 70/30 of the window: `editor` 95x43, `preview`
// 64x29, `shell` 64x13.
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

// TestKillFromInsideTearsDownFirst: `shop` started. In its `shell` window:
// `weave`, select `shop`, `x`, `y`.
//
// `tearing down`, then `teardown: false: command failed`,
// `shop's session will be killed anyway, with this pane.` and
// `Press Enter to go on.` One Enter kills `shop`.
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

// TestKillAsksWithItsKeys: `notes` started: `weave`, select `notes`, `x`,
// then `n`.
//
// The status line asks `Kill notes's session and everything running in
// it? (y/n)`, the keys line below it saying `y or enter yes` and `any
// other key no`. `n` brings the browser's keys back, `notes` still
// running.
func TestKillAsksWithItsKeys(t *testing.T) {
	h := start(t)
	h.started("notes")
	h.browse("notes")

	h.keys("x")
	h.sees("Kill notes's session and everything running in it? (y/n)")
	h.sees("y or enter yes")
	h.sees("any other key no")
	h.keys("n")
	h.gone("any other key no")
	h.sees("q quit")
	if !h.running("notes") {
		t.Fatal("notes ended though the kill was declined")
	}
}

// TestMiniaturesNeedLines: `weave`, select `notes`, then the terminal down
// to 20 lines.
//
// Under the window list, `code` and `shell` are drawn small, each under
// its name, `code`'s with its narrow `files` column. At 20 lines, only the
// names show.
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

// TestMiniaturesSettingOff: settings `miniatures: false`: `weave`, select
// `notes`.
//
// The window list names `code` and `shell` without their miniatures.
func TestMiniaturesSettingOff(t *testing.T) {
	h := start(t)
	h.settings("miniatures: false")
	h.browse("notes")

	names := regexp.MustCompile(` code +shell `)
	h.screenShows("the window names alone", func(screen string) bool {
		return !strings.Contains(screen, "┌──┬─ edit") && names.MatchString(screen)
	})
}

// TestTaskRunsInItsPane: `shop` started: `weave`, select `shop`, `t`,
// Enter on `wait`.
//
// `sleep 300` runs in `logs` and the client moves there. A new `weave`
// shows `● wait`.
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

// TestEnterGoesToRunningTask: `shop` started: `weave`, select `shop`,
// `t`, `w` on `wait`, then Enter once `● wait` shows.
//
// The client moves to shop's throwaway window `wait`, where the task runs,
// and no second copy starts.
func TestEnterGoesToRunningTask(t *testing.T) {
	h := start(t)
	h.started("shop")
	h.logsIdle()
	h.browse("shop")

	h.keys("t", "w")
	h.sees("● wait")
	h.sees("enter go")
	h.keys("Enter")

	h.eventually("the client moves to shop", func() bool { return h.clientOn() == "shop" })
	if window := h.tmux("display", "-p", "-t", "=shop:", "#{window_name}"); window != "wait" {
		h.fatal("the current window is %q, not the throwaway window wait", window)
	}
	if windows := h.tmux("list-windows", "-t", "=shop", "-F", "#{window_name}"); strings.Count(windows, "wait") != 1 {
		h.fatal("shop's windows are %q, want one wait", windows)
	}
}

// TestInterruptTask: `shop` started: `weave`, select `shop`, `t`, `r` on
// `wait`, then `i` once `● wait` shows.
//
// `sleep 300` ends in `logs`, the ● goes, the status line says
// `Interrupted wait in api › logs.`, and the client stays in the browser.
func TestInterruptTask(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	h.browse("shop")

	h.keys("t", "r")
	h.sees("● wait")
	h.waitRuns(logs, "sleep", 5*time.Second)
	h.keys("i")

	h.sees("Interrupted wait in api › logs.")
	h.gone("● wait")
	if on := h.clientOn(); on != "term" {
		h.fatal("i left the browser for %s", on)
	}
}

// TestInterruptPane: `shop` started, `sleep 300` typed into `logs`:
// `weave`, select `shop`, →, `i`, then `i` again.
//
// `sleep 300` ends, the status line says `Interrupted api › logs.`, and
// the second `i` says there is nothing to interrupt.
func TestInterruptPane(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	h.tmux("send-keys", "-t", logs, "sleep 300", "Enter")
	h.waitRuns(logs, "sleep", 5*time.Second)
	h.browse("shop")
	h.keys("Right")
	h.sees("i interrupt")

	h.keys("i")
	h.sees("Interrupted api › logs.")
	h.promptIn(logs)
	h.keys("i")
	h.sees("nothing to interrupt")
}

// TestReload: `shop` started: `weave`, select `shop`, →, `R`, `p`, then
// `R`, `w`, then `R`, `x`.
//
// `R` asks `Reload pane logs, or the whole api window?`; `p` restarts
// `logs` in place, its process new, and says `Reloaded api › logs.`; `w`
// replaces `api` at its index and says `Reloaded api.`; `x` does nothing.
func TestReload(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	pid := h.tmux("display", "-p", "-t", logs, "#{pane_pid}")
	index := h.tmux("display", "-p", "-t", "=shop:api", "#{window_index}")
	h.browse("shop")
	h.keys("Right")
	h.sees("R reload")

	h.keys("R")
	h.sees("Reload pane logs, or the whole api window?")
	h.sees("p pane")
	h.keys("p")
	h.sees("Reloaded api › logs.")
	if now := h.tmux("display", "-p", "-t", logs, "#{pane_pid}"); now == pid {
		h.fatal("logs still runs process %s", pid)
	}

	h.keys("R", "w")
	h.sees("Reloaded api.")
	if now := h.tmux("display", "-p", "-t", "=shop:api", "#{window_index}"); now != index {
		h.fatal("api is at index %s, was %s", now, index)
	}

	h.keys("R", "x")
	h.sees("Nothing done.")
}

// TestBrowserStaysLive: `shop` started: `weave`, then start a `blog`
// session and kill `shop` from outside.
//
// `● blog` shows without a key pressed, and `● shop` goes.
func TestBrowserStaysLive(t *testing.T) {
	h := start(t)
	h.started("shop")
	h.browse("")
	h.sees("● shop")

	h.tmux("new-session", "-d", "-s", "blog")
	h.sees("● blog")
	h.tmux("kill-session", "-t", "=shop")
	h.gone("● shop")
}

// TestBrowserShowsProjectAddedOnDisk: `weave`, then a `zine.yml` written
// beside the configs.
//
// `zine` joins the list without a key pressed.
func TestBrowserShowsProjectAddedOnDisk(t *testing.T) {
	h := start(t)
	h.browse("")
	h.sees("  shop")

	h.write("cfg/weaver/projects/zine.yml", "root: "+h.dir+"/notes\nwindows:\n  - name: edit\n")

	h.sees("  zine")
}

// TestBrowserDropsProjectDeletedOnDisk: `weave`, select `notes`, then
// `notes.yml` removed.
//
// `notes` leaves the list, and the cursor moves to the next project:
// the details are `shop.yml`'s.
func TestBrowserDropsProjectDeletedOnDisk(t *testing.T) {
	h := start(t)
	h.browse("")
	h.clicks("notes")
	h.sees("─ notes.yml ")

	if err := os.Remove(filepath.Join(h.dir, "cfg/weaver/projects/notes.yml")); err != nil {
		t.Fatal(err)
	}

	h.sees("─ shop.yml ")
	h.gone("notes")
}

// TestPreviewSettingOff: settings `preview: false`, `notes` started:
// `weave`, select `notes`, then `p`.
//
// The drawing shows, `p preview` in the key line, and `p` brings the
// preview: `notes.yml · preview`.
func TestPreviewSettingOff(t *testing.T) {
	h := start(t)
	h.settings("preview: false")
	h.started("notes")
	h.browse("notes")

	h.sees("p preview")
	h.gone("· preview")
	h.keys("p")
	h.sees("notes.yml · preview")
}

// TestTaskOfBusyPaneRunsInThrowawayWindow: `shop` started, `logs` busy:
// `weave`, select `shop`, `t`, `echo hi`, Enter, `y`.
//
// The dialog says `logs is busy`. A throwaway window `echo` opens with
// `@weaver_throwaway` 1 and `@weaver_task_of` set to the ID of `logs`,
// printing `hi`, and the client moves there.
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

// TestRerunTypesExitedCommandAgain: `shop` started, its `sleep 3` exited:
// `weave`, select `shop`, →, `r`.
//
// `sleep 3` runs in `logs` again and the client moves there.
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

// TestPreviewShowsSessionAsItIs: `notes` started: `weave`, select `notes`.
// Print `green` in green in `code`'s first pane, split `code` from
// outside, Tab twice to the new pane, `p`, `p`, then kill `notes` from
// outside.
//
// The title reads `notes.yml · preview`. `green` shows, drawn in green,
// and the new pane's levels read `not in the config`. After `p`, the
// drawing has neither. `p` brings the preview back, and the kill the
// drawing with `preview: notes's session ended`.
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

// TestPreviewOfOwnPaneShowsNoBrowserInside: `weave notes`, then in
// `code`'s focused pane, zoomed: `weave`, select `notes`.
//
// The preview draws the pane weave runs in with `this browser` in it, not
// the browser again inside itself.
func TestPreviewOfOwnPaneShowsNoBrowserInside(t *testing.T) {
	h := start(t)
	h.promptIn("term")
	h.typed("clear; weave notes")
	h.eventually("the client moves to notes", func() bool { return h.clientOn() == "notes" })
	h.tmux("resize-pane", "-Z", "-t", "=notes:code")

	h.browse("notes")

	h.sees("this browser")
}

// TestPreviewOfZoomedWindowShowsZoomedPane: `notes` started: `weave`,
// select `notes`, zoom `code` on `edit` from outside, `p`, `p`, unzoom.
//
// Zoomed, the preview shows `edit` alone, named alone in the pane list,
// its levels below. After `p`, the drawing selects `edit` too. Unzoomed,
// both panes are back, `edit` still selected.
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

// TestOpenRunsSetupAndLandsOnFocus: `weave shop`.
//
// The setup command's output shows in `term`. The client lands in window
// `api`, pane `back`, both focused. `echo "$APP_ENV $LOG"` prints
// `staging info` in `logs` and `dev info` in `back`.
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

// TestExitedCommandWarns: `shop` started, its `sleep 3` exited: `weave`,
// select `shop`, →.
//
// `⚠` on `logs`. The levels show `APP_ENV=staging` as
// `window api, over project's local`. `r rerun` is in the key line on
// `logs`, and gone after Tab to `back`.
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

// TestExitedCommandWarnsWhileShown: `shop` started, its `sleep 3` exited,
// `sleep 300` typed in `logs` from outside: `weave`, select `shop`, →, then
// Ctrl-C in `logs` from outside, then `sleep 300` typed again.
//
// No `⚠` while `sleep 300` runs. After Ctrl-C, `⚠` on `logs` and
// `r rerun` in the key line, without leaving the browser. Once `sleep 300`
// runs again, both go.
func TestExitedCommandWarnsWhileShown(t *testing.T) {
	h := start(t)
	h.started("shop")
	logs := h.logsIdle()
	h.tmux("send-keys", "-t", logs, "sleep 300", "Enter")
	h.waitRuns(logs, "sleep", 5*time.Second)
	h.browse("shop")
	h.keys("Right")
	h.sees("logs   back")
	h.gone("⚠")

	h.tmux("send-keys", "-t", logs, "C-c")

	h.sees(`⚠ window "api", pane "logs": command exited: sleep 3`)
	h.sees("r rerun")

	h.tmux("send-keys", "-t", logs, "sleep 300", "Enter")

	h.gone("⚠")
	h.gone("r rerun")
}

// TestTaskRunStayingLeavesYouInBrowser: `shop` started: `weave`, select
// `shop`, `t`, `r` on `wait`, then Ctrl-C in `logs`.
//
// `sleep 300` runs in `logs`, `● wait` shows, and the client stays in the
// browser. After Ctrl-C the ● goes while the browser is shown, and `wait`
// stays listed.
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

// TestConfigShowsAsStored: `shop` started: `weave`, select `shop`, `y`,
// `y`.
//
// The YAML shows as stored, then the preview comes back.
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

// TestGridColumnInCells: `weave notes`.
//
// The client switches to `notes`, window `code`, of the client's size less
// the status line, 160x44. `files` is exactly 30 columns wide and `edit`
// takes the rest, 129.
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

// TestBadSettingsStopOnlyTheBrowser: `settings.yml` with
// `keys: { remove: u }`: `weave`, then `weave --version`,
// `weave completion bash` and `weave notes --detached`.
//
// `weave` prints
// `Error: …/settings.yml: keys.remove: key taken: "u" already does
// keys.undo in the config form`
// and exits 1 before the browser opens, leaving the config as it was; the
// others run as usual, and `notes` starts.
func TestBadSettingsStopOnlyTheBrowser(t *testing.T) {
	h := start(t)
	h.settings("keys: { remove: u }")
	before := h.config("notes")
	h.promptIn("term")

	h.typed("clear; weave; echo exit=$?")

	h.sees("Error: " + h.dir + `/cfg/weaver/settings.yml: keys.remove: key taken: "u" already does keys.undo in the config form`)
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

// TestPreviewAndDrawingShareSelectedPane: `notes` started: `weave`, select
// `notes`, Tab, `p`, Tab, `p`, `t`, ↓, ↑.
//
// The preview selects `files`, then `edit`; the drawing keeps `edit`, then
// selects `files`, and the preview keeps `files`. Moving between `ls` and
// `echo saved` in the task list selects `files` and `edit` in the preview.
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

// TestKillUnzoomsFirst: `notes` started, `code` zoomed on `edit`:
// `weave kill notes`.
//
// `notes` is gone and `term` still runs: weave unzooms first, as tmux 3.4
// crashes killing a session with a zoomed window, taking every session
// along.
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

// TestTaskOfBrowsersOwnPaneRunsApart: `notes` started, `code` zoomed on
// `files`: in `files`, `weave`, select `notes`, `t`, `r` on `ls`, `w`, ↓,
// `r` on `echo saved`.
//
// `r` on `ls` is refused in red:
// `✗ files runs this browser: w runs ls in a throwaway window`. `w` runs
// it in a throwaway window `ls`, `code` staying the current window.
// `echo saved` runs in `edit`, `files` staying the current pane.
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

// TestBrowserAsksAboutBusyPaneInPlace: `notes` started, `edit` busy:
// `weave`, select `notes`, `t`, ↓, `r`, `y`.
//
// The browser asks in its status line, staying on screen:
// `edit is busy: run echo saved in a throwaway window? (y/n)`. After `y`
// it says the task runs in a throwaway window, and the client stays in the
// browser.
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

// TestClickPicksWindow: `weave`, select `notes`, click `shell` in the
// window list.
//
// `shell` is the window drawn: `2/2`.
func TestClickPicksWindow(t *testing.T) {
	h := start(t)
	h.browse("notes")
	h.sees("1/2")

	h.clicks("shell")

	h.sees("2/2")
}

// TestClickRunsTaskStaying: `notes` started: `weave`, select `notes`,
// click `ls` in the task list.
//
// `ls` runs in `files`, `Running ls in code › files.` shows, and the
// client stays in the browser.
func TestClickRunsTaskStaying(t *testing.T) {
	h := start(t)
	h.started("notes")
	files := h.paneOf("notes:code", "files")
	h.browse("notes")

	h.clicks("ls")

	h.sees("Running ls in code › files.")
	h.paneShows(files, "$ ls")
	if on := h.clientOn(); on != "term" {
		h.fatal("the click left the browser for %s", on)
	}
}
