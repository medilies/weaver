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

func runtimes(t *testing.T, pins ...string) domain.Runtimes {
	t.Helper()
	var list []domain.Runtime
	for i := 0; i+1 < len(pins); i += 2 {
		rt, err := domain.NewRuntime(pins[i], pins[i+1])
		if err != nil {
			t.Fatalf("NewRuntime(%q, %q): %v", pins[i], pins[i+1], err)
		}
		list = append(list, rt)
	}
	r, err := domain.NewRuntimes(list...)
	if err != nil {
		t.Fatalf("NewRuntimes: %v", err)
	}
	return r
}

// render flattens env vars to NAME=value, in order.
func render(vars []domain.EnvVar) []string {
	var out []string
	for _, v := range vars {
		out = append(out, v.Name()+"="+v.Value())
	}
	return out
}

// pins flattens runtimes to "kind version", in order.
func pins(r domain.Runtimes) []string {
	var out []string
	for _, rt := range r.List() {
		out = append(out, rt.String())
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

	t.Run("rejects PATH, which runtimes manage", func(t *testing.T) {
		_, err := domain.NewEnv(map[string]string{"PATH": "/usr/bin"})
		if !errors.Is(err, domain.ErrPathInEnv) {
			t.Fatalf("got %v, want ErrPathInEnv", err)
		}
	})

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

func TestNewRuntime(t *testing.T) {
	for _, version := range []string{"8.4", "20", "20.20.2"} {
		t.Run("accepts version "+version, func(t *testing.T) {
			if _, err := domain.NewRuntime("node", version); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("rejects an unknown toolchain", func(t *testing.T) {
		_, err := domain.NewRuntime("ruby", "3.3")
		if !errors.Is(err, domain.ErrUnknownRuntime) {
			t.Fatalf("got %v, want ErrUnknownRuntime", err)
		}
	})

	for _, version := range []string{"", "v20", "8.x", "8.", "8.4.1.2"} {
		t.Run("rejects version "+`"`+version+`"`, func(t *testing.T) {
			_, err := domain.NewRuntime("php", version)
			if !errors.Is(err, domain.ErrInvalidRuntimeVersion) {
				t.Fatalf("got %v, want ErrInvalidRuntimeVersion", err)
			}
		})
	}
}

func TestNewRuntimes(t *testing.T) {
	t.Run("rejects a toolchain pinned twice", func(t *testing.T) {
		php84, _ := domain.NewRuntime("php", "8.4")
		php83, _ := domain.NewRuntime("php", "8.3")
		_, err := domain.NewRuntimes(php84, php83)
		if !errors.Is(err, domain.ErrDuplicateRuntime) {
			t.Fatalf("got %v, want ErrDuplicateRuntime", err)
		}
	})
}

func TestRuntimesOverlay(t *testing.T) {
	project := runtimes(t, "php", "8.4", "node", "22")
	pane := runtimes(t, "node", "20")

	got := pins(project.Overlay(pane))
	if want := []string{"node 20", "php 8.4"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
