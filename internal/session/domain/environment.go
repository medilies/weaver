package domain

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// EnvVar is one environment variable a pane's shell starts with.
type EnvVar struct {
	name  string
	value string
}

func (v EnvVar) Name() string {
	return v.name
}

// Value is set literally: "$HOME" stays "$HOME".
func (v EnvVar) Value() string {
	return v.value
}

// Env is a set of environment variables for a project or a pane.
type Env struct {
	vars map[string]string
}

// NewEnv validates and constructs an Env.
func NewEnv(vars map[string]string) (Env, error) {
	for name := range vars {
		if !envNamePattern.MatchString(name) {
			return Env{}, fmt.Errorf("%w: %q", ErrInvalidEnvName, name)
		}
	}
	return Env{vars: maps.Clone(vars)}, nil
}

// Overlay is e with top's variables added, top winning when both set one,
// as a pane's variables do over its project's.
func (e Env) Overlay(top Env) Env {
	merged := make(map[string]string, len(e.vars)+len(top.vars))
	maps.Copy(merged, e.vars)
	maps.Copy(merged, top.vars)
	return Env{vars: merged}
}

// Vars are the variables sorted by name.
func (e Env) Vars() []EnvVar {
	vars := make([]EnvVar, 0, len(e.vars))
	for _, name := range slices.Sorted(maps.Keys(e.vars)) {
		vars = append(vars, EnvVar{name: name, value: e.vars[name]})
	}
	return vars
}
