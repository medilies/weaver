package infra_test

import (
	"io"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/infra"
)

func TestLineAcknowledger(t *testing.T) {
	t.Run("prints the message and waits for Enter", func(t *testing.T) {
		in, typed := io.Pipe()
		var out strings.Builder
		done := make(chan error, 1)
		go func() { done <- infra.NewLineAcknowledger(in, &out).Acknowledge("teardown: boom") }()

		select {
		case err := <-done:
			t.Fatalf("returned before Enter: %v", err)
		default:
		}
		if _, err := typed.Write([]byte("\n")); err != nil {
			t.Fatalf("typing Enter: %v", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(out.String(), "teardown: boom\n") {
			t.Fatalf("got %q, want the message first", out.String())
		}
	})

	t.Run("returns when there is nothing to read", func(t *testing.T) {
		if err := infra.NewLineAcknowledger(strings.NewReader(""), io.Discard).Acknowledge("teardown: boom"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
