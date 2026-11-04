package domain

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
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

// Env is a set of environment variables for a project or a pane. PATH is
// not one of them: runtimes prepend to it, and setting it here would
// silently drop them.
type Env struct {
	vars map[string]string
}

// NewEnv validates and constructs an Env.
func NewEnv(vars map[string]string) (Env, error) {
	for name := range vars {
		if !envNamePattern.MatchString(name) {
			return Env{}, fmt.Errorf("%w: %q", ErrInvalidEnvName, name)
		}
		if name == "PATH" {
			return Env{}, ErrPathInEnv
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

// RuntimeKind is a toolchain whose version a project or pane can pin.
type RuntimeKind string

const (
	// Node is Node.js, as installed by nvm.
	Node RuntimeKind = "node"
	// PHP is PHP, as installed side by side by the distribution.
	PHP RuntimeKind = "php"
)

// RuntimeKinds are the toolchains weaver can pin, sorted.
func RuntimeKinds() []RuntimeKind {
	return []RuntimeKind{Node, PHP}
}

var runtimeVersionPattern = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)

// Runtime pins a toolchain to a version, e.g. php 8.4 or node 20: the
// pane's PATH starts with that version's binaries.
type Runtime struct {
	kind    RuntimeKind
	version string
}

// NewRuntime validates and constructs a Runtime. The version is a prefix
// of the installed version: node 20 is the newest installed 20.x.y.
func NewRuntime(kind, version string) (Runtime, error) {
	k := RuntimeKind(strings.TrimSpace(kind))
	if !slices.Contains(RuntimeKinds(), k) {
		return Runtime{}, fmt.Errorf("%w: %q (want node or php)", ErrUnknownRuntime, kind)
	}
	v := strings.TrimSpace(version)
	if !runtimeVersionPattern.MatchString(v) {
		return Runtime{}, fmt.Errorf("%w: %s %q (want e.g. 8.4 or 20)", ErrInvalidRuntimeVersion, k, version)
	}
	return Runtime{kind: k, version: v}, nil
}

func (r Runtime) Kind() RuntimeKind {
	return r.kind
}

func (r Runtime) Version() string {
	return r.version
}

// String reads like "php 8.4".
func (r Runtime) String() string {
	return string(r.kind) + " " + r.version
}

// Runtimes pins at most one version per toolchain.
type Runtimes struct {
	byKind map[RuntimeKind]Runtime
}

// NewRuntimes validates and constructs Runtimes.
func NewRuntimes(runtimes ...Runtime) (Runtimes, error) {
	byKind := make(map[RuntimeKind]Runtime, len(runtimes))
	for _, runtime := range runtimes {
		if _, pinned := byKind[runtime.kind]; pinned {
			return Runtimes{}, fmt.Errorf("%w: %s", ErrDuplicateRuntime, runtime.kind)
		}
		byKind[runtime.kind] = runtime
	}
	return Runtimes{byKind: byKind}, nil
}

// Overlay is r with top's pins added, top winning per toolchain, as a
// pane's runtimes do over its project's.
func (r Runtimes) Overlay(top Runtimes) Runtimes {
	merged := make(map[RuntimeKind]Runtime, len(r.byKind)+len(top.byKind))
	maps.Copy(merged, r.byKind)
	maps.Copy(merged, top.byKind)
	return Runtimes{byKind: merged}
}

// List is the pinned runtimes sorted by toolchain.
func (r Runtimes) List() []Runtime {
	runtimes := make([]Runtime, 0, len(r.byKind))
	for _, kind := range slices.Sorted(maps.Keys(r.byKind)) {
		runtimes = append(runtimes, r.byKind[kind])
	}
	return runtimes
}
