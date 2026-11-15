//go:build terminal

package terminal

import "testing"

// TestBrowserLooksAsBefore: `weave`, select `notes`.
//
// The screen is `testdata/snapshots/browser-notes.txt`: the project list,
// the task list, and the details with `code`'s drawing and its miniatures.
func TestBrowserLooksAsBefore(t *testing.T) {
	h := start(t)

	h.browse("notes")

	h.sees("┌──┬─ edit")
	h.matchesSnapshot("browser-notes")
}

// TestNarrowBrowserLooksAsBefore: the terminal at 80x24, `weave`, select
// `notes`.
//
// The screen is `testdata/snapshots/browser-notes-narrow.txt`.
func TestNarrowBrowserLooksAsBefore(t *testing.T) {
	h := start(t)
	if err := h.screen.Resize(80, 24); err != nil {
		t.Fatal(err)
	}

	h.browse("notes")

	h.matchesSnapshot("browser-notes-narrow")
}

// TestFormLooksAsBefore: edit `notes`.
//
// The screen is `testdata/snapshots/form-notes.txt`: the project's card,
// the window list, and `code`'s card with its grid and drawing.
func TestFormLooksAsBefore(t *testing.T) {
	h := start(t)

	h.edit("notes")

	h.matchesSnapshot("form-notes")
}
