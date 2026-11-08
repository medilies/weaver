package infra

import (
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

// optionalPaneName reads a pane name, blank for none.
func optionalPaneName(raw string) (domain.PaneName, error) {
	if strings.TrimSpace(raw) == "" {
		return domain.PaneName{}, nil
	}
	return domain.NewPaneName(raw)
}
