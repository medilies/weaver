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
	// ErrProjectNotFound is returned when a registry has no project
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
	// ErrInvalidTrack is returned for a grid column or row size that is
	// neither a percentage from 1% to 100% nor a positive weight (2fr).
	ErrInvalidTrack = errors.New("invalid grid size")
	// ErrEmptyGrid is returned when a grid names no pane, or has an empty
	// row, or is missing.
	ErrEmptyGrid = errors.New("every grid row must name at least one pane")
	// ErrRaggedGrid is returned when a grid's rows have different lengths.
	ErrRaggedGrid = errors.New("every grid row must name as many panes")
	// ErrTrackCount is returned when a grid has more or fewer sizes than
	// columns or rows.
	ErrTrackCount = errors.New("a grid needs one size per column and per row")
	// ErrTrackShares is returned when a grid's percentages don't share out
	// the whole window: 100% without weights, less with some.
	ErrTrackShares = errors.New("grid sizes must share out the whole window")
	// ErrPaneNotRectangle is returned when a pane's cells in a grid don't
	// form one rectangle.
	ErrPaneNotRectangle = errors.New("a pane's cells in a grid must form one rectangle")
	// ErrInterlockedPanes is returned when panes of a grid can't be told
	// apart by straight cuts across the window, the only layouts tmux
	// builds.
	ErrInterlockedPanes = errors.New("tmux can't build this grid: no straight line across the window separates these panes")
	// ErrSplitInGrid is returned when a pane of a window laid out by a grid
	// also says how it splits another.
	ErrSplitInGrid = errors.New("a window laid out by a grid must not split its panes")
	// ErrPaneOutsideGrid is returned when a pane of a window laid out by a
	// grid isn't in its areas.
	ErrPaneOutsideGrid = errors.New("the window's grid doesn't place this pane")
	// ErrUnknownGridPane is returned when a grid names a pane its window
	// doesn't have.
	ErrUnknownGridPane = errors.New("the grid names a pane this window doesn't have")
)

var (
	// ErrInvalidEnvName is returned for an environment variable name that
	// a shell can't export.
	ErrInvalidEnvName = errors.New("invalid environment variable name")
	// ErrSessionNotRunning is returned when killing a session that isn't
	// running.
	ErrSessionNotRunning = errors.New("session not running")
	// ErrSessionNotOwned is returned when killing the session weaver runs
	// in while weaver didn't start it.
	ErrSessionNotOwned = errors.New("weave didn't start this session")
	// ErrNotInsideTmux is returned when asking for the session weaver runs
	// in while it runs outside tmux.
	ErrNotInsideTmux = errors.New("not inside tmux")
)
