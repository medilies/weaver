//go:build terminal

package terminal

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tuitest "github.com/microsoft/tui-test/bindings/go"
)

// TestFocusingWindowUnfocusesTheOther: Edit `shop`, the `shell` window's
// `focus` row, Enter, Ctrl-S, `y`.
//
// The diff removes one `focus: true`, `api`'s, and keeps the pane's.
// `Saved shop.` The file has a single `focus: true`, on `back`.
func TestFocusingWindowUnfocusesTheOther(t *testing.T) {
	h := start(t)
	h.edit("shop")
	h.cursorTo("focus")

	h.keys("Enter", "Ctrl+s")

	h.sees("Save these changes to the config?")
	removed, kept := regexp.MustCompile(`(?m)^ - +focus: true`), regexp.MustCompile(`(?m)^   +focus: true`)
	h.screenShows("the diff removes api's focus and keeps back's", func(screen string) bool {
		return len(removed.FindAllString(screen, -1)) == 1 && len(kept.FindAllString(screen, -1)) == 1
	})
	h.keys("y")
	h.sees("Saved shop.")
	config := h.config("shop")
	if n := strings.Count(config, "focus: true"); n != 1 {
		t.Fatalf("the config has %d focus: true, not 1:\n%s", n, config)
	}
	if focused := regexp.MustCompile(`- name: (\w+)\s+focus: true`).FindStringSubmatch(config); focused == nil || focused[1] != "back" {
		t.Fatalf("the focused pane isn't back:\n%s", config)
	}
}

// TestBindingUnderRegisteredNameAsksToReplace: `blog` started:
// `weave bind blog`, Ctrl-S, `n`, Ctrl-U, `journal`, Enter, Ctrl-S, `y`.
//
// Saving under `blog` asks `Replace blog's registered config with this?`
// under the diff, once; `n` goes back to the form typing the name, under
// it `blog is already registered`. Saving as `journal` asks
// `Save the new project's config?`. `journal.yml` has no window but
// `write`, and `focus: true` on `preview`.
func TestBindingUnderRegisteredNameAsksToReplace(t *testing.T) {
	h := start(t)
	h.started("blog")
	h.promptIn("term")
	h.typed("clear; weave bind blog")
	h.sees("↑↓ move")

	h.keys("Ctrl+s")

	h.sees("Replace blog's registered config with this?")
	if n, _ := h.screen.GetByText("Replace blog's", tuitest.TextSelectorOptions{}).Count(); n != 1 {
		h.fatal("the question is asked %d times, not once", n)
	}
	h.keys("n")
	h.sees("blog is already registered")
	h.keys("Ctrl+u")
	h.types("journal")
	h.keys("Enter", "Ctrl+s")
	h.sees("Save the new project's config?")
	h.keys("y")
	h.eventually("journal.yml is written", func() bool {
		_, err := os.Stat(filepath.Join(h.dir, "cfg/weaver/projects/journal.yml"))
		return err == nil
	})
	config := h.config("journal")
	if windows := regexp.MustCompile(`(?m)^  - name: (\w+)`).FindAllStringSubmatch(config, -1); len(windows) != 1 || windows[0][1] != "write" {
		t.Fatalf("journal's windows aren't write alone:\n%s", config)
	}
	if focused := regexp.MustCompile(`- name: (\w+)\s+focus: true`).FindStringSubmatch(config); focused == nil || focused[1] != "preview" {
		t.Fatalf("the focused pane isn't preview:\n%s", config)
	}
}

// TestSettingsChangeColourAndDrawingKeys: `settings.yml` with
// `selected-colour: 214` and
// `keys: { split-right: "|", split-below: "-" }`: edit `notes`, `shell` in
// the window list, down to its drawing, `|`, `u`, Esc.
//
// `settings.schema.json` is beside it. The key line reads
// `| split right  - split below`, `|` splits the pane right
// (`pane 1   pane 2`), and something is drawn in colour 214. `u` then Esc
// leave with `Nothing done.`
func TestSettingsChangeColourAndDrawingKeys(t *testing.T) {
	h := start(t)
	h.settings("selected-colour: 214", `keys: { split-right: "|", split-below: "-" }`)
	h.edit("notes")
	if _, err := os.Stat(filepath.Join(h.dir, "cfg/weaver/settings.schema.json")); err != nil {
		t.Fatal("no settings.schema.json beside settings.yml")
	}
	h.cursorTo("code +shell")
	h.keys("Right")
	h.sees("notes › window shell")
	for range 20 {
		if n, _ := h.at("┏").Count(); n > 0 {
			break
		}
		h.step("Down")
	}

	h.sees("| split right  - split below")
	h.keys("|")

	h.sees("pane 1   pane 2")
	if !h.drawnIn("214") {
		h.fatal("nothing is drawn in colour 214")
	}
	h.keys("u", "Escape")
	h.sees("Nothing done.")
}

