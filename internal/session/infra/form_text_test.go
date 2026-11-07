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

func TestParseEnv(t *testing.T) {
	t.Run("drops the spaces around names and values", func(t *testing.T) {
		env, err := parseEnv("FOO = bar\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vars := env.Vars(); len(vars) != 1 || vars[0].Name() != "FOO" || vars[0].Value() != "bar" {
			t.Fatalf("got %v, want FOO=bar", vars)
		}
	})

	t.Run("skips blank lines", func(t *testing.T) {
		env, err := parseEnv("\nA=1\n  \nB=2\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vars := env.Vars(); len(vars) != 2 {
			t.Fatalf("got %v, want A and B", vars)
		}
	})

	t.Run("rejects a line without =", func(t *testing.T) {
		if _, err := parseEnv("FOO bar\n"); err == nil {
			t.Fatal("got no error, want the line rejected")
		}
	})

	t.Run("rejects a name set twice, as a config does", func(t *testing.T) {
		if _, err := parseEnv("FOO=1\nFOO = 2\n"); err == nil {
			t.Fatal("got no error, want the second FOO rejected")
		}
	})
}

func TestParseRepos(t *testing.T) {
	t.Run("drops the spaces around directories and remotes", func(t *testing.T) {
		repos, err := parseRepos("back = git@host:back.git\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repos) != 1 || repos[0].Dir() != "back" || repos[0].Remote() != "git@host:back.git" {
			t.Fatalf("got %v, want back = git@host:back.git", repos)
		}
	})

	t.Run("skips blank lines", func(t *testing.T) {
		repos, err := parseRepos("\nback=git@host:back.git\n  \nfront=git@host:front.git\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repos) != 2 {
			t.Fatalf("got %v, want back and front", repos)
		}
	})

	t.Run("rejects a line without =", func(t *testing.T) {
		if _, err := parseRepos("back git@host:back.git\n"); err == nil {
			t.Fatal("got no error, want the line rejected")
		}
	})
}
