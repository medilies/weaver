package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func env(t *testing.T, vars map[string]string) domain.Env {
	t.Helper()
	e, err := domain.NewEnv(vars)
	if err != nil {
		t.Fatalf("NewEnv(%v): %v", vars, err)
	}
	return e
}

// render flattens env vars to NAME=value, in order.
func render(vars []domain.EnvVar) []string {
	var out []string
	for _, v := range vars {
		out = append(out, v.Name()+"="+v.Value())
	}
	return out
}

func TestNewEnv(t *testing.T) {
	for _, name := range []string{"", "1A", "APP-ENV", "APP ENV"} {
		t.Run("rejects the name "+`"`+name+`"`, func(t *testing.T) {
			_, err := domain.NewEnv(map[string]string{name: "x"})
			if !errors.Is(err, domain.ErrInvalidEnvName) {
				t.Fatalf("got %v, want ErrInvalidEnvName", err)
			}
		})
	}

	t.Run("lists variables sorted by name, with literal values", func(t *testing.T) {
		got := render(env(t, map[string]string{"B": "$HOME", "A": "1"}).Vars())
		if want := []string{"A=1", "B=$HOME"}; !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestEnvOverlay(t *testing.T) {
	project := env(t, map[string]string{"APP_ENV": "local", "DEBUG": "0"})
	pane := env(t, map[string]string{"DEBUG": "1"})

	got := render(project.Overlay(pane).Vars())
	if want := []string{"APP_ENV=local", "DEBUG=1"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
