// Package app holds the Session bounded context's use cases. It depends
// only on the domain package, never on infrastructure or the CLI.
package app

import (
	"errors"

	"github.com/medilies/weaver/internal/session/domain"
)

// ErrAborted is returned when the user cancels an interactive step.
var ErrAborted = errors.New("aborted")

// Ports about domain concepts live in the domain package. The ones below
// are about talking to the user, which only use cases do.

// ProjectDraft is what a form starts from: suggestions the user can
// change when binding, or a registered project and its config when
// editing it, whose name stays.
type ProjectDraft struct {
	Name      string
	NameStays bool // editing: only a new config can be named otherwise
	Root      string
	Env       domain.Env
	Before    []domain.Command
	Clearing  domain.Clearing
	Setup     []domain.Command
	Teardown  []domain.Command
	Windows   []domain.Window // a running session's, to start from; none to start blank
	Config    []byte          // the config as stored, which saving replaces; none when binding
}

// ProjectForm lets the user describe a project, starting from a draft.
type ProjectForm interface {
	// Fill returns the project the user described, or ErrAborted. Given a
	// draft whose name doesn't stay, it returns a project only once the
	// user has agreed to replace any config registered under its name.
	Fill(draft ProjectDraft) (domain.Project, error)
}

// Confirmer asks the user a yes-or-no question.
type Confirmer interface {
	// Confirm returns the answer, or ErrAborted.
	Confirm(question string) (bool, error)
}

// Acknowledger tells the user something and waits until they have read
// it.
type Acknowledger interface {
	// Acknowledge shows message and returns once the user is done with it.
	Acknowledge(message string) error
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
	// Document is the project's config as stored, or ErrProjectNotFound.
	Document(name domain.ProjectName) (ConfigDocument, error)
	// ReplaceDocument stores content as the project's config, in the
	// stored config's file, if it is valid; otherwise the
	// stored config stays and the error wraps domain.ErrInvalidConfig.
	ReplaceDocument(name domain.ProjectName, content []byte) error
	// Commented reports whether the project's config holds comments of
	// the user's, which saving the project in place of it drops.
	Commented(name domain.ProjectName) (bool, error)
}

// Action is what the user can do from the project browser.
type Action int

const (
	// ActionOpen opens the chosen project, as `weave <project>` does.
	ActionOpen Action = iota + 1
	// ActionBind registers a project rooted at the working directory.
	ActionBind
	// ActionEdit edits the chosen project's config in the config form.
	ActionEdit
	// ActionEditRaw edits the chosen project's config in the editor.
	ActionEditRaw
	// ActionKill kills the chosen project's session.
	ActionKill
	// ActionDelete unregisters the chosen project.
	ActionDelete
	// ActionRunTask runs the chosen task of the chosen project, then puts
	// the user where it runs.
	ActionRunTask
	// ActionRerun types the chosen pane's exited command into it again,
	// then puts the user in that pane.
	ActionRerun
	// ActionStart starts the chosen project, the browser staying.
	ActionStart
	// ActionRunTaskStaying runs the chosen task of the chosen project, the
	// browser staying.
	ActionRunTaskStaying
	// ActionRunTaskApart runs the chosen task of the chosen project in a
	// throwaway window, the browser staying.
	ActionRunTaskApart
	// ActionInterruptTask types ctrl+c into a copy of the chosen task of
	// the chosen project, the browser staying.
	ActionInterruptTask
	// ActionInterruptPane types ctrl+c into the chosen pane of the chosen
	// project, the browser staying.
	ActionInterruptPane
	// ActionReloadPane reloads the chosen pane of the chosen project, the
	// browser staying.
	ActionReloadPane
	// ActionReloadWindow reloads the window of the chosen pane of the
	// chosen project, the browser staying.
	ActionReloadWindow
)

// Choice is what the user picked in the project browser: an action, and
// the project highlighted then, which the action applies to (binding a
// project applies to none, but the browser comes back to it), the task
// picked when running or interrupting one, and the pane picked when
// rerunning its command or interrupting it.
type Choice struct {
	Action  Action
	Project domain.ProjectName
	Task    TaskAt
	Pane    PaneAt
}

// Notice is what the browser's status line tells: what the last action
// did, or why it failed.
type Notice struct {
	text    string
	failure bool
}

// Telling is the notice saying what an action did, such as "Saved shop.".
func Telling(text string) Notice {
	return Notice{text: text}
}

// FailingWith is the notice saying why an action failed.
func FailingWith(err error) Notice {
	return Notice{text: err.Error(), failure: true}
}

// Text is what the notice says.
func (n Notice) Text() string {
	return n.text
}

// Failure reports whether the notice says why an action failed.
func (n Notice) Failure() bool {
	return n.failure
}

// ProjectBrowser shows the registered projects and lets the user pick
// what to do next. It stays on screen from one choice to the next, until
// it leaves.
type ProjectBrowser interface {
	// Browse shows projects, the one named selected highlighted and
	// notice in a status line, and returns the user's choice, or
	// ErrAborted when they quit. Already on screen, it shows them in
	// place; after a choice, it stays on screen, waiting. While it shows
	// a project whose Previewable reports true, it keeps asking live for
	// the preview of the selected window and for the project as it is
	// now.
	Browse(projects []ProjectOverview, selected domain.ProjectName, notice Notice, live LiveSessions) (Choice, error)
	// Confirm asks in the status line while the browser is on screen,
	// otherwise as a Confirmer of its own.
	Confirmer
	// Working tells, while the browser is on screen waiting for an
	// action, what the action is doing, as in "Killing shop…"; off screen
	// it does nothing.
	Working(text string)
	// Leave takes the browser off screen, for what needs the terminal;
	// the next Browse shows it again.
	Leave() error
}

// LiveSessions tells how projects' owned sessions are now, for the
// browser to keep up with them.
type LiveSessions interface {
	// Preview is the window named window of project's owned session as
	// it is now. An error wraps domain.ErrSessionNotRunning when no
	// session runs, domain.ErrSessionNotOwned when weaver didn't start it,
	// domain.ErrWindowNotFound when the project has no such window, or
	// domain.ErrWindowNotRunning when the session lacks it.
	Preview(project ProjectOverview, window domain.WindowName) (domain.WindowPreview, error)
	// Refreshed is project as its owned session is now: the warnings
	// about commands that exited, and the tasks still running, found
	// again; none of either when tmux can't tell.
	Refreshed(project ProjectOverview) ProjectOverview
}
