package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func checkout(t *testing.T, dir, remote string) domain.Checkout {
	t.Helper()
	c, err := domain.NewCheckout(dir, remote)
	if err != nil {
		t.Fatalf("NewCheckout(%q, %q): %v", dir, remote, err)
	}
	return c
}

func TestNewCheckout(t *testing.T) {
	t.Run("treats a blank directory as the project root", func(t *testing.T) {
		if got := checkout(t, "", "git@host:repo.git").Dir(); got != "." {
			t.Fatalf("got %q, want %q", got, ".")
		}
	})

	t.Run("rejects an absolute directory", func(t *testing.T) {
		_, err := domain.NewCheckout("/srv/back", "git@host:repo.git")
		if !errors.Is(err, domain.ErrInvalidCheckoutDir) {
			t.Fatalf("got %v, want ErrInvalidCheckoutDir", err)
		}
	})

	t.Run("rejects a directory outside the project root", func(t *testing.T) {
		_, err := domain.NewCheckout("back/../../elsewhere", "git@host:repo.git")
		if !errors.Is(err, domain.ErrInvalidCheckoutDir) {
			t.Fatalf("got %v, want ErrInvalidCheckoutDir", err)
		}
	})

	t.Run("rejects a blank remote", func(t *testing.T) {
		_, err := domain.NewCheckout("back", " ")
		if !errors.Is(err, domain.ErrEmptyRemote) {
			t.Fatalf("got %v, want ErrEmptyRemote", err)
		}
	})
}

func TestNewCheckouts(t *testing.T) {
	t.Run("orders the root first and parents before nested repositories", func(t *testing.T) {
		checkouts, err := domain.NewCheckouts(
			checkout(t, "front/docs", "git@host:docs.git"),
			checkout(t, "back", "git@host:back.git"),
			checkout(t, "front", "git@host:front.git"),
			checkout(t, ".", "git@host:root.git"),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var dirs []string
		for _, c := range checkouts.List() {
			dirs = append(dirs, c.Dir())
		}
		if want := []string{".", "back", "front", "front/docs"}; !slices.Equal(dirs, want) {
			t.Fatalf("got %q, want %q", dirs, want)
		}
	})

	t.Run("rejects two repositories in one directory", func(t *testing.T) {
		_, err := domain.NewCheckouts(
			checkout(t, "back", "git@host:back.git"),
			checkout(t, "back/", "git@host:other.git"),
		)
		if !errors.Is(err, domain.ErrDuplicateCheckout) {
			t.Fatalf("got %v, want ErrDuplicateCheckout", err)
		}
	})
}
