package infra

import (
	"slices"
	"testing"
)

func TestCut(t *testing.T) {
	spots := []hotspot{{from: 0, to: 5, key: "a"}, {from: 6, to: 11, key: "b"}}

	t.Run("keeps a line that fits and all its spots", func(t *testing.T) {
		line, got := cut("alpha bravo", spots, 11)
		if line != "alpha bravo" || !slices.Equal(got, spots) {
			t.Fatalf("got %q %v, want the line and spots unchanged", line, got)
		}
	})

	t.Run("leaves the … of a cut line to act on nothing", func(t *testing.T) {
		line, got := cut("alpha bravo", spots, 8)
		want := []hotspot{{from: 0, to: 5, key: "a"}, {from: 6, to: 7, key: "b"}}
		if line != "alpha b…" || !slices.Equal(got, want) {
			t.Fatalf("got %q %v, want %q %v", line, got, "alpha b…", want)
		}
	})
}
