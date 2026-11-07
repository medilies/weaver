package app

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/medilies/weaver/internal/session/domain"
)

// EditProject lets the user edit a project's config in their editor. An
// invalid config is never stored: the user may edit it again, starting
// from what they wrote, or discard it and keep the stored one. Answering
// No reopens the editor, so Esc never loses edits.
type EditProject struct {
	documents ProjectDocuments
	editor    ConfigEditor
	confirm   Confirmer
}

// NewEditProject constructs the use case against its ports.
func NewEditProject(documents ProjectDocuments, editor ConfigEditor, confirm Confirmer) EditProject {
	return EditProject{documents: documents, editor: editor, confirm: confirm}
}

// Handle edits the config of the project named rawName and reports
// whether it changed.
func (uc EditProject) Handle(rawName string) (bool, error) {
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
