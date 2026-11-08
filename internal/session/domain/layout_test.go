package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestNewWindowName(t *testing.T) {
	t.Run("rejects blank names", func(t *testing.T) {
		_, err := domain.NewWindowName("  ")
		if !errors.Is(err, domain.ErrEmptyWindowName) {
			t.Fatalf("got %v, want ErrEmptyWindowName", err)
		}
	})

	t.Run("rejects control characters", func(t *testing.T) {
		_, err := domain.NewWindowName("api\tshell")
		if !errors.Is(err, domain.ErrControlCharacters) {
			t.Fatalf("got %v, want ErrControlCharacters", err)
		}
	})
}

func TestDefaultPaneName(t *testing.T) {
	if got := domain.DefaultPaneName(1).String(); got != "pane 2" {
		t.Fatalf("got %q, want %q", got, "pane 2")
	}
}

func TestNamePanes(t *testing.T) {
	unnamed := domain.PaneName{}

	t.Run("leaves a single pane unnamed", func(t *testing.T) {
		names, err := domain.NamePanes(unnamed)
		if err != nil || len(names) != 1 || !names[0].IsZero() {
			t.Fatalf("got %v, %v, want one unnamed pane", names, err)
		}
	})

	t.Run("gives the unnamed panes of several their defaults", func(t *testing.T) {
		names, err := domain.NamePanes(unnamed, paneName(t, "logs"), unnamed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got []string
		for _, name := range names {
			got = append(got, name.String())
		}
		if want := []string{"pane 1", "logs", "pane 3"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("rejects a name given twice", func(t *testing.T) {
		if _, err := domain.NamePanes(paneName(t, "logs"), paneName(t, "logs")); !errors.Is(err, domain.ErrDuplicatePaneName) {
			t.Fatalf("got %v, want ErrDuplicatePaneName", err)
		}
	})

	t.Run("rejects a default taken by a given name", func(t *testing.T) {
		if _, err := domain.NamePanes(paneName(t, "pane 2"), unnamed); !errors.Is(err, domain.ErrDuplicatePaneName) {
			t.Fatalf("got %v, want ErrDuplicatePaneName", err)
		}
	})
}

func TestNewPaneName(t *testing.T) {
	t.Run("rejects blank names", func(t *testing.T) {
		_, err := domain.NewPaneName("")
		if !errors.Is(err, domain.ErrEmptyPaneName) {
			t.Fatalf("got %v, want ErrEmptyPaneName", err)
		}
	})
}

func TestNewPaneDir(t *testing.T) {
	for _, raw := range []string{"", "  ", "."} {
		t.Run("treats "+`"`+raw+`"`+" as the project root", func(t *testing.T) {
			dir, err := domain.NewPaneDir(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !dir.IsRoot() {
				t.Fatalf("got %q, want the project root", dir.String())
			}
		})
	}

	t.Run("cleans relative paths", func(t *testing.T) {
		dir, err := domain.NewPaneDir("api/../app/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := dir.String(); got != "app" {
			t.Fatalf("got %q, want %q", got, "app")
		}
	})
}

func TestParseDirection(t *testing.T) {
	for raw, want := range map[string]domain.Direction{"right": domain.Right, "below": domain.Below} {
		t.Run("reads "+raw, func(t *testing.T) {
			got, err := domain.ParseDirection(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}

	t.Run("rejects other directions", func(t *testing.T) {
		_, err := domain.ParseDirection("left")
		if !errors.Is(err, domain.ErrInvalidDirection) {
			t.Fatalf("got %v, want ErrInvalidDirection", err)
		}
	})
}

func TestParseSize(t *testing.T) {
	for raw, want := range map[string]string{"40%": "40%", "20": "20", " 30% ": "30%"} {
		t.Run("reads "+raw, func(t *testing.T) {
			size, err := domain.ParseSize(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := size.String(); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}

	t.Run("treats blank as the default size", func(t *testing.T) {
		size, err := domain.ParseSize("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !size.IsDefault() {
			t.Fatalf("got %q, want the default size", size.String())
		}
	})

	for _, raw := range []string{"0%", "100%", "0", "-3", "half", "40 %"} {
		t.Run("rejects "+raw, func(t *testing.T) {
			_, err := domain.ParseSize(raw)
			if !errors.Is(err, domain.ErrInvalidSize) {
				t.Fatalf("got %v, want ErrInvalidSize", err)
			}
		})
	}
}

func TestNewCommand(t *testing.T) {
	t.Run("trims surrounding whitespace", func(t *testing.T) {
		if got := command(t, "  task dev ").String(); got != "task dev" {
			t.Fatalf("got %q, want %q", got, "task dev")
		}
	})

	t.Run("rejects blank commands", func(t *testing.T) {
		_, err := domain.NewCommand(" ")
		if !errors.Is(err, domain.ErrEmptyCommand) {
			t.Fatalf("got %v, want ErrEmptyCommand", err)
		}
	})

	for reason, raw := range map[string]string{"newlines": "npm i\nnpm run dev", "tabs": "git\tstatus"} {
		t.Run("rejects "+reason, func(t *testing.T) {
			_, err := domain.NewCommand(raw)
			if !errors.Is(err, domain.ErrControlCharacters) {
				t.Fatalf("got %v, want ErrControlCharacters", err)
			}
		})
	}
}

func TestCommandProgram(t *testing.T) {
	for raw, want := range map[string]string{
		"npm run dev":              "npm",
		"NODE_ENV=dev npm run dev": "npm",
		"./bin/dev --watch":        "./bin/dev",
	} {
		t.Run("finds "+want+" in "+raw, func(t *testing.T) {
			if got, ok := command(t, raw).Program(); !ok || got != want {
				t.Fatalf("got %q (%v), want %q", got, ok, want)
			}
		})
	}

	for reason, raw := range map[string]string{
		"a builtin":           "cd back",
		"a keyword":           "while true; do date; done",
		"an expansion":        "$EDITOR .",
		"a home path":         "~/bin/dev",
		"a subshell":          "(cd back && make)",
		"only an assignment":  "DEBUG=1",
		"a quoted executable": `"my tool" --flag`,
		"an escaped value":    `FOO=a\ b npm start`,
		"a limit builtin":     "ulimit -n 4096 && ./server",
		"a zsh builtin":       "unsetopt correct",
		"a comment":           "# run migrations",
	} {
		t.Run("leaves "+reason+" to the shell", func(t *testing.T) {
			if got, ok := command(t, raw).Program(); ok {
				t.Fatalf("got %q, want no program", got)
			}
		})
	}
}

func TestPaneStartup(t *testing.T) {
	t.Run("a new pane types nothing", func(t *testing.T) {
		if _, ok := domain.NewPane().Startup().Command(); ok {
			t.Fatal("got a command, want none")
		}
	})

	t.Run("a running pane submits its command", func(t *testing.T) {
		startup := domain.NewPane().Running(command(t, "task dev")).Startup()
		if !startup.Submits() {
			t.Fatal("got a prefilled command, want it submitted")
		}
	})

	t.Run("a prefilling pane leaves its command at the prompt", func(t *testing.T) {
		startup := domain.NewPane().Prefilling(command(t, "nimbus emulator")).Startup()
		if startup.Submits() {
			t.Fatal("got a submitted command, want it left at the prompt")
		}
	})

	t.Run("the last of running and prefilling wins", func(t *testing.T) {
		pane := domain.NewPane().Running(command(t, "task dev")).Prefilling(command(t, "nimbus emulator"))
		cmd, _ := pane.Startup().Command()
		if got := cmd.String(); got != "nimbus emulator" {
			t.Fatalf("got %q, want %q", got, "nimbus emulator")
		}
		if pane.Startup().Submits() {
			t.Fatal("got the command run, want it only prefilled")
		}
	})

	t.Run("a pane without its command types nothing", func(t *testing.T) {
		pane := domain.NewPane().Running(command(t, "task dev")).WithoutCommand()
		if _, ok := pane.Startup().Command(); ok {
			t.Fatal("got a command, want none")
		}
	})
}

func TestPaneNote(t *testing.T) {
	t.Run("does not change when the caller's lines do", func(t *testing.T) {
		lines := []string{"Usual commands:", "  npm i"}
		pane := domain.NewPane().WithNote(lines...)
		lines[1] = "  rm -rf /"

		if got := pane.Note(); !slices.Equal(got, []string{"Usual commands:", "  npm i"}) {
			t.Fatalf("got %q, want the original lines", got)
		}
	})
}

func TestWindowNamed(t *testing.T) {
	renamed := window(t, "api", domain.NewPane().Named(paneName(t, "shell"))).
		RunningFirst(command(t, "nvm use 20")).Named(windowName(t, "backend"))

	t.Run("takes the new name", func(t *testing.T) {
		if got := renamed.Name().String(); got != "backend" {
			t.Fatalf("got %q, want backend", got)
		}
	})

	t.Run("keeps its panes and before commands", func(t *testing.T) {
		if len(renamed.Panes()) != 1 || len(renamed.Before()) != 1 {
			t.Fatalf("got %d panes and %d before commands, want 1 and 1", len(renamed.Panes()), len(renamed.Before()))
		}
	})
}

func TestWindowRebased(t *testing.T) {
	for _, c := range []struct{ name, dir, want string }{
		{"moves a directory under from under to", "/srv/shop/api", "/home/me/shop/api"},
		{"moves from itself to to", "/srv/shop", "/home/me/shop"},
		{"keeps a directory elsewhere", "/srv/shopping", "/srv/shopping"},
		{"keeps a relative directory", "api", "api"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, err := domain.NewPaneDir(c.dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			w := window(t, "main", domain.NewPane().In(dir))
			if got, _ := w.Rebased("/srv/shop", "/home/me/shop").Panes()[0].Dir(); got.String() != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}

	t.Run("moves the window's dir", func(t *testing.T) {
		w := window(t, "main").In(paneDir(t, "/srv/shop/api"))
		if got := w.Rebased("/srv/shop", "/home/me/shop").Dir().String(); got != "/home/me/shop/api" {
			t.Fatalf("got %q, want /home/me/shop/api", got)
		}
	})

	t.Run("keeps the window's own commands", func(t *testing.T) {
		cmd, _ := domain.NewCommand("nvm use")
		w := window(t, "main").RunningFirst(cmd)
		if got := w.Rebased("/srv/shop", "/home/me/shop").Before(); len(got) != 1 || got[0].String() != "nvm use" {
			t.Fatalf("got %v, want nvm use", got)
		}
	})
}

func TestNewWindow(t *testing.T) {
	apiShell := domain.NewPane().Named(paneName(t, "api shell"))
	taskDev := domain.NewPane().Named(paneName(t, "task dev")).Splitting(split(t, 0, domain.Right, "40%"))

	t.Run("holds one shell in its dir when given no panes", func(t *testing.T) {
		panes := window(t, "repo").Panes()
		if len(panes) != 1 {
			t.Fatalf("got %d panes, want one", len(panes))
		}
		if _, set := panes[0].Dir(); set {
			t.Fatal("got a pane with its own dir, want one starting in the window's")
		}
	})

	t.Run("keeps its panes in creation order", func(t *testing.T) {
		psql := domain.NewPane().Named(paneName(t, "psql")).Splitting(split(t, 1, domain.Below, ""))
		var names []string
		for _, pane := range window(t, "api", apiShell, taskDev, psql).Panes() {
			names = append(names, pane.Name().String())
		}
		if want := []string{"api shell", "task dev", "psql"}; !slices.Equal(names, want) {
			t.Fatalf("got %q, want %q", names, want)
		}
	})

	t.Run("rejects a first pane that splits", func(t *testing.T) {
		_, err := domain.NewWindow(windowName(t, "api"), taskDev)
		if !errors.Is(err, domain.ErrFirstPaneSplits) {
			t.Fatalf("got %v, want ErrFirstPaneSplits", err)
		}
	})

	t.Run("rejects a later pane that doesn't split", func(t *testing.T) {
		_, err := domain.NewWindow(windowName(t, "api"), apiShell, domain.NewPane().Named(paneName(t, "psql")))
		if !errors.Is(err, domain.ErrPaneWithoutSplit) {
			t.Fatalf("got %v, want ErrPaneWithoutSplit", err)
		}
	})

	t.Run("rejects a pane that splits itself", func(t *testing.T) {
		self := domain.NewPane().Named(paneName(t, "psql")).Splitting(split(t, 1, domain.Below, ""))
		_, err := domain.NewWindow(windowName(t, "api"), apiShell, self)
		if !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("rejects a pane that splits a later pane", func(t *testing.T) {
		early := domain.NewPane().Named(paneName(t, "psql")).Splitting(split(t, 2, domain.Below, ""))
		_, err := domain.NewWindow(windowName(t, "api"), apiShell, early, taskDev)
		if !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("accepts an unnamed single pane", func(t *testing.T) {
		if _, err := domain.NewWindow(windowName(t, "repo"), domain.NewPane()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects an unnamed pane among several", func(t *testing.T) {
		unnamed := domain.NewPane().Splitting(split(t, 0, domain.Right, ""))
		_, err := domain.NewWindow(windowName(t, "api"), apiShell, unnamed)
		if !errors.Is(err, domain.ErrUnnamedPane) {
			t.Fatalf("got %v, want ErrUnnamedPane", err)
		}
	})

	t.Run("rejects two panes with the same name", func(t *testing.T) {
		twin := domain.NewPane().Named(paneName(t, "api shell")).Splitting(split(t, 0, domain.Right, ""))
		_, err := domain.NewWindow(windowName(t, "api"), apiShell, twin)
		if !errors.Is(err, domain.ErrDuplicatePaneName) {
			t.Fatalf("got %v, want ErrDuplicatePaneName", err)
		}
	})
}

func TestNewSplit(t *testing.T) {
	t.Run("rejects an unknown direction", func(t *testing.T) {
		_, err := domain.NewSplit(0, domain.Direction(7), domain.DefaultSize())
		if !errors.Is(err, domain.ErrInvalidDirection) {
			t.Fatalf("got %v, want ErrInvalidDirection", err)
		}
	})
}

// shop is a window in front/: back fills it, front splits back right,
// logs splits front below and shell splits back below.
func shop(t *testing.T) domain.Window {
	t.Helper()
	return window(t, "shop",
		domain.NewPane().Named(paneName(t, "back")),
		domain.NewPane().Named(paneName(t, "front")).Splitting(split(t, 0, domain.Right, "30%")).In(paneDir(t, "front")),
		domain.NewPane().Named(paneName(t, "logs")).Splitting(split(t, 1, domain.Below, "40%")),
		domain.NewPane().Named(paneName(t, "shell")).Splitting(split(t, 0, domain.Below, "20%")),
	).In(paneDir(t, "shop")).RunningFirst(command(t, "nvm use"))
}

func TestWindowAddingPane(t *testing.T) {
	t.Run("adds the pane last, splitting the pane it names", func(t *testing.T) {
		added, err := shop(t).AddingPane(domain.NewPane().Named(paneName(t, "db")).Splitting(split(t, 2, domain.Right, "50%")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := splits(added); got[4] != "db right of #3 50%" {
			t.Fatalf("got %q, want db right of #3 50%%", got)
		}
	})

	t.Run("names the panes of a window that had a single unnamed one", func(t *testing.T) {
		added, err := window(t, "w").AddingPane(domain.NewPane().Splitting(split(t, 0, domain.Right, "50%")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := splits(added); !slices.Equal(got, []string{"pane 1", "pane 2 right of #1 50%"}) {
			t.Fatalf("got %q, want the default names", got)
		}
	})

	t.Run("names an unnamed pane by the first default no pane has", func(t *testing.T) {
		w := window(t, "w",
			domain.NewPane().Named(paneName(t, "pane 1")),
			domain.NewPane().Named(paneName(t, "pane 2")).Splitting(split(t, 0, domain.Right, "50%")),
		)
		removed, err := w.WithoutPane(0)
		if err != nil {
			t.Fatalf("WithoutPane: %v", err)
		}
		added, err := removed.AddingPane(domain.NewPane().Splitting(split(t, 0, domain.Below, "50%")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := splits(added); !slices.Equal(got, []string{"pane 2", "pane 1 below of #1 50%"}) {
			t.Fatalf("got %q, want the free default pane 1", got)
		}
	})

	t.Run("keeps the window's dir and before commands", func(t *testing.T) {
		added, err := shop(t).AddingPane(domain.NewPane().Named(paneName(t, "db")).Splitting(split(t, 0, domain.Right, "50%")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if added.Dir().String() != "shop" || len(added.Before()) != 1 {
			t.Fatalf("got dir %q and %d before commands, want shop and 1", added.Dir(), len(added.Before()))
		}
	})

	t.Run("rejects a pane splitting one the window doesn't have", func(t *testing.T) {
		_, err := shop(t).AddingPane(domain.NewPane().Named(paneName(t, "db")).Splitting(split(t, 4, domain.Right, "50%")))
		if !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("rejects a pane named as another", func(t *testing.T) {
		_, err := shop(t).AddingPane(domain.NewPane().Named(paneName(t, "logs")).Splitting(split(t, 0, domain.Right, "50%")))
		if !errors.Is(err, domain.ErrDuplicatePaneName) {
			t.Fatalf("got %v, want ErrDuplicatePaneName", err)
		}
	})
}

func TestWindowReplacingPane(t *testing.T) {
	t.Run("re-splits the pane", func(t *testing.T) {
		pane := shop(t).Panes()[2].Splitting(split(t, 0, domain.Right, "25%"))
		replaced, err := shop(t).ReplacingPane(2, pane)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := splits(replaced)[2]; got != "logs right of #1 25%" {
			t.Fatalf("got %q, want logs right of #1 25%%", got)
		}
	})

	t.Run("names a pane whose name is cleared by the first default no pane has", func(t *testing.T) {
		replaced, err := shop(t).ReplacingPane(2, shop(t).Panes()[2].Named(domain.PaneName{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := replaced.Panes()[2].Name().String(); got != "pane 1" {
			t.Fatalf("got %q, want pane 1", got)
		}
	})

	t.Run("rejects a pane splitting a later one", func(t *testing.T) {
		pane := shop(t).Panes()[1].Splitting(split(t, 2, domain.Right, "50%"))
		if _, err := shop(t).ReplacingPane(1, pane); !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("rejects a pane the window doesn't have", func(t *testing.T) {
		if _, err := shop(t).ReplacingPane(4, domain.NewPane()); !errors.Is(err, domain.ErrPaneNotFound) {
			t.Fatalf("got %v, want ErrPaneNotFound", err)
		}
	})
}

func TestWindowWithoutPane(t *testing.T) {
	t.Run("gives a removed pane's splits to the pane it split", func(t *testing.T) {
		removed, err := shop(t).WithoutPane(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"back", "logs below of #1 40%", "shell below of #1 20%"}
		if got := splits(removed); !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("fills the window with the second pane when the first goes", func(t *testing.T) {
		removed, err := shop(t).WithoutPane(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"front", "logs below of #1 40%", "shell below of #1 20%"}
		if got := splits(removed); !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps the dir the second pane sets when it fills the window", func(t *testing.T) {
		removed, err := shop(t).WithoutPane(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dir, set := removed.Panes()[0].Dir(); !set || dir.String() != "front" {
			t.Fatalf("got dir %q (set: %v), want front set on the pane", dir, set)
		}
	})

	t.Run("keeps the window's dir", func(t *testing.T) {
		removed, err := shop(t).WithoutPane(3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if removed.Dir().String() != "shop" {
			t.Fatalf("got dir %q, want shop", removed.Dir())
		}
	})

	t.Run("rejects removing a window's only pane", func(t *testing.T) {
		if _, err := window(t, "w").WithoutPane(0); !errors.Is(err, domain.ErrLastPane) {
			t.Fatalf("got %v, want ErrLastPane", err)
		}
	})

	t.Run("rejects a pane the window doesn't have", func(t *testing.T) {
		if _, err := shop(t).WithoutPane(-1); !errors.Is(err, domain.ErrPaneNotFound) {
			t.Fatalf("got %v, want ErrPaneNotFound", err)
		}
	})
}

func TestWindowMovingPane(t *testing.T) {
	t.Run("keeps each split on the pane it splits", func(t *testing.T) {
		moved, err := shop(t).MovingPane(3, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"back", "shell below of #1 20%", "front right of #1 30%", "logs below of #3 40%"}
		if got := splits(moved); !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("rejects moving a pane before the pane it splits", func(t *testing.T) {
		if _, err := shop(t).MovingPane(2, 1); !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("keeps the dir a moved pane sets", func(t *testing.T) {
		moved, err := shop(t).MovingPane(3, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dir, set := moved.Panes()[2].Dir(); !set || dir.String() != "front" {
			t.Fatalf("got dir %q (set: %v), want front set on the pane", dir, set)
		}
	})

	t.Run("rejects moving a pane after a pane splitting it", func(t *testing.T) {
		if _, err := shop(t).MovingPane(1, 3); !errors.Is(err, domain.ErrSplitTargetNotEarlier) {
			t.Fatalf("got %v, want ErrSplitTargetNotEarlier", err)
		}
	})

	t.Run("rejects moving a pane first, where nothing is there to split", func(t *testing.T) {
		if _, err := shop(t).MovingPane(1, 0); !errors.Is(err, domain.ErrFirstPaneSplits) {
			t.Fatalf("got %v, want ErrFirstPaneSplits", err)
		}
	})

	t.Run("rejects moving to a place the window doesn't have", func(t *testing.T) {
		if _, err := shop(t).MovingPane(1, 4); !errors.Is(err, domain.ErrPaneNotFound) {
			t.Fatalf("got %v, want ErrPaneNotFound", err)
		}
	})
}
