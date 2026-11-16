package infra_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/medilies/weaver/internal/session/infra"
)

// A tmux that never answers, first on PATH: it stands for a server stuck
// or under load, without touching any real one.
func stuckTmux(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nexec sleep 60\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the stuck tmux: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestTmuxGivesUpOnATmuxThatDoesNotAnswer(t *testing.T) {
	stuckTmux(t)
	tmux := infra.NewTmux(infra.TmuxOptions{Limit: func() time.Duration { return 100 * time.Millisecond }})
	name := refAt(t, "/srv/shop", "shop")

	began := time.Now()
	_, _, err := tmux.Find(name)

	if err == nil || !strings.Contains(err.Error(), "no answer within 100ms") {
		t.Fatalf("Find on a stuck tmux = %v, want it to say tmux gave no answer in time", err)
	}
	if waited := time.Since(began); waited > 5*time.Second {
		t.Errorf("Find waited %s for a stuck tmux, want it to give up after its limit", waited)
	}
}
