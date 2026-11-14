package infra

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestThemeKeys(t *testing.T) {
	line, spots := defaultTheme.keys([]keyTold{{"a", "x"}, {"bb", "yy"}})

	t.Run("tells each key and what it does, two spaces apart", func(t *testing.T) {
		if got, want := ansi.Strip(line), "a x  bb yy"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("covers each key and what it does, to click it", func(t *testing.T) {
		want := []hotspot{{from: 0, to: 3, key: "a"}, {from: 5, to: 10, key: "bb"}}
		if !slices.Equal(spots, want) {
			t.Fatalf("got %v, want %v", spots, want)
		}
	})
}

func TestNamesList(t *testing.T) {
	line, spots := namesList(defaultTheme, []string{"api", "web"}, []int{0, 1}, 0, 40, true, []alert{noAlert, dirAlert})

	t.Run("puts the glyph of a pane's alert before its name", func(t *testing.T) {
		if got, want := ansi.Strip(line), " api   ✗ web "; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("clicks on the glyph select its pane", func(t *testing.T) {
		if got := spots[1]; got.from != 6 || got.item != 1 {
			t.Fatalf("got %v, want the second pane's spot to start at the glyph, cell 6", got)
		}
	})
}
