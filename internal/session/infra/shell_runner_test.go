package infra_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

func TestShellRunner(t *testing.T) {
	// run runs raw in dir with env, returning what it printed and its error.
	run := func(t *testing.T, raw, dir string, env map[string]string) (string, string, error) {
		t.Helper()
		cmd, err := domain.NewCommand(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		vars, err := domain.NewEnv(env)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var stdout, stderr bytes.Buffer
		err = infra.NewShellRunner(strings.NewReader(""), &stdout, &stderr).Run(cmd, dir, vars)
		return stdout.String(), stderr.String(), err
	}

	t.Run("runs the command in the directory with the env added", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("WEAVER_TEST_INHERITED", "kept")
		out, _, err := run(t, `pwd && echo "$APP_ENV $WEAVER_TEST_INHERITED"`, dir, map[string]string{"APP_ENV": "local"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := real + "\nlocal kept\n"; out != want {
			t.Fatalf("got %q, want %q", out, want)
		}
	})

	t.Run("runs the command with sh, so shell syntax works", func(t *testing.T) {
		dir := t.TempDir()
		if _, _, err := run(t, "touch made && test -f made", dir, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "made")); err != nil {
			t.Fatalf("got %v, want the file made in the directory", err)
		}
	})

	t.Run("reports a failing command with its last error lines", func(t *testing.T) {
		_, stderr, err := run(t, `printf 'one\ntwo\n\nthree\n\033[31mfour\033[0m\n' >&2; exit 3`, t.TempDir(), nil)
		if !errors.Is(err, domain.ErrCommandFailed) || !strings.HasSuffix(err.Error(), "(exit status 3): two / three / four") {
			t.Fatalf("got %v, want the exit status and the last three lines", err)
		}
		if !strings.Contains(stderr, "one\ntwo") {
			t.Fatalf("got stderr %q, want everything printed", stderr)
		}
	})

	t.Run("reports a failing command that printed nothing", func(t *testing.T) {
		_, _, err := run(t, "false", t.TempDir(), nil)
		if !errors.Is(err, domain.ErrCommandFailed) || err.Error() != "command failed (exit status 1)" {
			t.Fatalf("got %v, want the exit status alone", err)
		}
	})

	t.Run("keeps only what a line was redrawn to", func(t *testing.T) {
		_, _, err := run(t, `printf 'pulling 10%%\rpulling 100%%\rpull failed\n' >&2; exit 1`, t.TempDir(), nil)
		if err == nil || !strings.HasSuffix(err.Error(), ": pull failed") {
			t.Fatalf("got %v, want the line as last redrawn", err)
		}
	})
}
