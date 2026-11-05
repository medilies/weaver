package infra

import (
	"testing"
)

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
}

func TestParseCheckouts(t *testing.T) {
	t.Run("drops the spaces around directories and remotes", func(t *testing.T) {
		checkouts, err := parseCheckouts("back = git@host:back.git\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(checkouts) != 1 || checkouts[0].Dir() != "back" || checkouts[0].Remote() != "git@host:back.git" {
			t.Fatalf("got %v, want back = git@host:back.git", checkouts)
		}
	})

	t.Run("skips blank lines", func(t *testing.T) {
		checkouts, err := parseCheckouts("\nback=git@host:back.git\n  \nfront=git@host:front.git\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(checkouts) != 2 {
			t.Fatalf("got %v, want back and front", checkouts)
		}
	})

	t.Run("rejects a line without =", func(t *testing.T) {
		if _, err := parseCheckouts("back git@host:back.git\n"); err == nil {
			t.Fatal("got no error, want the line rejected")
		}
	})
}
