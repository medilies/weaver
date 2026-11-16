//go:build terminal

package terminal

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestBindingStartsFromASession: `blog` started; in a new directory,
// `journal`: `weave bind w/blog`, Ctrl-S, `y`.
//
// The form names the project `journal`, after the directory. Saving asks
// `Save the new project's config?`. `journal/weaver.yml` has no window but
// `write`, and `focus: true` on `preview`; the settings list `journal`
// first; `w/blog`, which weave started for `blog`, keeps its name.
func TestBindingStartsFromASession(t *testing.T) {
	h := start(t)
	h.started("blog")
	h.mkdir("journal")
	h.promptIn("term")
	h.typed("clear; cd " + filepath.Join(h.dir, "journal") + " && weave bind w/blog")
	h.sees("↑↓ move")
	h.sees("journal")

	h.keys("Ctrl+s")
	h.sees("Save the new project's config?")
	h.keys("y")

	h.eventually("journal's weaver.yml is written", func() bool {
		_, err := os.Stat(filepath.Join(h.dir, "journal", "weaver.yml"))
		return err == nil
	})
	config := h.config("journal")
	if windows := regexp.MustCompile(`(?m)^  - name: (\w+)`).FindAllStringSubmatch(config, -1); len(windows) != 1 || windows[0][1] != "write" {
		t.Fatalf("journal's windows aren't write alone:\n%s", config)
	}
	if focused := regexp.MustCompile(`- name: (\w+)\s+focus: true`).FindStringSubmatch(config); focused == nil || focused[1] != "preview" {
		t.Fatalf("the focused pane isn't preview:\n%s", config)
	}
	settings, err := os.ReadFile(filepath.Join(h.dir, "cfg/weaver/settings.yml"))
	if journal, blog := strings.Index(string(settings), "/journal"), strings.Index(string(settings), "/blog"); err != nil || journal < 0 || journal > blog {
		t.Fatalf("journal isn't listed first:\n%s", settings)
	}
	if !h.running("w/blog") {
		t.Fatal("w/blog was renamed")
	}
}

// TestSettingsChangeColourAndDrawingKeys: `settings.yml` with
// `theme: { selected: 214 }` and
// `keys: { split-right: "|", split-below: "-" }`: edit `notes`, `shell` in
// the window list, down to its drawing, `|`, `u`, Esc.
//
// `settings.schema.json` is beside it. The key line reads
// `| split right  - split below`, `|` splits the pane right
// (`pane 1   pane 2`), and something is drawn in colour 214. `u` then Esc
// leave with `Nothing done.`
func TestSettingsChangeColourAndDrawingKeys(t *testing.T) {
	h := start(t)
	h.settings("theme: { selected: 214 }", `keys: { split-right: "|", split-below: "-" }`)
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

// TestClickAddsWindow: edit `notes`, click `+ window`, Enter, `logs`,
// Enter.
//
// The windows card offers `enter names a new window`, and the window list
// then reads `code   shell   logs   + window`.
func TestClickAddsWindow(t *testing.T) {
	h := start(t)
	h.edit("notes")

	h.clicks("+ window")

	h.sees("enter names a new window")
	h.keys("Enter")
	h.types("logs")
	h.keys("Enter")
	h.seesLoosely("code shell logs + window")
}
