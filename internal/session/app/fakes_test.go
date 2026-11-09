package app_test

import (
	"fmt"
	"maps"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// inMemoryRegistry is a fake domain.ProjectRegistry, so these tests
// never touch disk.
type inMemoryRegistry struct {
	projects map[string]domain.Project
}

func (r inMemoryRegistry) FindByName(name domain.ProjectName) (domain.Project, error) {
	project, ok := r.projects[name.String()]
	if !ok {
		return domain.Project{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	return project, nil
}

func (r inMemoryRegistry) Save(project domain.Project) error {
	r.projects[project.Name().String()] = project
	return nil
}

func (r inMemoryRegistry) Delete(name domain.ProjectName) error {
	if _, ok := r.projects[name.String()]; !ok {
		return fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	delete(r.projects, name.String())
	return nil
}

func (r inMemoryRegistry) Names() ([]domain.ProjectName, error) {
	var names []domain.ProjectName
	for _, key := range slices.Sorted(maps.Keys(r.projects)) {
		names = append(names, r.projects[key].Name())
	}
	return names, nil
}

// fakeCommands is a fake domain.CommandRunner recording the commands run,
// and failing those in fails.
type fakeCommands struct {
	ran   []ranCommand
	fails map[string]bool
}

// ranCommand is a command fakeCommands ran, where and with what env.
type ranCommand struct {
	cmd string
	dir string
	env domain.Env
}

func (c *fakeCommands) Run(cmd domain.Command, dir string, env domain.Env) error {
	c.ran = append(c.ran, ranCommand{cmd: cmd.String(), dir: dir, env: env})
	if c.fails[cmd.String()] {
		return fmt.Errorf("%w (exit status 1): boom", domain.ErrCommandFailed)
	}
	return nil
}

// commandsRun is the text of the commands c ran, in order.
func (c *fakeCommands) commandsRun() []string {
	out := make([]string, 0, len(c.ran))
	for _, r := range c.ran {
		out = append(out, r.cmd)
	}
	return out
}
