package infra

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// theme is how weaver's screens are styled: the selected colour, the
// shades and the state colours. The selected colour marks what is
// selected, and where the cursor is; the shades rank the rest, bright for
// what the cursor is on or a heading, plain for what is read, muted for
// what is told about it; the state colours only ever tell a state, and
// are the terminal's own basic colours unless the settings say otherwise.
type theme struct {
	colour color.Color
	// picked is text in the selected colour.
	picked lipgloss.Style
	// bright and muted are the shades above and below plain text.
	bright, muted lipgloss.Style
	// ok, warning and failure are the state colours: green for what runs
	// or is fine, yellow for what would likely fail or has exited, red for
	// what fails.
	ok, warning, failure lipgloss.Style
}

// palette is what the settings choose of the theme: its colours, and its
// shades.
type palette struct {
	selected, ok, warning, failure color.Color
	bright, muted                  shade
}

// shade is a shade's look: a colour, if set, and bold or faint.
type shade struct {
	colour      color.Color
	bold, faint bool
}

// defaultPalette is the theme without settings: cyan, the terminal's own
// green, yellow and red, bold for bright and faint for muted.
func defaultPalette() palette {
	return palette{
		selected: lipgloss.Color("6"),
		ok:       lipgloss.Color("2"),
		warning:  lipgloss.Color("3"),
		failure:  lipgloss.Color("1"),
		bright:   shade{bold: true},
		muted:    shade{faint: true},
	}
}

func (p palette) theme() theme {
	return theme{
		colour:  p.selected,
		picked:  lipgloss.NewStyle().Foreground(p.selected),
		bright:  p.bright.style(),
		muted:   p.muted.style(),
		ok:      lipgloss.NewStyle().Foreground(p.ok),
		warning: lipgloss.NewStyle().Foreground(p.warning),
		failure: lipgloss.NewStyle().Foreground(p.failure),
	}
}

func (s shade) style() lipgloss.Style {
	style := lipgloss.NewStyle().Bold(s.bold).Faint(s.faint)
	if s.colour != nil {
		style = style.Foreground(s.colour)
	}
	return style
}

func (s Settings) theme() theme {
	return s.palette.theme()
}

// runningMark is the ● before a project, or a task, still running.
func (t theme) runningMark() string {
	return t.ok.Render("●")
}

// edge is how a card's border and title show.
type edge struct {
	border, title lipgloss.Style
}

// edge is a card's: in the selected colour with a bright title for the
// innermost card holding the cursor, as focused says it is, else muted.
func (t theme) edge(focused bool) edge {
	if focused {
		return edge{border: t.picked, title: t.bright}
	}
	return edge{border: t.muted, title: t.muted}
}

// gutter is the cells before a row or a list's line: the cursor's mark
// on the one it is on, as at says, in the selected colour while it holds
// the cursor, as focused says.
func (t theme) gutter(at, focused bool) string {
	switch {
	case !at:
		return "  "
	case focused:
		return t.picked.Render("›") + " "
	}
	return "› "
}

// chosen is label picked out of a row of them: bright and underlined, in
// the selected colour while the cursor is on the row, as focused says.
func (t theme) chosen(label string, focused bool) string {
	style := t.bright.Underline(true)
	if focused {
		style = style.Foreground(t.colour)
	}
	return style.Render(label)
}

// keyTold is a key a keys line tells, and what it does.
type keyTold struct {
	key, does string
}

// keysGap is the space between two keys told.
const keysGap = "  "

// keys is told side by side, each key in the selected colour and what it
// does muted, with the cells each covers.
func (t theme) keys(told []keyTold) (string, []hotspot) {
	var line string
	var spots []hotspot
	for i, k := range told {
		if i > 0 {
			line += keysGap
		}
		from := ansi.StringWidth(line)
		text := t.picked.Render(k.key)
		if k.does != "" {
			text += " " + t.muted.Render(k.does)
		}
		line += text
		spots = append(spots, hotspot{from: from, to: ansi.StringWidth(line), key: k.key})
	}
	return line, spots
}

// alertGlyph is the mark of a pane's alert, so that it reads without its
// colour: ✗ for a missing directory, which opening fails on, ⚠ for the
// others, and none for no alert.
func alertGlyph(a alert) string {
	switch a {
	case noAlert, selectedPane:
		return ""
	case dirAlert:
		return "✗"
	}
	return "⚠"
}

// alertStyle is the colour of a pane's alert, if it has one.
func (t theme) alertStyle(a alert) (lipgloss.Style, bool) {
	switch a {
	case dirAlert:
		return t.failure, true
	case exitedAlert, programAlert:
		return t.warning, true
	}
	return lipgloss.Style{}, false
}
