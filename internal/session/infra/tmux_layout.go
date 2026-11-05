package infra

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

var errLayout = errors.New("unexpected tmux window layout")

// layoutCell is a node of a tmux window layout, e.g. the layout
// "88c9,213x56,0,0{149x56,0,0,0,63x56,150,0,1}" is a cell holding two
// panes side by side: a checksum, then cells written as WxH,X,Y followed by
// ",<pane number>" for a pane, "{...}" for cells side by side, or "[...]"
// for cells stacked.
type layoutCell struct {
	width, height int
	pane          int // the pane's number in its tmux ID %N; -1 for a container
	sideBySide    bool
	children      []layoutCell
}

func parseLayout(layout string) (layoutCell, error) {
	_, cells, ok := strings.Cut(layout, ",")
	if !ok {
		return layoutCell{}, fmt.Errorf("%w: %q", errLayout, layout)
	}
	p := layoutParser{text: cells}
	cell, err := p.cell()
	if err == nil && p.pos != len(p.text) {
		err = errLayout
	}
	if err != nil {
		return layoutCell{}, fmt.Errorf("%w: %q", err, layout)
	}
	return cell, nil
}

type layoutParser struct {
	text string
	pos  int
}

func (p *layoutParser) cell() (layoutCell, error) {
	cell := layoutCell{pane: -1}
	var err error
	if cell.width, err = p.number(); err != nil {
		return layoutCell{}, err
	}
	for _, step := range []struct {
		sep  byte
		into *int
	}{{'x', &cell.height}, {',', new(int)}, {',', new(int)}} {
		if err := p.expect(step.sep); err != nil {
			return layoutCell{}, err
		}
		if *step.into, err = p.number(); err != nil {
			return layoutCell{}, err
		}
	}
	switch p.peek() {
	case ',':
		p.pos++
		cell.pane, err = p.number()
		return cell, err
	case '{', '[':
		cell.sideBySide = p.peek() == '{'
		closing := byte(']')
		if cell.sideBySide {
			closing = '}'
		}
		p.pos++
		for {
			child, err := p.cell()
			if err != nil {
				return layoutCell{}, err
			}
			cell.children = append(cell.children, child)
			if p.peek() != ',' {
				return cell, p.expect(closing)
			}
			p.pos++
		}
	}
	return layoutCell{}, errLayout
}

func (p *layoutParser) number() (int, error) {
	start := p.pos
	for p.pos < len(p.text) && p.text[p.pos] >= '0' && p.text[p.pos] <= '9' {
		p.pos++
	}
	return strconv.Atoi(p.text[start:p.pos])
}

func (p *layoutParser) expect(b byte) error {
	if p.peek() != b {
		return errLayout
	}
	p.pos++
	return nil
}

func (p *layoutParser) peek() byte {
	if p.pos < len(p.text) {
		return p.text[p.pos]
	}
	return 0
}

// layoutPane is a pane in an order that recreates its window: the first
// fills the window, each later one splits an earlier one.
type layoutPane struct {
	pane      int // tmux pane number
	splits    bool
	of        int // index, in this order, of the pane it splits
	direction domain.Direction
	percent   int // share of the split pane
}

// creationOrder lists root's panes in an order that recreates it with
// tmux split-window semantics. In a container, sibling k is split off the
// first pane of sibling k-1, which still covers siblings k-1 to the last,
// and takes the share of them siblings k to the last cover. Siblings are
// split before their insides, which then split the panes they start as.
func creationOrder(root layoutCell) []layoutPane {
	panes := []layoutPane{{pane: firstPane(root)}}
	index := func(pane int) int {
		for i, p := range panes {
			if p.pane == pane {
				return i
			}
		}
		return -1
	}
	var carve func(cell layoutCell)
	carve = func(cell layoutCell) {
		children := cell.children
		direction := domain.Below
		if cell.sideBySide {
			direction = domain.Right
		}
		remaining := make([]int, len(children)+1) // remaining[k]: siblings k.. with separators
		for k := len(children) - 1; k >= 0; k-- {
			size := children[k].height
			if cell.sideBySide {
				size = children[k].width
			}
			remaining[k] = remaining[k+1] + size
			if k < len(children)-1 {
				remaining[k]++ // separator
			}
		}
		for k := 1; k < len(children); k++ {
			percent := int(math.Round(100 * float64(remaining[k]) / float64(remaining[k-1])))
			panes = append(panes, layoutPane{
				pane:      firstPane(children[k]),
				splits:    true,
				of:        index(firstPane(children[k-1])),
				direction: direction,
				percent:   min(max(percent, 1), 99),
			})
		}
		for _, child := range children {
			carve(child)
		}
	}
	carve(root)
	return panes
}

func firstPane(cell layoutCell) int {
	for len(cell.children) > 0 {
		cell = cell.children[0]
	}
	return cell.pane
}
