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
		startup := domain.NewPane().Prefilling(command(t, "bicyclette emulator")).Startup()
		if startup.Submits() {
			t.Fatal("got a submitted command, want it left at the prompt")
		}
	})

	t.Run("the last of running and prefilling wins", func(t *testing.T) {
		pane := domain.NewPane().Running(command(t, "task dev")).Prefilling(command(t, "bicyclette emulator"))
		cmd, _ := pane.Startup().Command()
		if got := cmd.String(); got != "bicyclette emulator" {
			t.Fatalf("got %q, want %q", got, "bicyclette emulator")
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

func TestNewWindow(t *testing.T) {
	apiShell := domain.NewPane().Named(paneName(t, "api shell"))
	taskDev := domain.NewPane().Named(paneName(t, "task dev")).Splitting(split(t, 0, domain.Right, "40%"))

	t.Run("holds one shell in the project root when given no panes", func(t *testing.T) {
		panes := window(t, "repo").Panes()
		if len(panes) != 1 || !panes[0].Dir().IsRoot() {
			t.Fatalf("got %d panes, want one shell in the root", len(panes))
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
