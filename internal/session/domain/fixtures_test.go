package domain_test

import (
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

// The helpers below build valid fixtures and fail the test if a
// constructor rejects them, so each test reads as the behaviour it checks.

func projectName(t *testing.T, raw string) domain.ProjectName {
	t.Helper()
	name, err := domain.NewProjectName(raw)
	if err != nil {
		t.Fatalf("NewProjectName(%q): %v", raw, err)
	}
	return name
}

func projectRoot(t *testing.T, raw string) domain.ProjectRoot {
	t.Helper()
	root, err := domain.NewProjectRoot(raw)
	if err != nil {
		t.Fatalf("NewProjectRoot(%q): %v", raw, err)
	}
	return root
}

func windowName(t *testing.T, raw string) domain.WindowName {
	t.Helper()
	name, err := domain.NewWindowName(raw)
	if err != nil {
		t.Fatalf("NewWindowName(%q): %v", raw, err)
	}
	return name
}

func paneName(t *testing.T, raw string) domain.PaneName {
	t.Helper()
	name, err := domain.NewPaneName(raw)
	if err != nil {
		t.Fatalf("NewPaneName(%q): %v", raw, err)
	}
	return name
}

func paneDir(t *testing.T, raw string) domain.PaneDir {
	t.Helper()
	dir, err := domain.NewPaneDir(raw)
	if err != nil {
		t.Fatalf("NewPaneDir(%q): %v", raw, err)
	}
	return dir
}

func split(t *testing.T, of int, direction domain.Direction, rawSize string) domain.Split {
	t.Helper()
	size, err := domain.ParseSize(rawSize)
	if err != nil {
		t.Fatalf("ParseSize(%q): %v", rawSize, err)
	}
	s, err := domain.NewSplit(of, direction, size)
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}
	return s
}

func command(t *testing.T, raw string) domain.Command {
	t.Helper()
	cmd, err := domain.NewCommand(raw)
	if err != nil {
		t.Fatalf("NewCommand(%q): %v", raw, err)
	}
	return cmd
}

func window(t *testing.T, name string, panes ...domain.Pane) domain.Window {
	t.Helper()
	w, err := domain.NewWindow(windowName(t, name), panes...)
	if err != nil {
		t.Fatalf("NewWindow(%q): %v", name, err)
	}
	return w
}
