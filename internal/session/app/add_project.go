package app

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// AddRequest is how `weave freeze` was called.
type AddRequest struct {
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

// CurrentSession, as AddRequest.Session, is the session weaver runs in.
// tmux turns "." in session names into "_", so no session is named so.
const CurrentSession = "."

// AddProject registers a project the user describes in a form, started
// from what weaver finds: the repositories under its root, and the layout
// and commands of a running session if asked.
type AddProject struct {
	projects domain.ProjectRepository
	detector domain.RepoDetector
	capturer domain.SessionCapturer
	form     ProjectForm
	confirm  Confirmer
}

// NewAddProject constructs the use case against its ports.
func NewAddProject(
	projects domain.ProjectRepository,
	detector domain.RepoDetector,
	capturer domain.SessionCapturer,
	form ProjectForm,
	confirm Confirmer,
) AddProject {
	return AddProject{
		projects: projects, detector: detector, capturer: capturer, form: form, confirm: confirm,
	}
}

// Handle runs the form and saves the project, asking before replacing a
// registered one.
func (uc AddProject) Handle(request AddRequest) (domain.Project, error) {
	draft, err := uc.draft(request)
	if err != nil {
		return domain.Project{}, err
	}
	project, err := uc.form.Fill(draft)
	if err != nil {
		return domain.Project{}, err
	}
	names, err := uc.projects.Names()
	if err != nil {
		return domain.Project{}, err
	}
	if slices.Contains(names, project.Name()) {
		replace, err := uc.confirm.Confirm(fmt.Sprintf("%s is already registered. Replace it?", project.Name()))
		if err != nil {
			return domain.Project{}, err
		}
		if !replace {
			return domain.Project{}, ErrAborted
		}
	}
	return project, uc.projects.Save(project)
}

func (uc AddProject) draft(request AddRequest) (ProjectDraft, error) {
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
			if session, err = uc.capturer.Current(); err != nil {
				return ProjectDraft{}, err
			}
		}
		windows, err := uc.capturer.Capture(session)
		if err != nil {
			return ProjectDraft{}, err
		}
		draft.Windows = windows
		if _, err := domain.NewProjectName(session); err == nil && draft.Name == "" {
			draft.Name = session
		}
	}
	if draft.Name == "" {
		draft.Name = filepath.Base(draft.Root)
	}
	projectRoot, err := domain.NewProjectRoot(draft.Root)
	if err != nil {
		return ProjectDraft{}, err
	}
	if draft.Repos, err = uc.detector.Detect(projectRoot); err != nil {
		return ProjectDraft{}, err
	}
	return draft, nil
}
