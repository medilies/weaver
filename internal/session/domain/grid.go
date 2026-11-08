package domain

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Track is the width of a grid's column or the height of its row: a
// percentage of the window, or a weight (fr) sharing out what the
// percentages leave.
type Track struct {
	amount int
	weight bool
}

// ParseTrack reads "40%" (a percentage, up to 100%) or "2fr" (a weight).
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
	return Track{}, fmt.Errorf("%w: %q (want e.g. 40%% or 2fr)", ErrInvalidTrack, raw)
}

// String is the track as ParseTrack reads it.
func (t Track) String() string {
	if t.weight {
		return strconv.Itoa(t.amount) + "fr"
	}
	return strconv.Itoa(t.amount) + "%"
}

// Grid places a window's panes by naming them in rows and columns: a pane
// covers the rectangle of cells holding its name. Columns and rows are
// equal unless sized.
type Grid struct {
	areas   [][]PaneName
	columns []Track
	rows    []Track
	names   []PaneName // in reading order
	cell    LayoutCell // panes numbered by their index in names
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
	columnShares, err := shares(columns, width, "columns")
	if err != nil {
		return Grid{}, err
	}
	rowShares, err := shares(rows, len(areas), "rows")
	if err != nil {
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
	}
	g.cell, err = g.cut(columnShares, rowShares)
	if err != nil {
		return Grid{}, err
	}
	return g, nil
}

// shares is each of n tracks' share of the window, in percent: equal
// without tracks, else percentages as they are and weights sharing out
// the rest.
func shares(tracks []Track, n int, axis string) ([]float64, error) {
	out := make([]float64, n)
	if len(tracks) == 0 {
		for i := range out {
			out[i] = 100 / float64(n)
		}
		return out, nil
	}
	if len(tracks) != n {
		return nil, fmt.Errorf("%w: %d sizes for %d %s", ErrTrackCount, len(tracks), n, axis)
	}
	percent, weights := 0, 0
	for _, t := range tracks {
		if t.weight {
			weights += t.amount
		} else {
			percent += t.amount
		}
	}
	switch {
	case weights == 0 && percent != 100:
		return nil, fmt.Errorf("%w: %s add up to %d%% (want 100%%)", ErrTrackShares, axis, percent)
	case weights > 0 && percent >= 100:
		return nil, fmt.Errorf("%w: %s take %d%%, leaving nothing to share by fr", ErrTrackShares, axis, percent)
	}
	for i, t := range tracks {
		out[i] = float64(t.amount)
		if t.weight {
			out[i] = float64(100-percent) * float64(t.amount) / float64(weights)
		}
	}
	return out, nil
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
// else stacked.
func (g Grid) cut(columnShares, rowShares []float64) (LayoutCell, error) {
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
				child, err := cut(top, lines[k-1], bottom, lines[k], sum(columnShares[lines[k-1]:lines[k]]))
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
				child, err := cut(lines[k-1], left, lines[k], right, sum(rowShares[lines[k-1]:lines[k]]))
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
