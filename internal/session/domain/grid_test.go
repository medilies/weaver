package domain_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

// areas names the panes of each row.
func areas(t *testing.T, rows ...[]string) [][]domain.PaneName {
	t.Helper()
	out := make([][]domain.PaneName, len(rows))
	for i, row := range rows {
		for _, raw := range row {
			out[i] = append(out[i], paneName(t, raw))
		}
	}
	return out
}

func tracks(t *testing.T, raw ...string) []domain.Track {
	t.Helper()
	out := make([]domain.Track, len(raw))
	for i, r := range raw {
		track, err := domain.ParseTrack(r)
		if err != nil {
			t.Fatalf("ParseTrack(%q): %v", r, err)
		}
		out[i] = track
	}
	return out
}

func grid(t *testing.T, columns, rows []string, names ...[]string) domain.Grid {
	t.Helper()
	g, err := domain.NewGrid(areas(t, names...), tracks(t, columns...), tracks(t, rows...))
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	return g
}

// named is a pane for each name.
func named(t *testing.T, names ...string) []domain.Pane {
	t.Helper()
	panes := make([]domain.Pane, len(names))
	for i, name := range names {
		panes[i] = domain.NewPane().Named(paneName(t, name))
	}
	return panes
}

// splits writes a window's panes in creation order as "name", then
// "name right of #1 40%" for each later pane.
func splits(window domain.Window) []string {
	panes := window.Panes()
	out := make([]string, len(panes))
	for i, pane := range panes {
		out[i] = pane.Name().String()
		if split, ok := pane.Split(); ok {
			out[i] += fmt.Sprintf(" %s of #%d %s", split.Direction(), split.Of()+1, split.Size())
		}
	}
	return out
}

