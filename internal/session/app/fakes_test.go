package app_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

// inMemoryRegistry is a fake domain.ProjectRegistry, so these tests
// never touch disk. Its projects are keyed by name, and listed in that
// order.
type inMemoryRegistry struct {
	projects map[string]domain.Project
}

func (r inMemoryRegistry) Find(root domain.ProjectRoot) (domain.Project, error) {
	for _, project := range r.projects {
		if project.Root() == root {
			return project, nil
		}
	}
	return domain.Project{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, root)
}

func (r inMemoryRegistry) Save(project domain.Project, _ string) error {
	for key, registered := range r.projects {
		if registered.Root() == project.Root() {
			delete(r.projects, key)
		}
	}
	r.projects[project.Name().String()] = project
	return nil
}

func (r inMemoryRegistry) Delete(root domain.ProjectRoot) error {
	for key, project := range r.projects {
		if project.Root() == root {
			delete(r.projects, key)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", domain.ErrProjectNotFound, root)
}

func (r inMemoryRegistry) Registered() ([]domain.ProjectRef, error) {
	var refs []domain.ProjectRef
	for _, key := range slices.Sorted(maps.Keys(r.projects)) {
		refs = append(refs, r.projects[key].Ref())
	}
	return refs, nil
}

// fakeCommands is a fake domain.CommandRunner recording the commands run,
// and failing those in fails.
type fakeCommands struct {
	ran   []ranCommand
	fails map[string]bool
	onRun func() // called as each command runs
}

// ranCommand is a command fakeCommands ran, where and with what env.
type ranCommand struct {
	cmd string
	dir string
	env domain.Env
}

func (c *fakeCommands) Run(cmd domain.Command, dir string, env domain.Env) error {
	c.ran = append(c.ran, ranCommand{cmd: cmd.String(), dir: dir, env: env})
	if c.onRun != nil {
		c.onRun()
	}
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

// command is the command raw.
func command(t *testing.T, raw string) domain.Command {
	t.Helper()
	cmd, err := domain.NewCommand(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return cmd
}

// fakeAcks is a fake app.Acknowledger recording the messages shown, or
// failing with err.
type fakeAcks struct {
	shown []string
	err   error
}

func (a *fakeAcks) Acknowledge(message string) error {
	a.shown = append(a.shown, message)
	return a.err
}
