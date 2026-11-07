package domain

import "errors"

var (
	// ErrEmptyProjectName is returned when a project name is blank.
	ErrEmptyProjectName = errors.New("project name must not be empty")
	// ErrInvalidProjectName is returned when a project name has characters
	// a filename, a tmux session name or a command-line argument can't
	// carry safely.
	ErrInvalidProjectName = errors.New("project name must start with a letter or digit and contain only letters, digits, '-' and '_'")
	// ErrReservedProjectName is returned when a project name is one of
	// weave's commands, which `weave <name>` would run instead.
	ErrReservedProjectName = errors.New("project name is a weave command")
	// ErrEmptyProjectRoot is returned when a project root path is blank.
	ErrEmptyProjectRoot = errors.New("project root must not be empty")
	// ErrRelativeProjectRoot is returned when a project root is not an
	// absolute path: `weave <project>` runs from anywhere.
	ErrRelativeProjectRoot = errors.New("project root must be an absolute path")
	// ErrProjectNotFound is returned when a repository has no project
	// under the given name.
	ErrProjectNotFound = errors.New("project not found")
	// ErrInvalidConfig wraps whatever makes a stored project config
	// unreadable, as opposed to failing to read it at all.
	ErrInvalidConfig = errors.New("invalid config")
)

var (
	// ErrControlCharacters is returned when a name, directory or command
	// contains tabs, newlines or other control characters.
	ErrControlCharacters = errors.New("must not contain control characters such as tabs or newlines")
	// ErrEmptyWindowName is returned when a window name is blank.
	ErrEmptyWindowName = errors.New("window name must not be empty")
	// ErrEmptyPaneName is returned when a pane name is blank.
	ErrEmptyPaneName = errors.New("pane name must not be empty")
	// ErrInvalidDirection is returned for a split direction other than
	// right or below.
	ErrInvalidDirection = errors.New("invalid split direction")
	// ErrInvalidSize is returned for a split size that is neither a
	// percentage from 1% to 99% nor a positive number of cells.
	ErrInvalidSize = errors.New("invalid split size")
	// ErrEmptyCommand is returned when a pane command is blank.
	ErrEmptyCommand = errors.New("command must not be empty")
	// ErrFirstPaneSplits is returned when a window's first pane has a
	// split: it fills the window, there is nothing to split yet.
	ErrFirstPaneSplits = errors.New("the first pane fills the window and must not split another pane")
	// ErrPaneWithoutSplit is returned when a later pane doesn't say which
	// pane it splits.
	ErrPaneWithoutSplit = errors.New("every pane after the first must split an earlier pane")
	// ErrSplitTargetNotEarlier is returned when a pane splits itself or a
	// pane created after it.
	ErrSplitTargetNotEarlier = errors.New("a pane can only split a pane created before it")
	// ErrUnnamedPane is returned when a window with several panes has an
	// unnamed one: names label the pane borders.
	ErrUnnamedPane = errors.New("panes must be named when a window has several")
	// ErrDuplicatePaneName is returned when two panes of a window share a
	// name.
	ErrDuplicatePaneName = errors.New("pane names must be unique within a window")
	// ErrDuplicateWindowName is returned when two windows of a project
	// share a name.
	ErrDuplicateWindowName = errors.New("window names must be unique within a project")
)

var (
	// ErrInvalidEnvName is returned for an environment variable name that
	// a shell can't export.
	ErrInvalidEnvName = errors.New("invalid environment variable name")
	// ErrSessionNotRunning is returned when killing a session that isn't
	// running.
	ErrSessionNotRunning = errors.New("session not running")
	// ErrNotInsideTmux is returned when asking for the session weaver runs
	// in while it runs outside tmux.
	ErrNotInsideTmux = errors.New("not inside tmux")
)
