package app

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/medilies/weaver/internal/session/domain"
)

// AddRequest is how `weaver add` was called.
type AddRequest struct {
	// Name is the project's name; by default the root's base name.
	Name string
	// Root is the project's directory, relative to WorkingDir or
	// absolute; by default WorkingDir.
	Root string
	// WorkingDir is the directory weaver runs in.
	WorkingDir string
}

// AddProject registers a project the user describes in a form, started
// from what weaver finds: the repositories under its root and the
// installed toolchain versions.
type AddProject struct {
	projects domain.ProjectRepository
	detector domain.CheckoutDetector
	catalog  domain.RuntimeCatalog
	form     ProjectForm
	confirm  Confirmer
}

// NewAddProject constructs the use case against its ports.
func NewAddProject(
	projects domain.ProjectRepository,
	detector domain.CheckoutDetector,
	catalog domain.RuntimeCatalog,
	form ProjectForm,
	confirm Confirmer,
) AddProject {
	return AddProject{projects: projects, detector: detector, catalog: catalog, form: form, confirm: confirm}
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
	if draft.Name == "" {
		draft.Name = filepath.Base(draft.Root)
	}
	projectRoot, err := domain.NewProjectRoot(draft.Root)
	if err != nil {
		return ProjectDraft{}, err
	}
	if draft.Checkouts, err = uc.detector.Detect(projectRoot); err != nil {
		return ProjectDraft{}, err
	}
	if draft.Runtimes, err = uc.catalog.Available(); err != nil {
		return ProjectDraft{}, err
	}
	return draft, nil
}
