package infra

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/domain"
)

// drawnOrder is the indexes of window's panes in the order a drawing of
// a window of cols×rows cells places them: top to bottom, then left to
// right.
func drawnOrder(window domain.WindowPlan, cols, rows int) []int {
	boxes := layOut(window, cols, rows)
	order := make([]int, len(boxes))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(boxes[a].y, boxes[b].y), cmp.Compare(boxes[a].x, boxes[b].x))
	})
	return order
}

// steppedIn is the item by places after from in order, before it when by
// is negative, going round.
func steppedIn(order []int, from, by int) int {
	n := len(order)
	return order[((slices.Index(order, from)+by)%n+n)%n]
}

// box is the cells a pane covers in its window, its borders left out:
// tmux draws one cell of border between panes.
type box struct {
	x, y, width, height int
}

// layOut places window's panes in a window of cols×rows cells as tmux
// splits them: the first pane fills the window, and each later one cuts
// the pane it splits, taking its size of it, half by default, past one
// cell of border. A grid sizing a column or row in cells is sized to
// cols×rows first, as tmux makes it.
func layOut(window domain.WindowPlan, cols, rows int) []box {
	panes := window.SizedTo(cols, rows).Panes()
	boxes := make([]box, 0, len(panes))
	for i, pane := range panes {
		split, splits := pane.Split()
		if i == 0 || !splits {
			boxes = append(boxes, box{width: cols, height: rows})
			continue
		}
		cut := boxes[split.Of()]
		if split.Direction() == domain.Right {
			taken := splitSize(split.Size(), cut.width)
			boxes[split.Of()].width = max(cut.width-taken-1, 0)
			boxes = append(boxes, box{x: cut.x + cut.width - taken, y: cut.y, width: taken, height: cut.height})
		} else {
			taken := splitSize(split.Size(), cut.height)
			boxes[split.Of()].height = max(cut.height-taken-1, 0)
			boxes = append(boxes, box{x: cut.x, y: cut.y + cut.height - taken, width: cut.width, height: taken})
		}
	}
	return boxes
}

// splitSize is how many of total cells a new pane of size takes, as tmux
// rounds it: a percentage rounded down, half by default, and always
// leaving the split pane and the border a cell each.
func splitSize(size domain.Size, total int) int {
	text := size.String()
	amount, percent := strings.CutSuffix(text, "%")
	n, _ := strconv.Atoi(amount)
	switch {
	case text == "":
		n = (total+1)/2 - 1
	case percent:
		n = total * n / 100
	}
	return max(min(max(n, 1), total-2), 0)
}

// The sides a border cell joins, which pick its character.
const (
	joinsUp = 1 << iota
	joinsDown
	joinsLeft
	joinsRight
)

