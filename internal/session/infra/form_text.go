package infra

import (
	"fmt"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// The forms' multi-line text fields, read into domain values.

// parseEnv reads "NAME=value" lines, skipping blank ones. Spaces around
// the name and the value are dropped, so "FOO = bar" sets FOO to "bar".
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
		vars[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return domain.NewEnv(vars)
}

// parseRepos reads "dir = remote" lines, skipping blank ones.
func parseRepos(text string) ([]domain.Repo, error) {
	var repos []domain.Repo
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dir, remote, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%q: want directory = remote", line)
		}
		repo, err := domain.NewRepo(strings.TrimSpace(dir), strings.TrimSpace(remote))
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, nil
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
