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
// before commands in the order they run, whether they are cleared once
// they ran when a level sets it, then its env, each with the
// level it comes from, and what it is over when it replaces a higher
// level's.
func levels(project domain.Project, window, pane, width int) []string {
	w := project.Windows()[window]
	p := w.Panes()[pane]
	plan := project.Plan().Windows()[window].Panes()[pane]

	lines := []levelLine{{label: plan.Name().String(), value: contractHome(plan.Dir()), level: dirLevel(w, p)}}
	before := higherBefore(project, w)
	for _, cmd := range p.Before() {
		before = append(before, levelLine{value: cmd.String(), level: "pane"})
	}
	for i, line := range before {
		if i == 0 {
			line.label = "before"
		}
		lines = append(lines, line)
	}
	if len(before) > 0 {
		if line, ok := clearingLine(project, w, p); ok {
			lines = append(lines, line)
		}
	}
	paneVars := map[string]bool{}
	for _, v := range p.Env().Vars() {
		paneVars[v.Name()] = true
	}
	label := "env"
	for _, v := range plan.Env() {
		level := "project"
		if paneVars[v.Name()] {
			level = "pane"
			if over := overProject(project, v.Name()); over != "" {
				level += ", " + over
			}
		}
		lines = append(lines, levelLine{label: label, value: v.Name() + "=" + shownValue(v.Value()), level: level})
		label = ""
	}
	return levelTable(lines, width)
}

// higherBefore are the before commands a pane of window runs ahead of
// its own, from the levels above it: its project's, then window's, each
// with its level.
func higherBefore(project domain.Project, window domain.Window) []levelLine {
	var lines []levelLine
	for _, cmd := range project.Before() {
		lines = append(lines, levelLine{value: cmd.String(), level: "project"})
	}
	for _, cmd := range window.Before() {
		lines = append(lines, levelLine{value: cmd.String(), level: "window " + window.Name().String()})
	}
	return lines
}

// clearingLine is whether a pane p of window w clears its before
// commands, with the level setting it and what it is over, unless no
// level sets it.
func clearingLine(project domain.Project, w domain.Window, p domain.Pane) (levelLine, bool) {
	var set []levelLine
	for _, l := range []struct {
		clearing domain.Clearing
		level    string
	}{{p.Clearing(), "pane"}, {w.Clearing(), "window " + w.Name().String()}, {project.Clearing(), "project"}} {
		if l.clearing.IsSet() {
			set = append(set, levelLine{label: "clear", value: clearingText(l.clearing), level: l.level})
		}
	}
	if len(set) == 0 {
		return levelLine{}, false
	}
	line := set[0]
	if len(set) > 1 {
		line.level += ", over " + set[1].level + "'s " + set[1].value
	}
	return line, true
}

// clearingText is a set clearing as shown: yes or no.
func clearingText(c domain.Clearing) string {
	if c == domain.ClearingOn {
		return "yes"
	}
	return "no"
}

// overProject is what a pane's variable named name is over: the
// project's value, or nothing when the project doesn't set it.
func overProject(project domain.Project, name string) string {
	for _, v := range project.Env().Vars() {
		if v.Name() == name {
			return "over project's " + shownValue(v.Value())
		}
	}
	return ""
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
// empty, starts or ends with spaces, which wouldn't show, or holds
// control characters, which would break the lines.
func shownValue(value string) string {
	if value == "" || value != strings.TrimSpace(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
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