// TestTypedAreasAddPane: Edit `notes`, `code`'s `areas` row, Enter,
// Ctrl-U, type `files, edit / files, logs`, Enter, Ctrl-S, `y`.
//
// The drawing adds `logs` as it is typed. The diff adds the row
// `[files, logs]` and a pane `logs`. The saved config keeps
// `columns: [30, 1fr]` and lists `files`, `edit`, `logs`.
func TestTypedAreasAddPane(t *testing.T) {
	h := start(t)
	h.edit("notes")
	h.cursorTo("areas")

	h.keys("Enter", "Ctrl+u")
	h.types("files, edit / files, logs")

	h.sees("─ logs ─")
	h.keys("Enter", "Ctrl+s")
	h.sees("Save these changes to the config?")
	h.sees("- [files, logs]")
	h.sees("- name: logs")
	h.keys("y")
	h.sees("Saved notes.")
	config := h.config("notes")
	if !strings.Contains(config, "columns: [30, 1fr]") {
		t.Fatalf("the columns changed:\n%s", config)
	}
	names := regexp.MustCompile(`name: (files|edit|logs)\b`).FindAllStringSubmatch(config, -1)
	if got := len(names); got != 3 || names[0][1] != "files" || names[1][1] != "edit" || names[2][1] != "logs" {
		t.Fatalf("the panes aren't files, edit, logs:\n%s", config)
	}
}

// TestGridTurnedIntoSplitsKeepsItsSizes: Edit `notes`, `code`'s `layout`
// row, →, Ctrl-S, `y`.
//
// The `areas`, `columns` and `rows` rows go, and the drawing is unchanged:
// `edit` starts at the same column. The diff removes the grid. `edit`'s
// saved split is its share of the terminal, about 81% (129 of 160
// columns), not of tmux's default 80 (61%).
func TestGridTurnedIntoSplitsKeepsItsSizes(t *testing.T) {
	h := start(t)
	h.edit("notes")
	h.cursorTo("layout")
	before := h.splitAt()
	if before == 0 {
		h.fatal("no two prompts side by side in the drawing")
	}

	h.keys("Right")

	h.gone(" areas ")
	if after := h.splitAt(); after != before {
		h.fatal("edit starts at column %d, not %d", after, before)
	}
	h.keys("Ctrl+s")
	h.sees("Save these changes to the config?")
	h.sees("grid:")
	h.keys("y")
	h.sees("Saved notes.")
	config := h.config("notes")
	// edit is 129 of term's 160 columns; tmux's default 80 would make it
	// 49 of 80, 61%.
	size := regexp.MustCompile(`name: edit[^-]*?size: (\d+)%`).FindStringSubmatch(config)
	if size == nil {
		t.Fatalf("edit has no split size:\n%s", config)
	}
	if percent, _ := strconv.Atoi(size[1]); percent < 78 || percent > 84 {
		t.Fatalf("edit's split is %d%%, not about 81%%", percent)
	}
}

// splitAt is the column the drawing's second pane starts at, its prompt
// on the same line as the first's; 0 when no line has two prompts.
func (h *harness) splitAt() int {
	screen, _ := h.screen.Text(tuitest.TextOptions{})
	for _, line := range strings.Split(screen, "\n") {
		if strings.Count(line, "$ █") >= 2 {
			return len([]rune(line[:strings.LastIndex(line, "$ █")]))
		}
	}
	return 0
}

// TestFormScrollsBackToItsTop: `weave`, select `notes`, `e`, ↓ until the
// project card scrolls out of view, ↑ back to `root`.
//
// The project card shows whole again, its border and the `name` row above
// `root` included, though the cursor never stops on `name`.
func TestFormScrollsBackToItsTop(t *testing.T) {
	h := start(t)
	h.edit("notes")

	for range 60 {
		if n, _ := h.at("╭─ project").Count(); n == 0 {
			break
		}
		h.step("Down")
	}
	h.gone("╭─ project")
	root := regexp.MustCompile(`› +root`)
	for range 60 {
		if screen, _ := h.screen.Text(tuitest.TextOptions{}); root.MatchString(screen) {
			break
		}
		h.step("Up")
	}

	h.sees("╭─ project")
	h.sees("   name      notes")
}

// TestBrowserKeepsFilterAfterForm: `weave`, `/notes`, enter, `e`, esc.
//
// Back from the config form, the browser keeps the filter `/notes`, `shop`
// filtered out.
func TestBrowserKeepsFilterAfterForm(t *testing.T) {
	h := start(t)
	h.edit("notes")

	h.keys("Escape")

	h.sees("q quit")
	h.sees("/notes")
	h.gone("  shop")
}