// borderRunes are the characters joining a cell's sides, by the weight
// of each side, up, down, left and right: 0 none, 1 light, 2 heavy.
var borderRunes = map[[4]int]string{
	{0, 0, 1, 1}: "─", {0, 0, 2, 2}: "━", {0, 0, 1, 2}: "╼", {0, 0, 2, 1}: "╾",
	{1, 1, 0, 0}: "│", {2, 2, 0, 0}: "┃", {1, 2, 0, 0}: "╽", {2, 1, 0, 0}: "╿",
	{0, 0, 1, 0}: "─", {0, 0, 0, 1}: "─", {1, 0, 0, 0}: "│", {0, 1, 0, 0}: "│",
	{0, 0, 2, 0}: "━", {0, 0, 0, 2}: "━", {2, 0, 0, 0}: "┃", {0, 2, 0, 0}: "┃",

	{0, 1, 0, 1}: "┌", {0, 1, 0, 2}: "┍", {0, 2, 0, 1}: "┎", {0, 2, 0, 2}: "┏",
	{0, 1, 1, 0}: "┐", {0, 1, 2, 0}: "┑", {0, 2, 1, 0}: "┒", {0, 2, 2, 0}: "┓",
	{1, 0, 0, 1}: "└", {1, 0, 0, 2}: "┕", {2, 0, 0, 1}: "┖", {2, 0, 0, 2}: "┗",
	{1, 0, 1, 0}: "┘", {1, 0, 2, 0}: "┙", {2, 0, 1, 0}: "┚", {2, 0, 2, 0}: "┛",

	{1, 1, 0, 1}: "├", {1, 1, 0, 2}: "┝", {2, 1, 0, 1}: "┞", {1, 2, 0, 1}: "┟",
	{2, 2, 0, 1}: "┠", {2, 1, 0, 2}: "┡", {1, 2, 0, 2}: "┢", {2, 2, 0, 2}: "┣",
	{1, 1, 1, 0}: "┤", {1, 1, 2, 0}: "┥", {2, 1, 1, 0}: "┦", {1, 2, 1, 0}: "┧",
	{2, 2, 1, 0}: "┨", {2, 1, 2, 0}: "┩", {1, 2, 2, 0}: "┪", {2, 2, 2, 0}: "┫",
	{0, 1, 1, 1}: "┬", {0, 1, 2, 1}: "┭", {0, 1, 1, 2}: "┮", {0, 1, 2, 2}: "┯",
	{0, 2, 1, 1}: "┰", {0, 2, 2, 1}: "┱", {0, 2, 1, 2}: "┲", {0, 2, 2, 2}: "┳",
	{1, 0, 1, 1}: "┴", {1, 0, 2, 1}: "┵", {1, 0, 1, 2}: "┶", {1, 0, 2, 2}: "┷",
	{2, 0, 1, 1}: "┸", {2, 0, 2, 1}: "┹", {2, 0, 1, 2}: "┺", {2, 0, 2, 2}: "┻",

	{1, 1, 1, 1}: "┼", {1, 1, 2, 1}: "┽", {1, 1, 1, 2}: "┾", {1, 1, 2, 2}: "┿",
	{2, 1, 1, 1}: "╀", {1, 2, 1, 1}: "╁", {2, 2, 1, 1}: "╂", {2, 1, 2, 1}: "╃",
	{2, 1, 1, 2}: "╄", {1, 2, 2, 1}: "╅", {1, 2, 1, 2}: "╆", {2, 1, 2, 2}: "╇",
	{1, 2, 2, 2}: "╈", {2, 2, 2, 1}: "╉", {2, 2, 1, 2}: "╊", {2, 2, 2, 2}: "╋",
}

// borderRune is the character joining the sides joins, heavy those of
// them also in heavy.
func borderRune(joins, heavy int) string {
	var weights [4]int
	for i, side := range []int{joinsUp, joinsDown, joinsLeft, joinsRight} {
		switch {
		case joins&heavy&side != 0:
			weights[i] = 2
		case joins&side != 0:
			weights[i] = 1
		}
	}
	return borderRunes[weights]
}

// alert is the worst warning about a pane, which colours its box.
// selectedPane is no warning, but colours the selected pane's box the
// same way, any warning winning over it.
type alert int

const (
	noAlert alert = iota
	selectedPane
	exitedAlert
	programAlert
	dirAlert
)

var alertStyles = map[alert]lipgloss.Style{
	exitedAlert:  warningStyle,
	programAlert: warningStyle,
	dirAlert:     problemStyle,
}

// drawing is window's panes drawn as boxes in width×height cells, in the
// proportions they take in a window of cols×rows. Each box shows what
// its pane looks like the moment it opens: its name in the top border,
// its dir when it isn't root, its note, then its prompt. The selected
// pane, by its index, has a border and title in the selected colour,
// look's picked, heavy when heavy is
// set, else with its name in brackets. A pane with an alert, from alerts
// by its index, gets ⚠ in its top border and a border in the alert's
// colour, the worst winning on a border boxes share, and over the
// selected colour.
func drawing(look look, window domain.WindowPlan, root string, alerts []alert, selected int, heavy bool, cols, rows, width, height int) []string {
	grid := newGrid(width, height)
	// A line between cells, from -1 (the window's outer border) to cols or
	// rows (the other one), maps to the same drawn line for every box it
	// borders, so neighbours share their borders.
	column := func(line int) int { return scaled(line+1, cols+1, width-1) }
	row := func(line int) int { return scaled(line+1, rows+1, height-1) }

	type frame struct{ left, top, right, bottom int }
	boxes := layOut(window, cols, rows)
	frames := make([]frame, len(boxes))
	for i, b := range boxes {
		f := frame{column(b.x - 1), row(b.y - 1), column(b.x + b.width), row(b.y + b.height)}
		frames[i] = f
		thick := heavy && i == selected
		grid.line(f.left, f.top, f.right, f.top, thick)
		grid.line(f.left, f.bottom, f.right, f.bottom, thick)
		grid.line(f.left, f.top, f.left, f.bottom, thick)
		grid.line(f.right, f.top, f.right, f.bottom, thick)
	}
	grid.drawBorders()

	panes := window.Panes()
	for i, pane := range panes {
		f := frames[i]
		title := paneTitle(pane, len(panes))
		if alertOf(alerts, i) != noAlert {
			title = strings.TrimSpace("⚠ " + title)
		}
		room := f.right - f.left - 3
		open, closing := " ", " "
		if i == selected && !heavy && len(panes) > 1 && room >= 5 { // else no room for the brackets
			open, closing = "[ ", " ]"
		}
		if width := room - len(open) - len(closing); title != "" && width > 0 {
			grid.write(f.left+2, f.top, open+ansi.Truncate(title, width, "…")+closing, room)
		}
		for j, text := range paneLines(pane, root) {
			if f.top+1+j >= f.bottom {
				break
			}
			grid.write(f.left+2, f.top+1+j, text, f.right-f.left-3)
		}
	}
	for i, f := range frames {
		a := alertOf(alerts, i)
		if i == selected {
			a = max(a, selectedPane)
		}
		// A box scaled too small to show is left alone: colouring it would
		// colour the boxes it collapsed into.
		if a != noAlert && f.right-f.left >= 2 && f.bottom-f.top >= 2 {
			grid.alert(f.left, f.top, f.right, f.bottom, a)
			grid.alertCell(f.left+2, f.top, a) // the title, if written
		}
	}
	return grid.rows(look.picked)
}

