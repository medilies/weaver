package domain

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Checkout is a git repository a project expects at dir, relative to its
// root ("." is the root itself), cloned from remote when missing.
type Checkout struct {
	dir    string
	remote string
}

// NewCheckout validates and constructs a Checkout.
func NewCheckout(dir, remote string) (Checkout, error) {
	d, err := oneLine(dir)
	if err != nil {
		return Checkout{}, err
	}
	if filepath.IsAbs(d) {
		return Checkout{}, fmt.Errorf("%w: %q is absolute", ErrInvalidCheckoutDir, dir)
	}
	d = filepath.Clean(d) // "" becomes "."
	if d == ".." || strings.HasPrefix(d, "../") {
		return Checkout{}, fmt.Errorf("%w: %q leaves the project root", ErrInvalidCheckoutDir, dir)
	}
	r, err := oneLine(remote)
	if err != nil {
		return Checkout{}, err
	}
	if r == "" {
		return Checkout{}, fmt.Errorf("checkout %q: %w", d, ErrEmptyRemote)
	}
	return Checkout{dir: d, remote: r}, nil
}

// Dir is relative to the project root; "." is the root itself.
func (c Checkout) Dir() string {
	return c.dir
}

func (c Checkout) Remote() string {
	return c.remote
}

// Path is where the checkout lives under root.
func (c Checkout) Path(root ProjectRoot) string {
	return filepath.Join(root.path, c.dir)
}

// Checkouts are a project's repositories, at most one per directory.
type Checkouts struct {
	list []Checkout
}

// NewCheckouts validates and constructs Checkouts.
func NewCheckouts(checkouts ...Checkout) (Checkouts, error) {
	sorted := slices.Clone(checkouts)
	slices.SortFunc(sorted, func(a, b Checkout) int {
		return cloneOrder(a.dir, b.dir)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].dir == sorted[i-1].dir {
			return Checkouts{}, fmt.Errorf("%w: %q", ErrDuplicateCheckout, sorted[i].dir)
		}
	}
	return Checkouts{list: sorted}, nil
}

// cloneOrder sorts the root first and a directory before the ones inside
// it, so a repository is cloned before repositories nested in it.
func cloneOrder(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == ".":
		return -1
	case b == ".":
		return 1
	}
	return strings.Compare(a, b)
}

// List is the checkouts in clone order.
func (c Checkouts) List() []Checkout {
	return slices.Clone(c.list)
}
