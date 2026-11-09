package domain

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Track is the width of a grid's column or the height of its row: a
// percentage of the window, a number of cells, or a weight (fr) sharing
// out what the others leave.
type Track struct {
	amount int
	weight bool
	cells  bool
}

// ParseTrack reads "40%" (a percentage, up to 100%), "30" (cells) or
// "2fr" (a weight).
func ParseTrack(raw string) (Track, error) {
	trimmed := strings.TrimSpace(raw)
	if digits, ok := strings.CutSuffix(trimmed, "%"); ok {
		if n, err := strconv.Atoi(digits); err == nil && n >= 1 && n <= 100 {
			return Track{amount: n}, nil
		}
	}
	if digits, ok := strings.CutSuffix(trimmed, "fr"); ok {
		if n, err := strconv.Atoi(digits); err == nil && n >= 1 {
			return Track{amount: n, weight: true}, nil
		}
	}
	if n, err := strconv.Atoi(trimmed); err == nil && n >= 1 && !strings.HasPrefix(trimmed, "+") {
		return Track{amount: n, cells: true}, nil
	}
	return Track{}, fmt.Errorf("%w: %q (want e.g. 40%%, 30 or 2fr)", ErrInvalidTrack, raw)
}

// String is the track as ParseTrack reads it.
func (t Track) String() string {
	switch {
	case t.weight:
		return strconv.Itoa(t.amount) + "fr"
	case t.cells:
		return strconv.Itoa(t.amount)
	}
	return strconv.Itoa(t.amount) + "%"
}

// nominalCols and nominalRows are the window size, tmux's default, a grid
// with tracks in cells is drawn at until it is sized to a window.
const nominalCols, nominalRows = 80, 24

// Grid places a window's panes by naming them in rows and columns: a pane
// covers the rectangle of cells holding its name. Columns and rows are
// equal unless sized.
type Grid struct {
	areas   [][]PaneName
	columns []Track
	rows    []Track
	names   []PaneName // in reading order
	cell    LayoutCell // panes numbered by their index in names
	inCells bool       // a track is in cells, so the cut depends on the window's size
}

// NewGrid validates and constructs a Grid from its rows of pane names and
// its column and row sizes, none meaning equal ones. tmux only cuts a
// window in straight lines, each piece cut again, so a grid whose panes
// no straight line separates is refused.
func NewGrid(areas [][]PaneName, columns, rows []Track) (Grid, error) {
	if len(areas) == 0 || slices.ContainsFunc(areas, func(row []PaneName) bool { return len(row) == 0 }) {
		return Grid{}, ErrEmptyGrid
	}
	width := len(areas[0])
	for i, row := range areas {
		if len(row) != width {
			return Grid{}, fmt.Errorf("%w: row %d names %d, row 1 names %d", ErrRaggedGrid, i+1, len(row), width)
		}
	}
	if err := checkTracks(columns, width, "columns"); err != nil {
		return Grid{}, err
	}
	if err := checkTracks(rows, len(areas), "rows"); err != nil {
		return Grid{}, err
	}
	names, err := rectangles(areas)
	if err != nil {
		return Grid{}, err
	}
	g := Grid{
		areas:   cloneAreas(areas),
		columns: slices.Clone(columns),
		rows:    slices.Clone(rows),
		names:   names,
		inCells: slices.ContainsFunc(columns, Track.inCells) || slices.ContainsFunc(rows, Track.inCells),
	}
	if g.inCells {
		g.cell, err = g.cutIn(nominalCols, nominalRows)
	} else {
		g.cell, err = g.cut(shares(columns, width), shares(rows, len(areas)), 0)
	}
	if err != nil {
		return Grid{}, err
	}
	return g, nil
}

func (t Track) inCells() bool {
	return t.cells
}

// checkTracks checks n tracks share out the window: one per column or
// row, or none for equal ones; percentages adding up to 100% without
// weights, and to less with some; tracks in cells only beside a weight,
// which takes what they leave.
func checkTracks(tracks []Track, n int, axis string) error {
	if len(tracks) == 0 {
		return nil
	}
	if len(tracks) != n {
		return fmt.Errorf("%w: %d sizes for %d %s", ErrTrackCount, len(tracks), n, axis)
	}
	percent, weights, cells := 0, 0, false
	for _, t := range tracks {
		switch {
		case t.weight:
			weights += t.amount
		case t.cells:
			cells = true
		default:
			percent += t.amount
		}
	}
	switch {
	case cells && weights == 0:
		return fmt.Errorf("%w: %s in cells need a column or row in fr to take the rest", ErrTrackShares, axis)
	case weights == 0 && percent != 100:
		return fmt.Errorf("%w: %s add up to %d%% (want 100%%)", ErrTrackShares, axis, percent)
	case weights > 0 && percent >= 100:
		return fmt.Errorf("%w: %s take %d%%, leaving nothing to share by fr", ErrTrackShares, axis, percent)
	}
	return nil
}

