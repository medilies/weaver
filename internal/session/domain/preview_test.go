package domain_test

import (
	"errors"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestNewWindowPreview(t *testing.T) {
	pane := domain.NewPanePreview(0, 0, 80, 24, "", nil)

	t.Run("holds a window's size and panes", func(t *testing.T) {
		preview, err := domain.NewWindowPreview(80, 24, pane)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if width, height := preview.Size(); width != 80 || height != 24 || len(preview.Panes()) != 1 {
			t.Fatalf("got %dx%d with %d panes, want 80x24 with one", width, height, len(preview.Panes()))
		}
	})

	t.Run("refuses a window without a pane", func(t *testing.T) {
		if _, err := domain.NewWindowPreview(80, 24); !errors.Is(err, domain.ErrEmptyPreview) {
			t.Fatalf("got %v, want ErrEmptyPreview", err)
		}
	})

	t.Run("refuses a window without a size", func(t *testing.T) {
		if _, err := domain.NewWindowPreview(0, 0, pane); !errors.Is(err, domain.ErrEmptyPreview) {
			t.Fatalf("got %v, want ErrEmptyPreview", err)
		}
	})
}
