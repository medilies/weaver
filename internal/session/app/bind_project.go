package app

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/medilies/weaver/internal/session/domain"
)

// BindRequest is how `weave bind` was called.
type BindRequest struct {
	// Name is the project's name; by default its config's, else the
	// root's directory name.
	Name string
	// Config is the config to write, relative to the root or absolute; by
	// default the root's registered one, else weaver.yml.
	Config string
	// WorkingDir is the directory weaver runs in, the project's root.
	WorkingDir string
	// Session is a running tmux session, or CurrentSession, whose windows,
	// panes and running commands the form starts from.
	Session string
}

// CurrentSession, as BindRequest.Session, is the session weaver runs in.
// tmux turns "." in session names into "_", so no session is named so.
const CurrentSession = "."

// BindProject registers the working directory as a project's root,
// through a form, started from its config if it has one, and from the
// layout and commands of a running session if asked.
type BindProject struct {
	projects  domain.ProjectRegistry
	documents ProjectDocuments
	capturer  domain.SessionCapturer
	workspace domain.Workspace
	form      ProjectForm
}

// NewBindProject constructs the use case against its ports.
func NewBindProject(
	projects domain.ProjectRegistry,
	documents ProjectDocuments,
	capturer domain.SessionCapturer,
	workspace domain.Workspace,
	form ProjectForm,
) BindProject {
	return BindProject{
		projects: projects, documents: documents, capturer: capturer, workspace: workspace, form: form,
	}
}

// Handle runs the form, then saves the project and registers its root.
// The form shows what saving changes in a config already there. A session
// the form started from is then renamed after the project, so it counts
// as the project's.
func (uc BindProject) Handle(request BindRequest) (domain.Project, error) {
	draft, session, err := uc.draft(request)
	if err != nil {
		return domain.Project{}, err
	}
	project, err := uc.form.Fill(draft)
	if err != nil {
		return domain.Project{}, err
	}
	if err := uc.projects.Save(project, request.Config); err != nil {
		return domain.Project{}, err
	}
	if session != "" {
		if err := uc.capturer.Adopt(session, project.Ref()); err != nil {
			return project, fmt.Errorf("saved %s, but its session %s wasn't renamed: %w", project.Name(), session, err)
		}
	}
	return project, nil
}

// draft is what the form starts from, and the session it captured, if
// any.
func (uc BindProject) draft(request BindRequest) (ProjectDraft, string, error) {
	root, err := domain.NewProjectRoot(request.WorkingDir)
	if err != nil {
		return ProjectDraft{}, "", err
	}
	draft := ProjectDraft{Name: filepath.Base(root.String()), Root: root.String()}
	project, document, err := uc.documents.ConfigAt(root, request.Config)
	switch {
	case err == nil:
		draft = ProjectDraft{
			Name: project.Name().String(), Root: root.String(), Env: project.Env(), Before: project.Before(),
			Clearing: project.Clearing(), Setup: project.Setup(), Teardown: project.Teardown(),
			Windows: project.Windows(), Config: document.Content,
		}
	case errors.Is(err, domain.ErrInvalidConfig):
		return ProjectDraft{}, "", fmt.Errorf("%w; fix it first", err)
	case !errors.Is(err, domain.ErrMissingConfig):
		return ProjectDraft{}, "", err
	}
	if request.Name != "" {
		draft.Name = request.Name
	}
	if _, err := registeredAt(uc.projects, root); errors.Is(err, domain.ErrProjectNotFound) {
		draft.Unregistered = true
	} else if err != nil {
		return ProjectDraft{}, "", err
	}
	session := request.Session
	if session == "" {
		return draft, "", nil
	}
	if session == CurrentSession {
		session, err = uc.capturer.Current()
		if errors.Is(err, domain.ErrNotInsideTmux) {
			return ProjectDraft{}, "", fmt.Errorf("%w; name a session", err)
		}
		if err != nil {
			return ProjectDraft{}, "", err
		}
	}
	windows, err := uc.capturer.Capture(session)
	if err != nil {
		return ProjectDraft{}, "", err
	}
	draft.Windows = uc.underRoot(windows, draft.Root)
	return draft, session, nil
}

// underRoot moves captured pane directories, which tmux reports with
// their symlinks resolved, back under root as written, so the form and
// the config can make them relative to it. A root the user then changes
// in the form to its resolved path leaves them absolute.
func (uc BindProject) underRoot(windows []domain.Window, root string) []domain.Window {
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
