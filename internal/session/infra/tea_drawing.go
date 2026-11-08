package infra

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/domain"
)

// box is the cells a pane covers in its window, its borders left out:
// tmux draws one cell of border between panes.
type box struct {
	x, y, width, height int
}

// layOut places window's panes in a window of cols×rows cells as tmux
// splits them: the first pane fills the window, and each later one cuts
// the pane it splits, taking its size of it, half by default, past one
// cell of border.
func layOut(window domain.WindowPlan, cols, rows int) []box {
	panes := window.Panes()
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

var borderRunes = map[int]string{
	joinsLeft | joinsRight:                       "─",
	joinsUp | joinsDown:                          "│",
	joinsDown | joinsRight:                       "┌",
	joinsDown | joinsLeft:                        "┐",
	joinsUp | joinsRight:                         "└",
	joinsUp | joinsLeft:                          "┘",
	joinsUp | joinsDown | joinsRight:             "├",
	joinsUp | joinsDown | joinsLeft:              "┤",
	joinsDown | joinsLeft | joinsRight:           "┬",
	joinsUp | joinsLeft | joinsRight:             "┴",
	joinsUp | joinsDown | joinsLeft | joinsRight: "┼",
	joinsLeft:  "─",
	joinsRight: "─",
	joinsUp:    "│",
	joinsDown:  "│",
}

// alert is the worst warning about a pane, which colours its box.
type alert int

const (
	noAlert alert = iota
	programAlert
	dirAlert
)

var alertStyles = map[alert]lipgloss.Style{
	programAlert: warningStyle,
	dirAlert:     problemStyle,
}

// drawing is window's panes drawn as boxes in width×height cells, in the
// proportions they take in a window of cols×rows. Each box shows what
// its pane looks like the moment it opens: its name in the top border,
// its dir when it isn't root, its note, then its prompt. A pane with an
// alert, from alerts by its index, gets ⚠ in its top border and a border
// in the alert's colour, the worst winning on a border boxes share.
func drawing(window domain.WindowPlan, root string, alerts []alert, cols, rows, width, height int) []string {
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
		grid.line(f.left, f.top, f.right, f.top)
		grid.line(f.left, f.bottom, f.right, f.bottom)
		grid.line(f.left, f.top, f.left, f.bottom)
		grid.line(f.right, f.top, f.right, f.bottom)
	}
	grid.drawBorders()

	panes := window.Panes()
	for i, pane := range panes {
		f := frames[i]
		title := paneTitle(pane, len(panes))
		if alertOf(alerts, i) != noAlert {
			title = strings.TrimSpace("⚠ " + title)
		}
		if width := f.right - f.left - 5; title != "" && width > 0 {
			grid.write(f.left+2, f.top, " "+ansi.Truncate(title, width, "…")+" ", width+2)
		}
		for j, text := range paneLines(pane, root) {
			if f.top+1+j >= f.bottom {
				break
			}
			grid.write(f.left+2, f.top+1+j, text, f.right-f.left-3)
		}
	}
	for i, f := range frames {
		// A box scaled too small to show is left alone: colouring it would
		// colour the boxes it collapsed into.
		if a := alertOf(alerts, i); a != noAlert && f.right-f.left >= 2 && f.bottom-f.top >= 2 {
			grid.alert(f.left, f.top, f.right, f.bottom, a)
			grid.alertCell(f.left+2, f.top, a) // the title, if written
		}
	}
	return grid.rows()
}

// alertOf is the alert of the pane at index, none past the end of alerts.
func alertOf(alerts []alert, index int) alert {
	if index < len(alerts) {
		return alerts[index]
	}
	return noAlert
}

// paneList is window's panes one a line, for a space too small to draw
// them in: "api shell · api/ · $ task dev", each cut to width cells, ⚠
// first for a pane with an alert. When
// they take more than height lines, the last line says how many are left
// out.
func paneList(window domain.WindowPlan, root string, alerts []alert, width, height int) []string {
	panes := window.Panes()
	lines := make([]string, 0, len(panes))
	for i, pane := range panes {
		parts := paneLines(pane, root)
		if title := paneTitle(pane, len(panes)); title != "" {
			parts = append([]string{title}, parts...)
		}
		if alertOf(alerts, i) != noAlert {
			parts = append([]string{"⚠"}, parts...)
		}
		lines = append(lines, ansi.Truncate(strings.Join(parts, " · "), width, "…"))
	}
	if height = max(height, 1); len(lines) > height {
		lines = append(lines[:height-1], fmt.Sprintf("… %d more", len(lines)-height+1))
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

// grid is a drawing's cells, each holding what is drawn there and which
// sides its border joins.
type grid struct {
	cells  [][]string
	joins  [][]int
	alerts [][]alert
}

func newGrid(width, height int) grid {
	g := grid{cells: make([][]string, height), joins: make([][]int, height), alerts: make([][]alert, height)}
	for y := range g.cells {
		g.cells[y] = make([]string, width)
		g.joins[y] = make([]int, width)
		g.alerts[y] = make([]alert, width)
		for x := range g.cells[y] {
			g.cells[y][x] = " "
		}
	}
	return g
}

// line joins the cells from (x1, y1) to (x2, y2), on one row or one
// column, each to its neighbours on the line.
func (g grid) line(x1, y1, x2, y2 int) {
	if x1 == x2 && y1 == y2 {
		return
	}
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			if y < 0 || y >= len(g.joins) || x < 0 || x >= len(g.joins[y]) {
				continue
			}
			if y1 == y2 {
				if x > x1 {
					g.joins[y][x] |= joinsLeft
				}
				if x < x2 {
					g.joins[y][x] |= joinsRight
				}
			} else {
				if y > y1 {
					g.joins[y][x] |= joinsUp
				}
				if y < y2 {
					g.joins[y][x] |= joinsDown
				}
			}
		}
	}
}

// drawBorders puts in every joined cell the character joining its sides.
func (g grid) drawBorders() {
	for y, row := range g.joins {
		for x, joins := range row {
			if joins != 0 {
				g.cells[y][x] = borderRunes[joins]
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

func (g grid) rows() []string {
	rows := make([]string, len(g.cells))
	for y, row := range g.cells {
		var b strings.Builder
		for x, cell := range row {
			if style, ok := alertStyles[g.alerts[y][x]]; ok && cell != "" {
				cell = style.Render(cell)
			}
			b.WriteString(cell)
		}
		rows[y] = b.String()
	}
	return rows
}