// alertOf is the alert of the pane at index, none past the end of alerts.
func alertOf(alerts []alert, index int) alert {
	if index < len(alerts) {
		return alerts[index]
	}
	return noAlert
}

// undrawnPanes is window's panes one a line, for a space too small to draw
// them in: "api shell · api/ · $ task dev", each cut to width cells, the
// selected one's name in brackets, and ⚠ first for a pane with an alert.
// When they take more than height lines, the last line says how many are
// left out.
func undrawnPanes(window domain.WindowPlan, root string, alerts []alert, selected, width, height int) []string {
	panes := window.Panes()
	lines := make([]string, 0, len(panes))
	for i, pane := range panes {
		parts := paneLines(pane, root)
		if title := paneTitle(pane, len(panes)); title != "" {
			if i == selected {
				title = "[ " + title + " ]"
			}
			parts = append([]string{title}, parts...)
		}
		if alertOf(alerts, i) != noAlert {
			parts = append([]string{"⚠"}, parts...)
		}
		lines = append(lines, ansi.Truncate(strings.Join(parts, " · "), width, "…"))
	}
	return fitted(lines, max(height, 1))
}

// neighbour is the pane whose box is next to boxes[from] across one
// border, to the right for dx 1, the left for -1, below for dy 1 or
// above for -1: the one sharing the most of that side, the first of
// those sharing as much. There is none at the window's edge.
func neighbour(boxes []box, from, dx, dy int) (int, bool) {
	f := boxes[from]
	found, shared := -1, 0
	for i, b := range boxes {
		var next bool
		var overlap int
		if dx != 0 {
			next = (dx > 0 && b.x == f.x+f.width+1) || (dx < 0 && b.x+b.width+1 == f.x)
			overlap = min(b.y+b.height, f.y+f.height) - max(b.y, f.y)
		} else {
			next = (dy > 0 && b.y == f.y+f.height+1) || (dy < 0 && b.y+b.height+1 == f.y)
			overlap = min(b.x+b.width, f.x+f.width) - max(b.x, f.x)
		}
		if next && overlap > shared {
			found, shared = i, overlap
		}
	}
	return found, found >= 0
}

// fitted is lines in height lines at most, the last saying how many are
// left out when they don't fit.
func fitted(lines []string, height int) []string {
	switch {
	case height <= 0:
		return nil
	case len(lines) > height:
		return append(lines[:height-1:height-1], fmt.Sprintf("… %d more", len(lines)-height+1))
	}
	return lines
}

// paneTitle is the name tmux shows in the pane's top border, which it
// draws only when the window has several panes.
func paneTitle(pane domain.PanePlan, count int) string {
	if count == 1 {
		return ""
	}
	return pane.Name().String()
}

// paneLines are what the pane shows inside its borders when it opens:
// its dir unless it is root, its note, then its prompt, with the command
// run ("$ task dev"), left typed ("$ nimbus emulator█") or none ("$ █").
func paneLines(pane domain.PanePlan, root string) []string {
	var lines []string
	if dir := pane.Dir(); dir != root {
		if rel, err := filepath.Rel(root, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			lines = append(lines, rel+"/")
		} else {
			lines = append(lines, contractHome(dir)+"/")
		}
	}
	lines = append(lines, pane.Note()...)
	cmd, ok := pane.Startup().Command()
	switch {
	case !ok:
		lines = append(lines, "$ █")
	case pane.Startup().Submits():
		lines = append(lines, "$ "+cmd.String())
	default:
		lines = append(lines, "$ "+cmd.String()+"█")
	}
	return lines
}

