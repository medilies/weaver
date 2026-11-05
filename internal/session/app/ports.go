package app

import (
	"errors"

	"github.com/medilies/weaver/internal/session/domain"
)

// ErrAborted is returned when the user cancels an interactive step.
var ErrAborted = errors.New("aborted")

// Ports about domain concepts live in the domain package. The ones below
// are about talking to the user, which only use cases do.

// ProjectDraft is what the add form starts from: suggestions the user can
// change.
type ProjectDraft struct {
	Name      string
	Root      string
	Checkouts domain.Checkouts // found under Root, offered for cloning
	Runtimes  []domain.Runtime // installed versions, offered as pins
	Windows   []domain.Window  // a running session's, to start from; none to start blank
}

// ProjectForm lets the user describe a project, starting from a draft.
type ProjectForm interface {
	// Fill returns the project the user described, or ErrAborted.
	Fill(draft ProjectDraft) (domain.Project, error)
}

// Confirmer asks the user a yes-or-no question.
type Confirmer interface {
	// Confirm returns the answer, or ErrAborted.
	Confirm(question string) (bool, error)
}

// ConfigDocument is a project's config as the user writes it: the file it
// is stored in, e.g. "shop.yml", and what that file holds.
type ConfigDocument struct {
	File    string
	Content []byte
}

// ConfigEditor lets the user edit a project's config document.
type ConfigEditor interface {
	// Edit opens document in the user's editor and returns its content
	// as saved.
	Edit(document ConfigDocument) ([]byte, error)
}

// ProjectDocuments are projects' configs as the user writes them.
type ProjectDocuments interface {
	// Document is the project's config as stored, or ProjectNotFound.
	Document(name domain.ProjectName) (ConfigDocument, error)
	// ReplaceDocument stores content as the project's config, in the
	// stored config's file and format, if it is valid; otherwise the
	// stored config stays and the error wraps domain.ErrInvalidConfig.
	ReplaceDocument(name domain.ProjectName, content []byte) error
}

// Action is what the user can do from the project browser.
type Action int

const (
	// ActionOpen opens the chosen project, as `weave <project>` does.
	ActionOpen Action = iota + 1
	// ActionAdd adds a project rooted at the working directory.
	ActionAdd
	// ActionEdit edits the chosen project's config.
	ActionEdit
	// ActionKill stops the chosen project's session.
	ActionKill
	// ActionDelete unregisters the chosen project.
	ActionDelete
)

// Choice is what the user picked in the project browser: an action, and
// the project highlighted then, which the action applies to (adding a
// project applies to none, but the browser comes back to it).
type Choice struct {
	Action  Action
	Project domain.ProjectName
}

// ProjectBrowser shows the registered projects and lets the user pick
// what to do next.
type ProjectBrowser interface {
	// Browse shows projects, the one named selected highlighted and
	// notice in a status line, and returns the user's choice, or
	// ErrAborted when they quit.
	Browse(projects []ProjectOverview, selected domain.ProjectName, notice string) (Choice, error)
}
