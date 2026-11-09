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

func TestListed(t *testing.T) {
	labels := []string{" api ", " web ", " db "}

	t.Run("gives each label the cells it covers, a space apart", func(t *testing.T) {
		_, got := listed(labels, 0, 40)
		want := []hotspot{{from: 0, to: 5, item: 0}, {from: 6, to: 11, item: 1}, {from: 12, to: 16, item: 2}}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("drops the first labels for the selected one when they don't fit", func(t *testing.T) {
		line, got := listed(labels, 2, 12)
		want := []hotspot{{from: 1, to: 6, item: 1}, {from: 7, to: 11, item: 2}}
		if line != "… web   db " || !slices.Equal(got, want) {
			t.Fatalf("got %q %v, want %q %v", line, got, "… web   db ", want)
		}
	})
}
