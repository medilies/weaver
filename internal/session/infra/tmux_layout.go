package infra

import (
	"errors"
	"fmt"
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
	if cell.height, err = p.numberAfter('x'); err != nil {
		return layoutCell{}, err
	}
	if _, err = p.numberAfter(','); err != nil { // X offset, unused
		return layoutCell{}, err
	}
	if _, err = p.numberAfter(','); err != nil { // Y offset, unused
		return layoutCell{}, err
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

// numberAfter reads the separator sep, then a number.
func (p *layoutParser) numberAfter(sep byte) (int, error) {
	if err := p.expect(sep); err != nil {
		return 0, err
	}
	return p.number()
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

// layoutCell is c as the domain reads it. A cell's size is along its
// container's axis, and every cell but the last also counts the border
// after it, so the shares split-window takes come out as tmux drew them.
func (c layoutCell) layoutCell(size int) domain.LayoutCell {
	if len(c.children) == 0 {
		return domain.PaneCell(c.pane, float64(size))
	}
	children := make([]domain.LayoutCell, len(c.children))
	for k, child := range c.children {
		along := child.height
		if c.sideBySide {
			along = child.width
		}
		if k < len(c.children)-1 {
			along++ // the border
		}
		children[k] = child.layoutCell(along)
	}
	if c.sideBySide {
		return domain.SideBySide(float64(size), children...)
	}
	return domain.Stacked(float64(size), children...)
}