// shares is each of n tracks' share of the window, in percent: equal
// without tracks, else percentages as they are and weights sharing out
// the rest. None is in cells.
func shares(tracks []Track, n int) []float64 {
	out := make([]float64, n)
	if len(tracks) == 0 {
		for i := range out {
			out[i] = 100 / float64(n)
		}
		return out
	}
	percent, weights := 0, 0
	for _, t := range tracks {
		if t.weight {
			weights += t.amount
		} else {
			percent += t.amount
		}
	}
	for i, t := range tracks {
		out[i] = float64(t.amount)
		if t.weight {
			out[i] = float64(100-percent) * float64(t.amount) / float64(weights)
		}
	}
	return out
}

// cellsIn is how many cells each of n tracks takes in total cells, the
// n-1 borders between them aside: equal without tracks, else cells as
// they are, percentages of what the borders leave, and weights sharing
// out the rest. The last weight, or the last track without one, takes
// what rounding leaves. Each takes a cell at least, as tmux makes it, so
// a window too small for the cells asked shrinks none of them.
func cellsIn(tracks []Track, n, total int) []float64 {
	room := max(total-(n-1), n)
	out := make([]float64, n)
	if len(tracks) == 0 {
		for i := range out {
			out[i] = float64(room / n)
		}
		out[n-1] += float64(room % n)
		return out
	}
	taken, weights, last := 0, 0, -1
	for i, t := range tracks {
		switch {
		case t.weight:
			weights += t.amount
			last = i
		case t.cells:
			taken += t.amount
		default:
			taken += t.amount * room / 100
		}
	}
	if weights == 0 {
		// Percentages only, adding up to 100%: the last takes what
		// rounding leaves.
		left := room
		for i, t := range tracks[:n-1] {
			out[i] = max(float64(t.amount*room/100), 1)
			left -= int(out[i])
		}
		out[n-1] = float64(max(left, 1))
		return out
	}
	rest, shared := max(room-taken, 0), 0
	for i, t := range tracks {
		switch {
		case t.weight:
			n := rest * t.amount / weights
			if i == last {
				n = rest - shared
			}
			shared += n
			out[i] = float64(n)
		case t.cells:
			out[i] = float64(t.amount)
		default:
			out[i] = float64(t.amount * room / 100)
		}
		out[i] = max(out[i], 1)
	}
	return out
}

// cutIn is the grid's cut in a window of cols×rows, in cells.
func (g Grid) cutIn(cols, rows int) (LayoutCell, error) {
	return g.cut(cellsIn(g.columns, len(g.areas[0]), cols), cellsIn(g.rows, len(g.areas), rows), 1)
}

// rectangles checks each name's cells form one rectangle, and returns
// the names in reading order.
func rectangles(areas [][]PaneName) ([]PaneName, error) {
	type box struct{ top, left, bottom, right, cells int }
	boxes := make(map[PaneName]*box)
	var names []PaneName
	for r, row := range areas {
		for c, name := range row {
			if name.IsZero() {
				return nil, fmt.Errorf("row %d: %w", r+1, ErrEmptyPaneName)
			}
			b, seen := boxes[name]
			if !seen {
				b = &box{top: r, left: c, bottom: r, right: c}
				boxes[name] = b
				names = append(names, name)
			}
			b.top, b.left = min(b.top, r), min(b.left, c)
			b.bottom, b.right = max(b.bottom, r), max(b.right, c)
			b.cells++
		}
	}
	for _, name := range names {
		b := boxes[name]
		if b.cells != (b.bottom-b.top+1)*(b.right-b.left+1) {
			return nil, fmt.Errorf("%w: %q", ErrPaneNotRectangle, name.value)
		}
	}
	return names, nil
}

