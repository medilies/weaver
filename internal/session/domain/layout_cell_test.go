package domain_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

// carved writes a creation order as "pane", then "pane right of #1 67%"
// for each later pane, #1 being the first in the order.
func carved(order []domain.CarvedPane) []string {
	out := make([]string, len(order))
	for i, p := range order {
		out[i] = fmt.Sprint(p.Pane())
		if split, ok := p.Split(); ok {
			out[i] += fmt.Sprintf(" %s of #%d %s", split.Direction(), split.Of()+1, split.Size())
		}
	}
	return out
}

func TestLayoutCellCreationOrder(t *testing.T) {
	t.Run("a pane alone splits nothing", func(t *testing.T) {
		got := carved(domain.PaneCell(7, 1).CreationOrder())
		if want := []string{"7"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("each cell takes its share of the cells it is split from", func(t *testing.T) {
		cell := domain.SideBySide(3, domain.PaneCell(0, 1), domain.PaneCell(1, 1), domain.PaneCell(2, 1))
		got := carved(cell.CreationOrder())
		if want := []string{"0", "1 right of #1 67%", "2 right of #2 50%"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("splits sibling cells before their insides", func(t *testing.T) {
		cell := domain.SideBySide(1,
			domain.Stacked(60, domain.PaneCell(0, 70), domain.PaneCell(1, 30)),
			domain.PaneCell(2, 40),
		)
		got := carved(cell.CreationOrder())
		if want := []string{"0", "2 right of #1 40%", "1 below of #1 30%"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps every share between 1% and 99%", func(t *testing.T) {
		cell := domain.Stacked(1, domain.PaneCell(0, 1000), domain.PaneCell(1, 1))
		got := carved(cell.CreationOrder())
		if want := []string{"0", "1 below of #1 1%"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
