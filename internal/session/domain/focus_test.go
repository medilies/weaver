package domain_test

import (
	"errors"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestWindowFocus(t *testing.T) {
	logs := domain.NewPane().Named(paneName(t, "logs"))
	editor := domain.NewPane().Named(paneName(t, "editor")).Splitting(split(t, 0, domain.Right, "70%"))
	shell := domain.NewPane().Named(paneName(t, "shell")).Splitting(split(t, 0, domain.Below, ""))

	t.Run("selects the first pane unless another is focused", func(t *testing.T) {
		if got := window(t, "edit", logs, editor).FocusedPane(); got != 0 {
			t.Fatalf("got pane #%d, want the first", got+1)
		}
	})

	t.Run("selects the focused pane", func(t *testing.T) {
		if got := window(t, "edit", logs, editor.WithFocus()).FocusedPane(); got != 1 {
			t.Fatalf("got pane #%d, want #2", got+1)
		}
	})

	t.Run("rejects two focused panes", func(t *testing.T) {
		_, err := domain.NewWindow(windowName(t, "edit"), logs.WithFocus(), editor.WithFocus())
		if !errors.Is(err, domain.ErrFocusedPaneTwice) {
			t.Fatalf("got %v, want ErrFocusedPaneTwice", err)
		}
	})

	t.Run("moves its focus from one pane to another", func(t *testing.T) {
		moved, err := window(t, "edit", logs, editor.WithFocus(), shell).Focusing(2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := moved.FocusedPane(); got != 2 {
			t.Fatalf("got pane #%d, want #3", got+1)
		}
	})

	t.Run("goes back to its first pane when the focused one is removed", func(t *testing.T) {
		removed, err := window(t, "edit", logs, editor.WithFocus(), shell).WithoutPane(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := removed.FocusedPane(); got != 0 {
			t.Fatalf("got pane #%d, want the first", got+1)
		}
	})

	t.Run("rejects focusing a pane it doesn't have", func(t *testing.T) {
		if _, err := window(t, "edit", logs, editor).Focusing(2); !errors.Is(err, domain.ErrPaneNotFound) {
			t.Fatalf("got %v, want ErrPaneNotFound", err)
		}
	})
}

func TestProjectFocus(t *testing.T) {
	project := domain.NewProject(projectName(t, "shop"), projectRoot(t, "/srv/shop"))

	t.Run("lands in the first window unless another is focused", func(t *testing.T) {
		withWindows, err := project.WithWindows(window(t, "shell"), window(t, "editor"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := withWindows.FocusedWindow(); got != 0 {
			t.Fatalf("got window #%d, want the first", got+1)
		}
	})

	t.Run("lands in the focused window", func(t *testing.T) {
		withWindows, err := project.WithWindows(window(t, "shell"), window(t, "editor").WithFocus())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := withWindows.FocusedWindow(); got != 1 {
			t.Fatalf("got window #%d, want #2", got+1)
		}
	})

	t.Run("rejects two focused windows", func(t *testing.T) {
		_, err := project.WithWindows(window(t, "shell").WithFocus(), window(t, "editor").WithFocus())
		if !errors.Is(err, domain.ErrFocusedWindowTwice) {
			t.Fatalf("got %v, want ErrFocusedWindowTwice", err)
		}
	})

	t.Run("moves its focus from one window to another", func(t *testing.T) {
		withWindows, err := project.WithWindows(window(t, "shell"), window(t, "editor").WithFocus(), window(t, "logs"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		moved, err := withWindows.Focusing(2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := moved.FocusedWindow(); got != 2 {
			t.Fatalf("got window #%d, want #3", got+1)
		}
	})

	t.Run("keeps landing in whichever window is first once focus goes back to the first", func(t *testing.T) {
		withWindows, err := project.WithWindows(window(t, "shell"), window(t, "editor").WithFocus())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		unfocused, err := withWindows.Focusing(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		moved, err := unfocused.MovingWindow(0, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := moved.FocusedWindow(); got != 0 {
			t.Fatalf("got window #%d, want the first", got+1)
		}
	})

	t.Run("plans the focused window and each window's focused pane", func(t *testing.T) {
		logs := domain.NewPane().Named(paneName(t, "logs"))
		editor := domain.NewPane().Named(paneName(t, "editor")).Splitting(split(t, 0, domain.Right, "")).WithFocus()
		withWindows, err := project.WithWindows(window(t, "shell"), window(t, "edit", logs, editor).WithFocus())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		plan := withWindows.Plan()
		if got := plan.FocusedWindow(); got != 1 {
			t.Fatalf("got window #%d, want #2", got+1)
		}
		if got := plan.Windows()[1].FocusedPane(); got != 1 {
			t.Fatalf("got pane #%d, want #2", got+1)
		}
	})
}
