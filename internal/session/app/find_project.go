package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// ErrNoProjectAt is returned when no registered project's root contains
// a directory.
var ErrNoProjectAt = errors.New("no registered project contains this directory")

// FindProject finds the registered project a command line names: by its
// name, or by a directory its root contains.
type FindProject struct {
	projects  domain.ProjectRegistry
	workspace domain.Workspace
}

// NewFindProject constructs the use case against its ports.
func NewFindProject(projects domain.ProjectRegistry, workspace domain.Workspace) FindProject {
	return FindProject{projects: projects, workspace: workspace}
}

// Handle finds the project arg names: "." or a path, holding "/", is a
// directory, relative to workingDir, which the project's root contains;
// anything else is the project's name, which only one project may have.
func (uc FindProject) Handle(arg, workingDir string) (domain.ProjectRef, error) {
	if arg == "." || arg == ".." || strings.Contains(arg, "/") {
		dir := arg
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workingDir, dir)
		}
		return uc.containing(filepath.Clean(dir))
	}
	name, err := domain.NewProjectName(arg)
	if err != nil {
		return domain.ProjectRef{}, err
	}
	refs, err := uc.projects.Registered()
	if err != nil {
		return domain.ProjectRef{}, err
	}
	var named []domain.ProjectRef
	var roots []string
	for _, ref := range refs {
		if ref.Name() == name {
			named = append(named, ref)
			roots = append(roots, ref.Root().String())
		}
	}
	switch len(named) {
	case 0:
		return domain.ProjectRef{}, fmt.Errorf("%w: %s (run weave to see the registered projects)", domain.ErrProjectNotFound, name)
	case 1:
		return named[0], nil
	}
	return domain.ProjectRef{}, fmt.Errorf("%w: %s is rooted at %s; name one by its root instead", domain.ErrAmbiguousProject,
		name, strings.Join(roots, " and "))
}

// containing is the project whose root contains dir; when roots nest,
// the innermost one. Roots and dir are compared with their symlinks
// resolved, as they are on disk, and as written only when no root
// contains dir that way, as when dir is reached through a link inside a
// root that leads out of every root. So a link inside one root that leads
// into another finds the other.
func (uc FindProject) containing(dir string) (domain.ProjectRef, error) {
	refs, err := uc.projects.Registered()
	if err != nil {
		return domain.ProjectRef{}, err
	}
	realDir := uc.workspace.RealPath(dir)
	var resolved, written []rootMatch
	for _, ref := range refs {
		root := ref.Root()
		resolvedRoot, err := domain.NewProjectRoot(uc.workspace.RealPath(root.String()))
		switch {
		case err == nil && resolvedRoot.Contains(realDir):
			resolved = append(resolved, rootMatch{project: ref, root: resolvedRoot.String()})
		case root.Contains(dir):
			written = append(written, rootMatch{project: ref, root: root.String()})
		}
	}
	if len(resolved) == 0 {
		resolved = written
	}
	var found *rootMatch
	for _, m := range resolved {
		if found == nil || len(m.root) > len(found.root) {
			found = &m
		}
	}
	if found == nil {
		return domain.ProjectRef{}, fmt.Errorf("%w: %s (run weave bind there to register it)", ErrNoProjectAt, dir)
	}
	return found.project, nil
}

// rootMatch is a project whose root, in one form, contains a directory.
type rootMatch struct {
	project domain.ProjectRef
	root    string
}
