package infra

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func TestParseRoot(t *testing.T) {
	t.Run("expands a leading ~ as configs do", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home directory: %v", err)
		}
		root, err := parseRoot("~/dev/shop")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join(home, "dev/shop"); root.String() != want {
			t.Fatalf("got %q, want %q", root, want)
		}
	})

	t.Run("rejects a relative root", func(t *testing.T) {
		if _, err := parseRoot("dev/shop"); !errors.Is(err, domain.ErrRelativeProjectRoot) {
			t.Fatalf("got %v, want ErrRelativeProjectRoot", err)
		}
	})
}
