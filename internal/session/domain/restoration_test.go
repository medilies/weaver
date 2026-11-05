package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

// windowNames builds window names, failing the test on an invalid one.
func windowNames(t *testing.T, raw ...string) []domain.WindowName {
	t.Helper()
	names := make([]domain.WindowName, 0, len(raw))
	for _, r := range raw {
		names = append(names, windowName(t, r))
	}
	return names
}

// describe renders placements as "b after a", "a before b" or
// "a at the end".
func describe(placements []domain.WindowPlacement) []string {
	var out []string
	for _, p := range placements {
		name := p.Window().Name().String()
		if after, ok := p.After(); ok {
			out = append(out, name+" after "+after.String())
		} else if before, ok := p.Before(); ok {
			out = append(out, name+" before "+before.String())
		} else {
			out = append(out, name+" at the end")
		}
	}
	return out
}

func TestProjectRestoration(t *testing.T) {
	project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
		WithWindows(window(t, "a"), window(t, "b"), window(t, "c"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]struct {
		present []string
		want    []string
	}{
		"plans nothing when no window is missing":              {[]string{"a", "b", "c"}, nil},
		"puts a missing window after the one listed before it": {[]string{"a", "c"}, []string{"b after a"}},
		"puts a missing first window before the first left":    {[]string{"b", "c"}, []string{"a before b"}},
		"chains missing windows that follow each other":        {[]string{"c"}, []string{"a before c", "b after a"}},
		"appends when no project window is left":               {nil, []string{"a at the end", "b after a", "c after b"}},
		"ignores windows the project doesn't list":             {[]string{"a", "manual", "c"}, []string{"b after a"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			placements, err := project.Restoration(windowNames(t, c.present...), installed{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := describe(placements); !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestProjectRestorationResolvesOnlyMissingWindows(t *testing.T) {
	pinned := domain.NewPane().WithRuntimes(runtimes(t, "php", "8.4"))
	project, err := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop")).
		WithWindows(window(t, "back", pinned), window(t, "repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("ignores an uninstalled version in a window still there", func(t *testing.T) {
		if _, err := project.Restoration(windowNames(t, "back"), installed{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("fails on an uninstalled version in a missing window", func(t *testing.T) {
		_, err := project.Restoration(windowNames(t, "repo"), installed{})
		if !errors.Is(err, domain.ErrRuntimeNotInstalled) {
			t.Fatalf("got %v, want ErrRuntimeNotInstalled", err)
		}
	})
}
