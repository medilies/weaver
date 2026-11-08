package app

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// EditProject lets the user edit a project's config, in the config form
// or raw in their editor. An invalid config is never stored.
type EditProject struct {
	projects  domain.ProjectRegistry
	documents ProjectDocuments
	form      ProjectForm
	editor    ConfigEditor
	confirm   Confirmer
}

// NewEditProject constructs the use case against its ports.
func NewEditProject(
	projects domain.ProjectRegistry,
	documents ProjectDocuments,
	form ProjectForm,
	editor ConfigEditor,
	confirm Confirmer,
) EditProject {
	return EditProject{projects: projects, documents: documents, form: form, editor: editor, confirm: confirm}
}

// Handle changes the project named rawName in the config form, starting
// from its project and its config as stored, and saves the project the
// form returns. Saving drops the config's comments, so a commented config
// asks first; ErrAborted when the user declines or leaves the form.
func (uc EditProject) Handle(rawName string) error {
	name, err := domain.NewProjectName(rawName)
	if err != nil {
		return err
	}
	project, err := uc.projects.FindByName(name)
	if errors.Is(err, domain.ErrInvalidConfig) {
		return fmt.Errorf("%w; edit it raw to fix it", err)
	}
	if err != nil {
		return err
	}
	stored, err := uc.documents.Document(name)
	if err != nil {
		return err
	}
	commented, err := uc.documents.Commented(name)
	if err != nil {
		return err
	}
	if commented {
		question := fmt.Sprintf("%s's config has comments, which saving it from the config form drops. Go on?", name)
		yes, err := uc.confirm.Confirm(question)
		if err != nil {
			return err
		}
		if !yes {
			return ErrAborted
		}
	}
	changed, err := uc.form.Fill(ProjectDraft{
		Name:      name.String(),
		NameStays: true,
		Root:      project.Root().String(),
		Env:       project.Env(),
		Before:    project.Before(),
		Windows:   project.Windows(),
		Config:    stored.Content,
	})
	if err != nil {
		return err
	}
	if changed.Name() != name {
		return fmt.Errorf("the form renamed %s to %s, which only a new config can be", name, changed.Name())
	}
	return uc.projects.Save(changed)
}

// HandleRaw edits the config of the project named rawName in the user's
// editor and reports whether it changed. The user may edit an invalid
// config again, starting from what they wrote, or discard it and keep the
// stored one. Answering No reopens the editor, so Esc never loses edits.
func (uc EditProject) HandleRaw(rawName string) (bool, error) {
	name, err := domain.NewProjectName(rawName)
	if err != nil {
		return false, err
	}
	stored, err := uc.documents.Document(name)
	if err != nil {
		return false, err
	}
	document := stored
	for {
		edited, err := uc.editor.Edit(document)
		if err != nil {
			return false, fmt.Errorf("%w; nothing was saved", err)
		}
		if bytes.Equal(edited, stored.Content) {
			return false, nil
		}
		err = uc.documents.ReplaceDocument(name, edited)
		if !errors.Is(err, domain.ErrInvalidConfig) {
			return err == nil, err
		}
		discard, confirmErr := uc.confirm.Confirm(fmt.Sprintf("%v\n\nDiscard your changes?", err))
		if confirmErr != nil {
			return false, confirmErr
		}
		if discard {
			return false, fmt.Errorf("changes discarded: %w", err)
		}
		document.Content = edited
	}
}
