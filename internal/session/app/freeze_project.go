package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// FreezeRequest is how `weave freeze` was called.
type FreezeRequest struct {
	// Name is the project's name; by default the root's base name.
	Name string
	// Root is the project's directory, relative to WorkingDir or
	// absolute; by default WorkingDir.
	Root string
	// WorkingDir is the directory weaver runs in.
	WorkingDir string
	// Session is a running tmux session, or CurrentSession, whose windows,
	// panes and running commands the form starts from; it also suggests
	// the name.
	Session string
}

// CurrentSession, as FreezeRequest.Session, is the session weaver runs in.
// tmux turns "." in session names into "_", so no session is named so.
const CurrentSession = "."

// FreezeProject registers a project the user describes in a form, started
// from the layout and commands of a running session if asked.
type FreezeProject struct {
	projects  domain.ProjectRegistry
	capturer  domain.SessionCapturer
	workspace domain.Workspace
	form      ProjectForm
	confirm   Confirmer
}

// NewFreezeProject constructs the use case against its ports.
func NewFreezeProject(
	projects domain.ProjectRegistry,
	capturer domain.SessionCapturer,
	workspace domain.Workspace,
	form ProjectForm,
	confirm Confirmer,
) FreezeProject {
	return FreezeProject{
		projects: projects, capturer: capturer, workspace: workspace, form: form, confirm: confirm,
	}
}

// Handle runs the form and saves the project, asking before replacing a
// registered one. Declining opens the form again on the project as filled
// in, typing its name, so another can be given.
func (uc FreezeProject) Handle(request FreezeRequest) (domain.Project, error) {
	draft, err := uc.draft(request)
	if err != nil {
		return domain.Project{}, err
	}
	for {
		project, err := uc.form.Fill(draft)
		if err != nil {
			return domain.Project{}, err
		}
		names, err := uc.projects.Names()
		if err != nil {
			return domain.Project{}, err
		}
		if !slices.Contains(names, project.Name()) {
			return project, uc.projects.Save(project)
		}
		replace, err := uc.confirm.Confirm(fmt.Sprintf("%s is already registered. Replace it?", project.Name()))
		if err != nil {
			return domain.Project{}, err
		}
		if replace {
			return project, uc.projects.Save(project)
		}
		draft = ProjectDraft{
			Name:     project.Name().String(),
			Retype:   fmt.Errorf("%s is already registered: type another name, or keep it to replace it", project.Name()),
			Root:     project.Root().String(),
			Env:      project.Env(),
			Before:   project.Before(),
			Clearing: project.Clearing(),
			Setup:    project.Setup(),
			Teardown: project.Teardown(),
			Windows:  project.Windows(),
		}
	}
}

func (uc FreezeProject) draft(request FreezeRequest) (ProjectDraft, error) {
	root := request.Root
	if root == "" {
		root = request.WorkingDir
	} else if !filepath.IsAbs(root) {
		root = filepath.Join(request.WorkingDir, root)
	}
	draft := ProjectDraft{Name: request.Name, Root: filepath.Clean(root)}
	if session := request.Session; session != "" {
		if session == CurrentSession {
			var err error
			session, err = uc.capturer.Current()
			if errors.Is(err, domain.ErrNotInsideTmux) {
				return ProjectDraft{}, fmt.Errorf("%w; name a session", err)
			}
			if err != nil {
				return ProjectDraft{}, err
			}
		}
		windows, err := uc.capturer.Capture(session)
		if err != nil {
			return ProjectDraft{}, err
		}
		draft.Windows = uc.underRoot(windows, draft.Root)
		if _, err := domain.NewProjectName(session); err == nil && draft.Name == "" {
			draft.Name = session
		}
	}
	if draft.Name == "" {
		draft.Name = filepath.Base(draft.Root)
	}
	uc.keepCommands(&draft)
	return draft, nil
}

// keepCommands starts draft with the setup and teardown commands of the
// project registered under its name, if any, which a session can't show,
// so replacing that project keeps them; the form's diff shows them under
// whatever name it is saved. A config that can't be read has none to
// keep.
func (uc FreezeProject) keepCommands(draft *ProjectDraft) {
	name, err := domain.NewProjectName(draft.Name)
	if err != nil {
		return
	}
	registered, err := uc.projects.FindByName(name)
	if err != nil {
		return
	}
	draft.Setup, draft.Teardown = registered.Setup(), registered.Teardown()
}

// underRoot moves captured pane directories, which tmux reports with
// their symlinks resolved, back under root as written, so the form and
// the config can make them relative to it. A root the user then changes
// in the form to its resolved path leaves them absolute.
func (uc FreezeProject) underRoot(windows []domain.Window, root string) []domain.Window {
	realRoot := uc.workspace.RealPath(root)
	if realRoot == root {
		return windows
	}
	moved := make([]domain.Window, 0, len(windows))
	for _, window := range windows {
		moved = append(moved, window.Rebased(realRoot, root))
	}
	return moved
}
