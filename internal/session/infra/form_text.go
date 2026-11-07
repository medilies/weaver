package infra

import (
	"fmt"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// The forms' text fields, read into domain values.

// parseRoot reads a project root as configs write it: absolute, or
// starting with "~/" for the home directory.
func parseRoot(text string) (domain.ProjectRoot, error) {
	path, err := ExpandHome(strings.TrimSpace(text))
	if err != nil {
		return domain.ProjectRoot{}, err
	}
	return domain.NewProjectRoot(path)
}

// parseEnv reads "NAME=value" lines, skipping blank ones. Spaces around
// the name and the value are dropped, so "FOO = bar" sets FOO to "bar".
// A name set twice is an error, as in a config.
func parseEnv(text string) (domain.Env, error) {
	vars := make(map[string]string)
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return domain.Env{}, fmt.Errorf("%q: want NAME=value", line)
		}
		name = strings.TrimSpace(name)
		if _, set := vars[name]; set {
			return domain.Env{}, fmt.Errorf("%s: set twice", name)
		}
		vars[name] = strings.TrimSpace(value)
	}
	return domain.NewEnv(vars)
}

// parseCommands reads one command per line, skipping blank ones.
func parseCommands(text string) ([]domain.Command, error) {
	var cmds []domain.Command
	for line := range strings.Lines(text) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cmd, err := domain.NewCommand(line)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}