func TestParseTrack(t *testing.T) {
	for _, raw := range []string{"40%", "100%", "2fr", " 1fr "} {
		t.Run("reads "+raw, func(t *testing.T) {
			track, err := domain.ParseTrack(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, want := track.String(), strings.TrimSpace(raw); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}

	for _, raw := range []string{"", "0%", "101%", "0fr", "1.5fr", "40", "fr"} {
		t.Run("rejects "+`"`+raw+`"`, func(t *testing.T) {
			if _, err := domain.ParseTrack(raw); !errors.Is(err, domain.ErrInvalidTrack) {
				t.Fatalf("got %v, want ErrInvalidTrack", err)
			}
		})
	}
}

func TestNewGrid(t *testing.T) {
	refusal := func(t *testing.T, want error, columns, rows []string, names ...[]string) error {
		t.Helper()
		_, err := domain.NewGrid(areas(t, names...), tracks(t, columns...), tracks(t, rows...))
		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
		return err
	}
	refuses := func(t *testing.T, want error, columns, rows []string, names ...[]string) {
		t.Helper()
		_ = refusal(t, want, columns, rows, names...)
	}

	t.Run("rejects a grid without rows", func(t *testing.T) {
		refuses(t, domain.ErrEmptyGrid, nil, nil)
	})

	t.Run("rejects an empty row", func(t *testing.T) {
		refuses(t, domain.ErrEmptyGrid, nil, nil, []string{"a"}, []string{})
	})

	t.Run("rejects rows of different lengths", func(t *testing.T) {
		refuses(t, domain.ErrRaggedGrid, nil, nil, []string{"a", "b"}, []string{"c"})
	})

	t.Run("rejects more column sizes than columns", func(t *testing.T) {
		refuses(t, domain.ErrTrackCount, []string{"50%", "25%", "25%"}, nil, []string{"a", "b"})
	})

	t.Run("rejects fewer row sizes than rows", func(t *testing.T) {
		refuses(t, domain.ErrTrackCount, nil, []string{"100%"}, []string{"a"}, []string{"b"})
	})

	t.Run("rejects percentages short of the window", func(t *testing.T) {
		refuses(t, domain.ErrTrackShares, []string{"60%", "30%"}, nil, []string{"a", "b"})
	})

	t.Run("rejects percentages leaving nothing to share by fr", func(t *testing.T) {
		refuses(t, domain.ErrTrackShares, []string{"100%", "1fr"}, nil, []string{"a", "b"})
	})

	t.Run("rejects a pane whose cells form an L", func(t *testing.T) {
		refuses(t, domain.ErrPaneNotRectangle, nil, nil, []string{"a", "b"}, []string{"a", "a"})
	})

	t.Run("rejects a pane in two places", func(t *testing.T) {
		refuses(t, domain.ErrPaneNotRectangle, nil, nil, []string{"a", "b", "a"})
	})

	t.Run("rejects panes no straight line separates, naming them", func(t *testing.T) {
		err := refusal(t, domain.ErrInterlockedPanes, nil, nil,
			[]string{"a", "a", "b"},
			[]string{"d", "e", "b"},
			[]string{"d", "c", "c"},
		)
		if want := `"a", "b", "d", "e", "c"`; !strings.Contains(err.Error(), want) {
			t.Fatalf("got %v, want it to name %s", err, want)
		}
	})

	t.Run("does not change when the caller's areas do", func(t *testing.T) {
		names := areas(t, []string{"a", "b"})
		g, err := domain.NewGrid(names, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		names[0][0] = paneName(t, "z")
		if got := g.Areas()[0][0].String(); got != "a" {
			t.Fatalf("got %q, want a", got)
		}
	})
}

func TestNewGridWindow(t *testing.T) {
	makes := func(t *testing.T, g domain.Grid, panes ...domain.Pane) []string {
		t.Helper()
		window, err := domain.NewGridWindow(windowName(t, "w"), g, panes...)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return splits(window)
	}

	t.Run("sizes panes as shares of the window", func(t *testing.T) {
		g := grid(t, []string{"60%", "40%"}, []string{"70%", "30%"},
			[]string{"app shell", "flutter run"},
			[]string{"app shell", "emulator"},
		)
		got := makes(t, g, named(t, "app shell", "flutter run", "emulator")...)
		want := []string{"app shell", "flutter run right of #1 40%", "emulator below of #2 30%"}
		if !slices.Equal(got, want) {
			t.Fatalf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("makes equal columns without sizes", func(t *testing.T) {
		got := makes(t, grid(t, nil, nil, []string{"a", "b", "c"}), named(t, "a", "b", "c")...)
		if want := []string{"a", "b right of #1 67%", "c right of #2 50%"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("splits the same by fr as by percentages", func(t *testing.T) {
		byWeight := makes(t, grid(t, []string{"3fr", "2fr"}, nil, []string{"a", "b"}), named(t, "a", "b")...)
		byPercent := makes(t, grid(t, []string{"60%", "40%"}, nil, []string{"a", "b"}), named(t, "a", "b")...)
		if !slices.Equal(byWeight, byPercent) {
			t.Fatalf("got %q by fr, %q by percentages", byWeight, byPercent)
		}
	})

	t.Run("shares what percentages leave by fr", func(t *testing.T) {
		g := grid(t, []string{"50%", "1fr", "1fr"}, nil, []string{"a", "b", "c"})
		got := makes(t, g, named(t, "a", "b", "c")...)
		if want := []string{"a", "b right of #1 50%", "c right of #2 50%"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("cuts a 2x2 grid into columns first", func(t *testing.T) {
		g := grid(t, nil, nil, []string{"a", "b"}, []string{"c", "d"})
		got := makes(t, g, named(t, "a", "b", "c", "d")...)
		want := []string{"a", "b right of #1 50%", "c below of #1 50%", "d below of #2 50%"}
		if !slices.Equal(got, want) {
			t.Fatalf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("cuts inside a cut", func(t *testing.T) {
		g := grid(t, nil, nil,
			[]string{"a", "b"},
			[]string{"a", "c"},
			[]string{"d", "d"},
		)
		got := makes(t, g, named(t, "a", "b", "c", "d")...)
		want := []string{"a", "d below of #1 33%", "b right of #1 50%", "c below of #3 50%"}
		if !slices.Equal(got, want) {
			t.Fatalf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("takes its panes in any order", func(t *testing.T) {
		g := grid(t, nil, nil, []string{"a", "b"})
		got := makes(t, g, named(t, "b", "a")...)
		if want := []string{"a", "b right of #1 50%"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps its grid", func(t *testing.T) {
		window, err := domain.NewGridWindow(windowName(t, "w"), grid(t, nil, nil, []string{"a"}), named(t, "a")...)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := window.Grid(); !ok {
			t.Fatal("got no grid")
		}
	})

	refusal := func(t *testing.T, want error, g domain.Grid, panes ...domain.Pane) error {
		t.Helper()
		_, err := domain.NewGridWindow(windowName(t, "w"), g, panes...)
		if !errors.Is(err, want) {
			t.Fatalf("got %v, want %v", err, want)
		}
		return err
	}
	refuses := func(t *testing.T, want error, g domain.Grid, panes ...domain.Pane) {
		t.Helper()
		_ = refusal(t, want, g, panes...)
	}

	t.Run("rejects a pane that splits another, naming it", func(t *testing.T) {
		panes := named(t, "a", "flutter run")
		panes[1] = panes[1].Splitting(split(t, 0, domain.Right, ""))
		err := refusal(t, domain.ErrSplitInGrid, grid(t, nil, nil, []string{"a", "flutter run"}), panes...)
		if !strings.Contains(err.Error(), `pane "flutter run"`) {
			t.Fatalf("got %v, want it to name the pane", err)
		}
	})

	t.Run("rejects a pane the grid doesn't place", func(t *testing.T) {
		refuses(t, domain.ErrPaneOutsideGrid, grid(t, nil, nil, []string{"a"}), named(t, "a", "b")...)
	})

	t.Run("rejects an unnamed pane", func(t *testing.T) {
		refuses(t, domain.ErrPaneOutsideGrid, grid(t, nil, nil, []string{"a"}), domain.NewPane())
	})

	t.Run("rejects a grid naming a pane the window lacks", func(t *testing.T) {
		refuses(t, domain.ErrUnknownGridPane, grid(t, nil, nil, []string{"a", "b"}), named(t, "a")...)
	})

	t.Run("rejects the zero grid", func(t *testing.T) {
		refuses(t, domain.ErrEmptyGrid, domain.Grid{}, named(t, "a")...)
	})

	t.Run("rejects two panes with the same name", func(t *testing.T) {
		refuses(t, domain.ErrDuplicatePaneName, grid(t, nil, nil, []string{"a"}), named(t, "a", "a")...)
	})
}

func TestNewWindowHasNoGrid(t *testing.T) {
	if _, ok := window(t, "w").Grid(); ok {
		t.Fatal("got a grid, want none")
	}
}

func TestChangingGridWindowPanes(t *testing.T) {
	gridWindow := func(t *testing.T) domain.Window {
		t.Helper()
		w, err := domain.NewGridWindow(windowName(t, "w"), grid(t, nil, nil, []string{"a", "b"}), named(t, "a", "b")...)
		if err != nil {
			t.Fatalf("NewGridWindow: %v", err)
		}
		return w
	}

	t.Run("replaces a pane keeping its name and split", func(t *testing.T) {
		w := gridWindow(t)
		replaced, err := w.ReplacingPane(1, w.Panes()[1].Running(command(t, "task dev")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, gridded := replaced.Grid(); !gridded {
			t.Fatal("lost its grid")
		}
		if cmd, _ := replaced.Panes()[1].Startup().Command(); cmd.String() != "task dev" {
			t.Fatalf("got command %q, want task dev", cmd)
		}
		if got, want := splits(replaced), splits(w); !slices.Equal(got, want) {
			t.Fatalf("got %q, want the grid's %q", got, want)
		}
	})

	t.Run("rejects renaming a pane", func(t *testing.T) {
		w := gridWindow(t)
		if _, err := w.ReplacingPane(1, w.Panes()[1].Named(paneName(t, "c"))); !errors.Is(err, domain.ErrChangingGridPanes) {
			t.Fatalf("got %v, want ErrChangingGridPanes", err)
		}
	})

	t.Run("rejects re-splitting a pane", func(t *testing.T) {
		w := gridWindow(t)
		if _, err := w.ReplacingPane(1, w.Panes()[1].Splitting(split(t, 0, domain.Below, "50%"))); !errors.Is(err, domain.ErrChangingGridPanes) {
			t.Fatalf("got %v, want ErrChangingGridPanes", err)
		}
	})

	t.Run("rejects adding a pane", func(t *testing.T) {
		if _, err := gridWindow(t).AddingPane(named(t, "c")[0].Splitting(split(t, 0, domain.Below, "50%"))); !errors.Is(err, domain.ErrChangingGridPanes) {
			t.Fatalf("got %v, want ErrChangingGridPanes", err)
		}
	})

	t.Run("rejects removing a pane", func(t *testing.T) {
		if _, err := gridWindow(t).WithoutPane(1); !errors.Is(err, domain.ErrChangingGridPanes) {
			t.Fatalf("got %v, want ErrChangingGridPanes", err)
		}
	})
}