// cut turns the grid into the layout cell tmux builds: in each region,
// the straight lines crossing no pane cut it into cells side by side, or
// else stacked. A cell covering several columns or rows is their sizes
// and the borders between them, each border's size.
func (g Grid) cut(columnShares, rowShares []float64, border float64) (LayoutCell, error) {
	span := func(sizes []float64) float64 { return sum(sizes) + border*float64(len(sizes)-1) }
	index := make(map[PaneName]int, len(g.names))
	for i, name := range g.names {
		index[name] = i
	}
	var cut func(top, left, bottom, right int, size float64) (LayoutCell, error)
	cut = func(top, left, bottom, right int, size float64) (LayoutCell, error) {
		// Names are rectangles: one holding both corners fills the region.
		if first := g.areas[top][left]; g.areas[bottom-1][right-1] == first {
			return PaneCell(index[first], size), nil
		}
		if lines := g.lines(left, right, func(c int) bool {
			for r := top; r < bottom; r++ {
				if g.areas[r][c-1] == g.areas[r][c] {
					return false
				}
			}
			return true
		}); len(lines) > 2 {
			children := make([]LayoutCell, 0, len(lines)-1)
			for k := 1; k < len(lines); k++ {
				child, err := cut(top, lines[k-1], bottom, lines[k], span(columnShares[lines[k-1]:lines[k]]))
				if err != nil {
					return LayoutCell{}, err
				}
				children = append(children, child)
			}
			return SideBySide(size, children...), nil
		}
		if lines := g.lines(top, bottom, func(r int) bool {
			for c := left; c < right; c++ {
				if g.areas[r-1][c] == g.areas[r][c] {
					return false
				}
			}
			return true
		}); len(lines) > 2 {
			children := make([]LayoutCell, 0, len(lines)-1)
			for k := 1; k < len(lines); k++ {
				child, err := cut(lines[k-1], left, lines[k], right, span(rowShares[lines[k-1]:lines[k]]))
				if err != nil {
					return LayoutCell{}, err
				}
				children = append(children, child)
			}
			return Stacked(size, children...), nil
		}
		return LayoutCell{}, fmt.Errorf("%w: %s", ErrInterlockedPanes, g.namesIn(top, left, bottom, right))
	}
	return cut(0, 0, len(g.areas), len(g.areas[0]), 100)
}

// lines is from, the lines between from and to that clear says cross no
// pane, and to.
func (g Grid) lines(from, to int, clear func(int) bool) []int {
	lines := []int{from}
	for i := from + 1; i < to; i++ {
		if clear(i) {
			lines = append(lines, i)
		}
	}
	return append(lines, to)
}

// namesIn lists the panes in a region, quoted, in reading order.
func (g Grid) namesIn(top, left, bottom, right int) string {
	var quoted []string
	for _, row := range g.areas[top:bottom] {
		for _, name := range row[left:right] {
			if q := strconv.Quote(name.value); !slices.Contains(quoted, q) {
				quoted = append(quoted, q)
			}
		}
	}
	return strings.Join(quoted, ", ")
}

func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

func cloneAreas(areas [][]PaneName) [][]PaneName {
	out := make([][]PaneName, len(areas))
	for i, row := range areas {
		out[i] = slices.Clone(row)
	}
	return out
}

// WithAreas is the grid placing its panes by areas instead, keeping its
// columns' sizes while it has as many columns, and its rows' while it has
// as many rows: sizes for another count would refuse it, so changing the
// count makes them equal.
func (g Grid) WithAreas(areas [][]PaneName) (Grid, error) {
	columns, rows := g.columns, g.rows
	if len(areas) == 0 || len(g.areas) == 0 || len(areas[0]) != len(g.areas[0]) {
		columns = nil
	}
	if len(areas) != len(g.areas) {
		rows = nil
	}
	return NewGrid(areas, columns, rows)
}

// renaming is the grid with the pane named from named to instead, in
// every cell it covers. A name another pane has is refused.
func (g Grid) renaming(from, to PaneName) (Grid, error) {
	if slices.Contains(g.names, to) {
		return Grid{}, fmt.Errorf("%w: %q", ErrDuplicatePaneName, to.value)
	}
	areas := cloneAreas(g.areas)
	for _, row := range areas {
		for i, name := range row {
			if name == from {
				row[i] = to
			}
		}
	}
	return NewGrid(areas, g.columns, g.rows)
}

// Areas are the grid's rows of pane names.
func (g Grid) Areas() [][]PaneName {
	return cloneAreas(g.areas)
}

// Columns are the columns' sizes, or none for equal columns.
func (g Grid) Columns() []Track {
	return slices.Clone(g.columns)
}

// Rows are the rows' sizes, or none for equal rows.
func (g Grid) Rows() []Track {
	return slices.Clone(g.rows)
}

// carvedIn is how the grid's panes are created in a window of cols×rows,
// splits sized in cells, as CreationOrder orders them: it differs from
// the nominal cut in sizes only. Its grid has a track in cells.
func (g Grid) carvedIn(cols, rows int) []CarvedPane {
	cell, err := g.cutIn(cols, rows)
	if err != nil {
		return g.cell.CreationOrder() // NewGrid cut it already: sizes don't change the cut.
	}
	return cell.creationOrder(true)
}
