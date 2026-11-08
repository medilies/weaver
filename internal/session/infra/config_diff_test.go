package infra

import (
	"slices"
	"testing"
)

func TestConfigDiff(t *testing.T) {
	t.Run("shows a changed line as removed then added, among the lines kept", func(t *testing.T) {
		got := configDiff("name: shop\nroot: /srv/shop\nwindows: []\n", "name: shop\nroot: /srv/store\nwindows: []\n")
		want := []string{"  name: shop", "- root: /srv/shop", "+ root: /srv/store", "  windows: []"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("shows a comment the new config lacks as removed", func(t *testing.T) {
		got := configDiff("# mine\nroot: /srv/shop\n", "root: /srv/shop\n")
		want := []string{"- # mine", "  root: /srv/shop"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("leaves out kept lines far from any change", func(t *testing.T) {
		got := configDiff("a\nb\nc\nd\ne\nf\ng\n", "a\nb\nc\nd\ne\nf\nG\n")
		want := []string{"⋯", "  e", "  f", "- g", "+ G"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("is empty when nothing changes", func(t *testing.T) {
		if got := configDiff("root: /srv/shop\n", "root: /srv/shop\n"); len(got) != 0 {
			t.Fatalf("got %q, want nothing", got)
		}
	})
}
