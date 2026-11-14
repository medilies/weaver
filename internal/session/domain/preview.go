package domain

import "slices"

// WindowPreview is a window of a running session as it is now: its size
// in terminal cells and its panes where tmux lays them out.
type WindowPreview struct {
	width, height int
	panes         []PanePreview
}

// NewWindowPreview is a window of width×height cells holding panes, in
// the session's order.
func NewWindowPreview(width, height int, panes ...PanePreview) WindowPreview {
	return WindowPreview{width: width, height: height, panes: slices.Clone(panes)}
}

// Size is the window's width and height in cells.
func (w WindowPreview) Size() (width, height int) {
	return w.width, w.height
}

// Panes are the window's panes, in the session's order.
func (w WindowPreview) Panes() []PanePreview {
	return slices.Clone(w.panes)
}

// PanePreview is a pane of a running session as it is now: where it is in
// its window, the name the session gives it, the lines it shows, and the
// pane of the config it is, if any.
type PanePreview struct {
	x, y, width, height int
	name                string
	lines               []string
	configured          int // -1 for a pane the config doesn't have
	browsing            bool
}

// NewPanePreview is a pane of width×height cells whose top left cell is
// (x, y) in its window, named name in the session, none for a pane
// without, showing lines, top to bottom, which may hold the terminal's
// escape sequences. It is no pane of the config until Configured says so.
func NewPanePreview(x, y, width, height int, name string, lines []string) PanePreview {
	return PanePreview{x: x, y: y, width: width, height: height, name: name, lines: slices.Clone(lines), configured: -1}
}

// Configured is p as the pane at index of its window in the config.
func (p PanePreview) Configured(index int) PanePreview {
	p.configured = index
	return p
}

// Browsing is p as the pane weaver's browser runs in, which shows no
// lines: they would show the preview showing them.
func (p PanePreview) Browsing() PanePreview {
	p.browsing, p.lines = true, nil
	return p
}

// ShowsTheBrowser reports whether weaver's browser runs in the pane.
func (p PanePreview) ShowsTheBrowser() bool {
	return p.browsing
}

// Place is where the pane is in its window, in cells, its borders left
// out.
func (p PanePreview) Place() (x, y, width, height int) {
	return p.x, p.y, p.width, p.height
}

// Name is the name the session gives the pane, empty for none.
func (p PanePreview) Name() string {
	return p.name
}

// Lines are what the pane shows, top to bottom.
func (p PanePreview) Lines() []string {
	return slices.Clone(p.lines)
}

// InConfig is the index of the config's pane p is in its window, or false
// for a pane the config doesn't have, such as one split by hand.
func (p PanePreview) InConfig() (int, bool) {
	return p.configured, p.configured >= 0
}
