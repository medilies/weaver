package infra

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/domain"
)

// levelLine is one thing a pane gets: under label, its value, and the
// level it comes from.
type levelLine struct {
	label, value, level string
}

// minLevelValueWidth is the fewest cells a value keeps when the level
// beside it would not fit otherwise.
const minLevelValueWidth = 12

// levels is what the pane at index pane of project's window at index
// window gets, one line each, cut to width: its dir, under its name, its
// before commands in the order they run, then its env, each with the
// level it comes from, and what it is over when it replaces a higher
// level's.
func levels(project domain.Project, window, pane, width int) []string {
	w := project.Windows()[window]
	p := w.Panes()[pane]
	plan := project.Plan().Windows()[window].Panes()[pane]
	windowLevel := "window " + w.Name().String()

	lines := []levelLine{{label: plan.Name().String(), value: contractHome(plan.Dir()), level: dirLevel(w, p)}}
	label := "before"
	for _, from := range []struct {
		cmds  []domain.Command
		level string
	}{{project.Before(), "project"}, {w.Before(), windowLevel}, {p.Before(), "pane"}} {
		for _, cmd := range from.cmds {
			lines = append(lines, levelLine{label: label, value: cmd.String(), level: from.level})
			label = ""
		}
	}
	projectVars := map[string]string{}
	for _, v := range project.Env().Vars() {
		projectVars[v.Name()] = v.Value()
	}
	paneVars := map[string]bool{}
	for _, v := range p.Env().Vars() {
		paneVars[v.Name()] = true
	}
	label = "env"
	for _, v := range plan.Env() {
		level := "project"
		if paneVars[v.Name()] {
			level = "pane"
			if over, ok := projectVars[v.Name()]; ok {
				level += ", over project's " + shownValue(over)
			}
		}
		lines = append(lines, levelLine{label: label, value: v.Name() + "=" + shownValue(v.Value()), level: level})
		label = ""
	}
	return levelTable(lines, width)
}

// dirLevel is the level the dir p starts in comes from: p's own, over its
// window's when that isn't the root, else w's, else the project's root.
func dirLevel(w domain.Window, p domain.Pane) string {
	_, own := p.Dir()
	switch {
	case own && !w.Dir().IsRoot():
		return "pane, over window's " + contractHome(w.Dir().String())
	case own:
		return "pane"
	case !w.Dir().IsRoot():
		return "window " + w.Name().String()
	}
	return "project"
}

// shownValue is a variable's value as written, or quoted when it is
// empty or holds control characters, which would break the lines.
func shownValue(value string) string {
	if value == "" || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return strconv.Quote(value)
	}
	return value
}

// levelTable lines up lines in columns, the levels dimmed, values cut
// short so the levels fit in width while they keep minLevelValueWidth
// cells, and every line cut to width.
func levelTable(lines []levelLine, width int) []string {
	labelWidth, valueWidth, levelWidth := 0, 0, 0
	for _, line := range lines {
		labelWidth = max(labelWidth, ansi.StringWidth(line.label))
		valueWidth = max(valueWidth, ansi.StringWidth(line.value))
		levelWidth = max(levelWidth, ansi.StringWidth(line.level))
	}
	gap := "  "
	if labelWidth > 0 {
		labelWidth += len(gap)
	}
	valueWidth = min(valueWidth, max(width-labelWidth-len(gap)-levelWidth, minLevelValueWidth))
	table := make([]string, 0, len(lines))
	for _, line := range lines {
		value := ansi.Truncate(line.value, valueWidth, "…")
		text := padded(line.label, labelWidth) + padded(value, valueWidth) + gap + keysStyle.Render(line.level)
		table = append(table, ansi.Truncate(text, width, "…"))
	}
	return table
}

// padded is text with spaces after it to fill width cells.
func padded(text string, width int) string {
	return text + strings.Repeat(" ", max(width-ansi.StringWidth(text), 0))
}
