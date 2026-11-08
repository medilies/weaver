package domain

import (
	"math"
	"slices"
)

// LayoutCell is how tmux tiles a window, as #{window_layout} writes it: a
// pane, or cells side by side or stacked. A cell's size only counts
// against its siblings', in any unit.
type LayoutCell struct {
	size       float64
	pane       int // -1 for cells side by side or stacked
	sideBySide bool
	children   []LayoutCell
}

// PaneCell is the pane numbered pane, of size. The number is the caller's:
// CreationOrder hands it back.
func PaneCell(pane int, size float64) LayoutCell {
	return LayoutCell{size: size, pane: pane}
}

// SideBySide is cells side by side, left to right, of size.
func SideBySide(size float64, cells ...LayoutCell) LayoutCell {
	return LayoutCell{size: size, pane: -1, sideBySide: true, children: slices.Clone(cells)}
}

// Stacked is cells stacked, top to bottom, of size.
func Stacked(size float64, cells ...LayoutCell) LayoutCell {
	return LayoutCell{size: size, pane: -1, children: slices.Clone(cells)}
}

// CarvedPane is a pane of a layout cell, in an order that recreates the
// cell with tmux split-window semantics.
type CarvedPane struct {
	pane   int
	split  Split
	splits bool
}

// Pane is the pane's number, as given to PaneCell.
func (p CarvedPane) Pane() int {
	return p.pane
}

// Split is how the pane is created, unless it comes first: its Of is an
// index in the creation order.
func (p CarvedPane) Split() (Split, bool) {
	return p.split, p.splits
}

// CreationOrder lists c's panes in an order that recreates it: the first
// fills the cell, each later one splits an earlier one. Among cells side
// by side or stacked, cell k is split off the first pane of cell k-1,
// which still covers cells k-1 to the last, and takes the share of them
// cells k to the last cover. Sibling cells are split before their
// insides, which then split the panes they start as.
func (c LayoutCell) CreationOrder() []CarvedPane {
	order := []CarvedPane{{pane: c.firstPane()}}
	index := func(pane int) int {
		for i, p := range order {
			if p.pane == pane {
				return i
			}
		}
		return -1
	}
	var carve func(cell LayoutCell)
	carve = func(cell LayoutCell) {
		children := cell.children
		direction := Below
		if cell.sideBySide {
			direction = Right
		}
		remaining := make([]float64, len(children)+1) // remaining[k]: cells k to the last
		for k := len(children) - 1; k >= 0; k-- {
			remaining[k] = remaining[k+1] + children[k].size
		}
		for k := 1; k < len(children); k++ {
			percent := int(math.Round(100 * remaining[k] / remaining[k-1]))
			order = append(order, CarvedPane{
				pane: children[k].firstPane(),
				split: Split{
					of:        index(children[k-1].firstPane()),
					direction: direction,
					size:      Size{amount: min(max(percent, 1), 99), percent: true},
				},
				splits: true,
			})
		}
		for _, child := range children {
			carve(child)
		}
	}
	carve(c)
	return order
}

func (c LayoutCell) firstPane() int {
	for len(c.children) > 0 {
		c = c.children[0]
	}
	return c.pane
}