// scaled is n of total mapped onto to, rounded to the nearest.
func scaled(n, total, to int) int {
	if total <= 0 {
		return 0
	}
	return (2*n*to + total) / (2 * total)
}

// grid is a drawing's cells, each holding what is drawn there, which
// sides its border joins, and which of those are heavy.
type grid struct {
	cells  [][]string
	joins  [][]int
	heavy  [][]int
	alerts [][]alert
}

func newGrid(width, height int) grid {
	g := grid{cells: make([][]string, height), joins: make([][]int, height), heavy: make([][]int, height), alerts: make([][]alert, height)}
	for y := range g.cells {
		g.cells[y] = make([]string, width)
		g.joins[y] = make([]int, width)
		g.heavy[y] = make([]int, width)
		g.alerts[y] = make([]alert, width)
		for x := range g.cells[y] {
			g.cells[y][x] = " "
		}
	}
	return g
}

// line joins the cells from (x1, y1) to (x2, y2), on one row or one
// column, each to its neighbours on the line, heavy when thick is set.
func (g grid) line(x1, y1, x2, y2 int, thick bool) {
	if x1 == x2 && y1 == y2 {
		return
	}
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			if y < 0 || y >= len(g.joins) || x < 0 || x >= len(g.joins[y]) {
				continue
			}
			var sides int // the sides of the cell this line joins
			if y1 == y2 {
				if x > x1 {
					sides |= joinsLeft
				}
				if x < x2 {
					sides |= joinsRight
				}
			} else {
				if y > y1 {
					sides |= joinsUp
				}
				if y < y2 {
					sides |= joinsDown
				}
			}
			g.joins[y][x] |= sides
			if thick {
				g.heavy[y][x] |= sides
			}
		}
	}
}

// drawBorders puts in every joined cell the character joining its sides.
func (g grid) drawBorders() {
	for y, row := range g.joins {
		for x, joins := range row {
			if joins != 0 {
				g.cells[y][x] = borderRune(joins, g.heavy[y][x])
			}
		}
	}
}

// write puts text at (x, y), tabs as spaces, cut to width cells with "…"
// in place of the rest. The text is kept whole in its first cell, the
// cells it covers left empty, so the terminal measures it, combining
// marks and all.
func (g grid) write(x, y int, text string, width int) {
	if width <= 0 || y < 0 || y >= len(g.cells) || x < 0 || x >= len(g.cells[y]) {
		return
	}
	text = ansi.Truncate(strings.ReplaceAll(text, "\t", " "), min(width, len(g.cells[y])-x), "…")
	cells := ansi.StringWidth(text)
	if cells == 0 {
		return
	}
	for i := x; i < x+cells; i++ {
		g.cells[y][i] = ""
		g.joins[y][i] = 0 // text, no longer border
	}
	g.cells[y][x] = text
}

// alert colours the border of the box from (left, top) to (right,
// bottom) as a, unless a worse alert coloured it, but not the text
// written over it, such as a neighbour's title.
func (g grid) alert(left, top, right, bottom int, a alert) {
	for y := max(top, 0); y <= bottom && y < len(g.alerts); y++ {
		for x := max(left, 0); x <= right && x < len(g.alerts[y]); x++ {
			if (y == top || y == bottom || x == left || x == right) && g.joins[y][x] != 0 {
				g.alerts[y][x] = max(g.alerts[y][x], a)
			}
		}
	}
}

// alertCell colours the text written at (x, y) as a, if any.
func (g grid) alertCell(x, y int, a alert) {
	if y >= 0 && y < len(g.cells) && x >= 0 && x < len(g.cells[y]) && g.joins[y][x] == 0 && g.cells[y][x] != "" {
		g.alerts[y][x] = max(g.alerts[y][x], a)
	}
}

// rows are the drawn rows, the selected pane's border in picked.
func (g grid) rows(picked lipgloss.Style) []string {
	rows := make([]string, len(g.cells))
	for y, row := range g.cells {
		var b strings.Builder
		for x, cell := range row {
			style, ok := alertStyles[g.alerts[y][x]]
			if g.alerts[y][x] == selectedPane {
				style, ok = picked, true
			}
			if ok && cell != "" {
				cell = style.Render(cell)
			}
			b.WriteString(cell)
		}
		rows[y] = b.String()
	}
	return rows
}
