package domain

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// oneLine trims raw and rejects control characters such as tabs, newlines
// and escapes: names end up in tmux borders and commands are typed into a
// shell, where those would misfire.
func oneLine(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.IndexFunc(trimmed, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%q: %w", raw, ErrControlCharacters)
	}
	return trimmed, nil
}

// WindowName names a tmux window. It is unique within a project, which is
// how a running session's closed windows are found again.
type WindowName struct {
	value string
}

// NewWindowName validates and constructs a WindowName: trimmed, non-empty,
// one line.
func NewWindowName(raw string) (WindowName, error) {
	value, err := oneLine(raw)
	if err != nil {
		return WindowName{}, err
	}
	if value == "" {
		return WindowName{}, ErrEmptyWindowName
	}
	return WindowName{value: value}, nil
}

func (n WindowName) String() string {
	return n.value
}

// PaneName labels a pane in its window's top border. The zero value is an
// unnamed pane, which only a window's single pane may be.
type PaneName struct {
	value string
}

// NewPaneName validates and constructs a PaneName: trimmed, non-empty, one
// line.
func NewPaneName(raw string) (PaneName, error) {
	value, err := oneLine(raw)
	if err != nil {
		return PaneName{}, err
	}
	if value == "" {
		return PaneName{}, ErrEmptyPaneName
	}
	return PaneName{value: value}, nil
}

// DefaultPaneName is the label of the unnamed pane at index (from 0) of
// a window with several panes, which need one each: "pane 1", "pane 2"…
func DefaultPaneName(index int) PaneName {
	return PaneName{value: "pane " + strconv.Itoa(index+1)}
}

func (n PaneName) String() string {
	return n.value
}

// IsZero reports whether the pane is unnamed.
func (n PaneName) IsZero() bool {
	return n.value == ""
}

// NamePanes names the panes of a window, in creation order, from the
// names given: a single pane may stay unnamed, but when there are several
// each needs a name for its border, so an unnamed one gets its default.
// The names must then be unique.
func NamePanes(names ...PaneName) ([]PaneName, error) {
	named := slices.Clone(names)
	for i, name := range named {
		if name.IsZero() && len(named) > 1 {
			named[i] = DefaultPaneName(i)
		}
		if slices.Contains(named[:i], named[i]) {
			return nil, fmt.Errorf("%q: %w", named[i].value, ErrDuplicatePaneName)
		}
	}
	return named, nil
}

// PaneDir is the directory a pane's shell starts in, set on the pane or
// on its window: relative to the project root, or absolute. The zero
// value is the project root itself.
type PaneDir struct {
	path string
}

// NewPaneDir validates and constructs a PaneDir. Blank and "." both mean
// the project root.
func NewPaneDir(raw string) (PaneDir, error) {
	path, err := oneLine(raw)
	if err != nil {
		return PaneDir{}, err
	}
	if path == "" {
		return PaneDir{}, nil
	}
	if cleaned := filepath.Clean(path); cleaned != "." {
		return PaneDir{path: cleaned}, nil
	}
	return PaneDir{}, nil
}

// String is the cleaned path, or "" for the project root.
func (d PaneDir) String() string {
	return d.path
}

// IsRoot reports whether the pane starts in the project root.
func (d PaneDir) IsRoot() bool {
	return d.path == ""
}

// rebased is d moved from one directory to another, as Window.Rebased
// moves it.
func (d PaneDir) rebased(from, to string) PaneDir {
	if !filepath.IsAbs(d.path) {
		return d
	}
	if rel, ok := under(from, d.path); ok {
		return PaneDir{path: filepath.Join(to, rel)}
	}
	return d
}

// Direction is where a split puts the new pane relative to the pane it
// splits.
type Direction int

const (
	// Right puts the new pane to the right (tmux split-window -h).
	Right Direction = iota + 1
	// Below puts the new pane underneath (tmux split-window -v).
	Below
)

// ParseDirection reads "right" or "below".
func ParseDirection(raw string) (Direction, error) {
	switch strings.TrimSpace(raw) {
	case "right":
		return Right, nil
	case "below":
		return Below, nil
	}
	return 0, fmt.Errorf("%w: %q (want right or below)", ErrInvalidDirection, raw)
}

func (d Direction) String() string {
	switch d {
	case Right:
		return "right"
	case Below:
		return "below"
	}
	return "invalid"
}

// Size is how much of the split pane a new pane takes: a percentage, a
// number of cells, or tmux's default of half.
type Size struct {
	amount  int
	percent bool
}

// DefaultSize lets tmux split the pane in half.
func DefaultSize() Size {
	return Size{}
}

// Percent is a share of the split pane, from 1% to 99%.
func Percent(n int) (Size, error) {
	if n < 1 || n > 99 {
		return Size{}, fmt.Errorf("%w: %d%% (want 1%% to 99%%)", ErrInvalidSize, n)
	}
	return Size{amount: n, percent: true}, nil
}

// cells is a number of columns (right) or lines (below).
func cells(n int) (Size, error) {
	if n < 1 {
		return Size{}, fmt.Errorf("%w: %d cells (want at least 1)", ErrInvalidSize, n)
	}
	return Size{amount: n}, nil
}

// ParseSize reads "40%" (a percentage), "20" (cells) or "" (the default).
func ParseSize(raw string) (Size, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultSize(), nil
	}
	digits, percent := strings.CutSuffix(trimmed, "%")
	n, err := strconv.Atoi(digits)
	if err != nil {
		return Size{}, fmt.Errorf("%w: %q (want e.g. 40%% or 20)", ErrInvalidSize, raw)
	}
	if percent {
		return Percent(n)
	}
	return cells(n)
}

// IsDefault reports whether tmux picks the size.
func (s Size) IsDefault() bool {
	return s.amount == 0
}

// String is the size as tmux's split-window -l takes it, or "" for the
// default.
func (s Size) String() string {
	switch {
	case s.IsDefault():
		return ""
	case s.percent:
		return strconv.Itoa(s.amount) + "%"
	default:
		return strconv.Itoa(s.amount)
	}
}

// Split says how a pane is carved out of an earlier pane of its window,
// with tmux split-window semantics: the size is a share of the split pane.
type Split struct {
	of        int
	direction Direction
	size      Size
}

// NewSplit splits the window's pane at index of (0 is the first pane).
func NewSplit(of int, direction Direction, size Size) (Split, error) {
	if direction != Right && direction != Below {
		return Split{}, fmt.Errorf("%w: %d", ErrInvalidDirection, direction)
	}
	return Split{of: of, direction: direction, size: size}, nil
}

// Of is the index, within the window, of the pane being split.
func (s Split) Of() int {
	return s.of
}

func (s Split) Direction() Direction {
	return s.direction
}

func (s Split) Size() Size {
	return s.size
}

// Command is one shell command line, typed into a pane.
type Command struct {
	text string
}

// NewCommand validates and constructs a Command: trimmed, non-empty, one
// line.
func NewCommand(raw string) (Command, error) {
	text, err := oneLine(raw)
	if err != nil {
		return Command{}, err
	}
	if text == "" {
		return Command{}, ErrEmptyCommand
	}
	return Command{text: text}, nil
}

func (c Command) String() string {
	return c.text
}

// assignment is a variable set for the command that follows it, as in
// "NODE_ENV=dev npm run dev".
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// shellWords are builtins and keywords: what follows them isn't looked up
// on PATH, or not as the first word.
var shellWords = []string{
	"!", ".", ":", "alias", "autoload", "bg", "builtin", "case", "cd", "command", "coproc",
	"declare", "disown", "eval", "exec", "exit", "export", "fg", "for", "function", "hash", "history",
	"if", "jobs", "let", "local", "noglob", "nocorrect", "popd", "print", "pushd", "read", "readonly",
	"return", "select", "set", "setopt", "shift", "shopt", "source", "time", "trap", "type", "typeset",
	"ulimit", "umask", "unalias", "unset", "unsetopt", "until", "wait", "while",
}

// Program is the program the command line starts, e.g. "npm" for
// "NODE_ENV=dev npm run dev", or false when the shell decides what runs:
// a builtin, a keyword, or a word it expands or quotes, even in a
// variable's value. A comment starts none. Aliases and shell functions
// can't be told apart from programs.
func (c Command) Program() (string, bool) {
	for _, word := range strings.Fields(c.text) {
		if strings.HasPrefix(word, "#") || strings.ContainsAny(word, "$`'\"\\(){}[]*?~;&|<>") || slices.Contains(shellWords, word) {
			return "", false
		}
		if assignment.MatchString(word) {
			continue
		}
		return word, true
	}
	return "", false
}

// Unclosed reports whether the command line leaves a quote open, so a
// shell would wait for the rest of it instead of running it. Only when no
// shell closes it: POSIX shells end single quotes at the next one, while
// fish's take \' as a quote; a # starting a word outside quotes begins a
// comment, its quotes not counting.
func (c Command) Unclosed() bool {
	return openQuote(c.text, false) && openQuote(c.text, true)
}

// openQuote reports whether line ends inside quotes, single quotes taking
// \' and \\ as escapes when fish says so.
func openQuote(line string, fish bool) bool {
	var quote rune
	escaped, wordStart := false, true
	for _, r := range line {
		start := wordStart
		wordStart = false
		switch {
		case escaped:
			escaped = false
		case quote == '\'':
			escaped = fish && r == '\\'
			if r == '\'' {
				quote = 0
			}
		case quote == '"':
			escaped = r == '\\'
			if r == '"' {
				quote = 0
			}
		case r == '\\':
			escaped = true
		case r == '\'' || r == '"':
			quote = r
		case r == '#' && start:
			return false
		default:
			wordStart = strings.ContainsRune(" ;&|()", r)
		}
	}
	return quote != 0
}

// Startup is what a pane does once its shell is up: nothing, run a
// command, or leave a command typed at the prompt for the user to run.
type Startup struct {
	command Command
	submit  bool
}

// Command is the command typed into the pane, if any.
func (s Startup) Command() (Command, bool) {
	return s.command, s.command.text != ""
}

// Submits reports whether the typed command is run (Enter is pressed).
func (s Startup) Submits() bool {
	return s.submit
}

// Pane is one shell in a window. Build it fluently from NewPane; each
// method returns a new Pane.
type Pane struct {
	name    PaneName
	dir     PaneDir
	dirSet  bool
	split   Split
	splits  bool
	startup Startup
	before  []Command
	note    []string
	env     Env
	clears  Clearing
	tasks   []Task
	focused bool
}

// NewPane is an unnamed shell in its window's directory.
func NewPane() Pane {
	return Pane{}
}

// Named labels the pane.
func (p Pane) Named(name PaneName) Pane {
	p.name = name
	return p
}

// In starts the pane's shell in dir, instead of its window's: the root
// itself is a dir too, so a pane can leave its window's.
func (p Pane) In(dir PaneDir) Pane {
	p.dir = dir
	p.dirSet = true
	return p
}

// InWindowDir starts the pane's shell in its window's dir, as it does
// until In gives it its own.
func (p Pane) InWindowDir() Pane {
	p.dir = PaneDir{}
	p.dirSet = false
	return p
}

// Splitting creates the pane by splitting an earlier pane of its window.
func (p Pane) Splitting(split Split) Pane {
	p.split = split
	p.splits = true
	return p
}

// Running types cmd into the pane's shell and runs it. Typing it keeps it
// in the shell history, so it can be run again after Ctrl-C.
func (p Pane) Running(cmd Command) Pane {
	p.startup = Startup{command: cmd, submit: true}
	return p
}

// Prefilling types cmd at the pane's prompt without running it.
func (p Pane) Prefilling(cmd Command) Pane {
	p.startup = Startup{command: cmd}
	return p
}

// WithoutCommand starts a plain shell in the pane, typing nothing.
func (p Pane) WithoutCommand() Pane {
	p.startup = Startup{}
	return p
}

// RunningFirst runs cmds in the pane's shell, in order, before its own
// command, after its project's and its window's, e.g. to pick a toolchain
// version with nvm use.
func (p Pane) RunningFirst(cmds ...Command) Pane {
	p.before = slices.Clone(cmds)
	return p
}

// WithClearing sets whether the pane's before commands are hidden once
// they ran, over its window's and its project's.
func (p Pane) WithClearing(c Clearing) Pane {
	p.clears = c
	return p
}

// WithNote prints lines in the pane before its shell starts, or after its
// before commands when it clears them.
func (p Pane) WithNote(lines ...string) Pane {
	p.note = slices.Clone(lines)
	return p
}

// WithEnv sets variables for the pane's shell, over its window's and its
// project's.
func (p Pane) WithEnv(env Env) Pane {
	p.env = env
	return p
}

// WithFocus makes the pane the one its window selects. A window has at
// most one; its first pane is selected unless another is focused.
func (p Pane) WithFocus() Pane {
	p.focused = true
	return p
}

// Offering gives the pane tasks to run on request, in order, in place of
// those it had. Its window checks they can be told apart.
func (p Pane) Offering(tasks ...Task) Pane {
	p.tasks = slices.Clone(tasks)
	return p
}

func (p Pane) Name() PaneName {
	return p.name
}

// Dir is the pane's own directory, unless it starts in its window's.
func (p Pane) Dir() (PaneDir, bool) {
	return p.dir, p.dirSet
}

// Split is how the pane is created, unless it is its window's first pane.
func (p Pane) Split() (Split, bool) {
	return p.split, p.splits
}

func (p Pane) Startup() Startup {
	return p.startup
}

// Before is the pane's own commands run before its command, without its
// project's and its window's.
func (p Pane) Before() []Command {
	return slices.Clone(p.before)
}

// Clearing is whether the pane's own setting hides its before commands
// once they ran, without its window's and its project's.
func (p Pane) Clearing() Clearing {
	return p.clears
}

// Note is the lines printed before the pane's shell starts.
func (p Pane) Note() []string {
	return slices.Clone(p.note)
}

// Env is the pane's own variables, without its window's and its
// project's.
func (p Pane) Env() Env {
	return p.env
}

// Tasks are the commands the pane offers to run on request, in order.
func (p Pane) Tasks() []Task {
	return slices.Clone(p.tasks)
}

// Window is a tmux window and its panes, in creation order: the first pane
// fills the window and each later one splits an earlier one. Its panes
// say how they split, or its grid places them.
type Window struct {
	name    WindowName
	dir     PaneDir
	panes   []Pane
	before  []Command
	env     Env
	clears  Clearing
	grid    Grid
	gridded bool
	listing []int // a grid window's panes, by index, in the order its config lists them
	focused bool
}

// NewWindow validates and constructs a Window. With no panes, it holds one
// shell in its directory. Panes must be named, uniquely, when there are
// several: the names label their borders. Each pane's tasks must be told
// apart: no two share a name, nor two unnamed ones a command. At most one
// pane is focused.
func NewWindow(name WindowName, panes ...Pane) (Window, error) {
	if len(panes) == 0 {
		panes = []Pane{NewPane()}
	}
	named := make(map[string]bool, len(panes))
	focused := false
	for i, pane := range panes {
		if pane.focused && focused {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), ErrFocusedPaneTwice)
		}
		focused = focused || pane.focused
		if err := checkPlacement(i, pane); err != nil {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), err)
		}
		if err := checkTasks(pane.tasks); err != nil {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), err)
		}
		if len(panes) == 1 {
			continue
		}
		if pane.name.IsZero() {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), ErrUnnamedPane)
		}
		if named[pane.name.value] {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), ErrDuplicatePaneName)
		}
		named[pane.name.value] = true
	}
	return Window{name: name, panes: slices.Clone(panes)}, nil
}

// NewGridWindow validates and constructs a Window laid out by grid: every
// pane is named in its areas, and none says how it splits another. The
// panes are put in the order tmux creates them, each splitting an
// earlier one as the grid needs.
func NewGridWindow(name WindowName, grid Grid, panes ...Pane) (Window, error) {
	if len(grid.names) == 0 {
		return Window{}, ErrEmptyGrid // the zero Grid, not one from NewGrid
	}
	index := make(map[PaneName]int, len(panes))
	for i, pane := range panes {
		if _, splits := pane.Split(); splits {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), ErrSplitInGrid)
		}
		if _, taken := index[pane.name]; taken {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), ErrDuplicatePaneName)
		}
		if !slices.Contains(grid.names, pane.name) {
			return Window{}, fmt.Errorf("pane %s: %w", paneRef(i, pane), ErrPaneOutsideGrid)
		}
		index[pane.name] = i
	}
	for _, name := range grid.names {
		if _, ok := index[name]; !ok {
			return Window{}, fmt.Errorf("%w: %q", ErrUnknownGridPane, name.value)
		}
	}
	order := grid.cell.CreationOrder()
	ordered := make([]Pane, len(order))
	for i, carved := range order {
		ordered[i] = panes[index[grid.names[carved.Pane()]]]
		if split, ok := carved.Split(); ok {
			ordered[i] = ordered[i].Splitting(split)
		}
	}
	window, err := NewWindow(name, ordered...)
	if err != nil {
		return Window{}, err
	}
	window.grid, window.gridded = grid, true
	window.listing = make([]int, len(panes))
	for k, pane := range panes {
		window.listing[k] = slices.IndexFunc(ordered, func(p Pane) bool { return p.name == pane.name })
	}
	return window, nil
}

// Listing is the indexes of the window's panes in the order its config
// lists them: for a grid window, as they were given, which needn't be the
// order tmux creates them in; for any other, that order.
func (w Window) Listing() []int {
	if !w.gridded {
		return seq(len(w.panes))
	}
	return slices.Clone(w.listing)
}

// InSplits is the grid window laid out by the splits its grid makes in
// a window of cols×rows instead, its panes in the order they are
// created, each splitting an earlier one as the grid did, sized in
// percentages: a column or row in cells becomes its share of that
// window. A window laid out by splits already is as it is.
func (w Window) InSplits(cols, rows int) Window {
	if !w.gridded {
		return w
	}
	if w.grid.inCells {
		w.panes = slices.Clone(w.panes)
		for i, carved := range w.grid.carvedSharesIn(cols, rows) {
			if split, ok := carved.Split(); ok && i < len(w.panes) {
				w.panes[i].split = split
			}
		}
	}
	w.grid, w.gridded, w.listing = Grid{}, false, nil
	return w
}

// LaidOutBy is the grid window laid out by grid instead: a pane the grid
// still names keeps what it runs and its place in the listing, a name new
// to the window is a new plain pane, listed last, and a pane the grid no
// longer names is removed. The window keeps its own settings. A window
// laid out by splits can't take a grid this way.
func (w Window) LaidOutBy(grid Grid) (Window, error) {
	if !w.gridded {
		return Window{}, ErrNotAGrid
	}
	var panes []Pane
	listed := make(map[PaneName]bool, len(w.panes))
	for _, index := range w.listing {
		pane := w.panes[index]
		if slices.Contains(grid.names, pane.name) {
			pane.split, pane.splits = Split{}, false
			panes = append(panes, pane)
			listed[pane.name] = true
		}
	}
	for _, name := range grid.names {
		if !listed[name] {
			panes = append(panes, NewPane().Named(name))
		}
	}
	laid, err := NewGridWindow(w.name, grid, panes...)
	if err != nil {
		return Window{}, err
	}
	w.panes, w.grid, w.listing = laid.panes, laid.grid, laid.listing
	return w, nil
}

// checkPlacement enforces the creation order: the first pane fills the
// window, every later pane splits one created before it.
func checkPlacement(index int, pane Pane) error {
	split, splits := pane.Split()
	switch {
	case index == 0 && splits:
		return ErrFirstPaneSplits
	case index > 0 && !splits:
		return ErrPaneWithoutSplit
	case splits && (split.of < 0 || split.of >= index):
		return fmt.Errorf("%w (it splits pane #%d)", ErrSplitTargetNotEarlier, split.of+1)
	}
	return nil
}

// paneRef names a pane in errors: by name, else by 1-based position.
func paneRef(index int, pane Pane) string {
	if pane.name.IsZero() {
		return "#" + strconv.Itoa(index+1)
	}
	return strconv.Quote(pane.name.value)
}

// RunningFirst runs cmds in each of the window's panes, in order, after
// its project's and before the pane's own.
func (w Window) RunningFirst(cmds ...Command) Window {
	w.before = slices.Clone(cmds)
	return w
}

// WithEnv sets variables for its panes' shells, over its project's and
// under a pane's own.
func (w Window) WithEnv(env Env) Window {
	w.env = env
	return w
}

// WithClearing sets whether its panes' before commands are hidden once
// they ran, over its project's and under a pane's own.
func (w Window) WithClearing(c Clearing) Window {
	w.clears = c
	return w
}

// WithFocus makes the window the one opening a project lands in. A
// project has at most one; its first window is, unless another is
// focused.
func (w Window) WithFocus() Window {
	w.focused = true
	return w
}

// Focusing is the window selecting the pane at index instead of the one
// it selected. Its first pane is selected unless another is focused, so
// focusing it unfocuses the others.
func (w Window) Focusing(index int) (Window, error) {
	if err := w.checkPaneIndex(index); err != nil {
		return Window{}, err
	}
	w.panes = slices.Clone(w.panes)
	for i := range w.panes {
		w.panes[i].focused = index > 0 && i == index
	}
	return w, nil
}

// FocusedPane is the index of the pane the window selects: the focused
// one, else the first.
func (w Window) FocusedPane() int {
	return max(slices.IndexFunc(w.panes, func(p Pane) bool { return p.focused }), 0)
}

// Named renames the window. Window names are unique within a project,
// which the project checks.
func (w Window) Named(name WindowName) Window {
	w.name = name
	return w
}

// In starts the window's panes in dir, except those with their own.
func (w Window) In(dir PaneDir) Window {
	w.dir = dir
	return w
}

func (w Window) Name() WindowName {
	return w.name
}

// Dir is the directory the window's panes start in, unless they have
// their own: the project root by default.
func (w Window) Dir() PaneDir {
	return w.dir
}

// dirOf is the directory a pane starts in: its own, else its window's.
func (w Window) dirOf(pane Pane) PaneDir {
	if pane.dirSet {
		return pane.dir
	}
	return w.dir
}

// Panes are the window's panes in creation order.
func (w Window) Panes() []Pane {
	return slices.Clone(w.panes)
}

// Grid is the grid laying the window out, if any: its panes' splits then
// come from it.
func (w Window) Grid() (Grid, bool) {
	return w.grid, w.gridded
}

// Before is the window's own commands run first in each of its panes.
func (w Window) Before() []Command {
	return slices.Clone(w.before)
}

// Env is the window's own variables, without its project's and its
// panes'.
func (w Window) Env() Env {
	return w.env
}

// Clearing is the window's own setting of whether its panes' before
// commands are hidden once they ran.
func (w Window) Clearing() Clearing {
	return w.clears
}

// Rebased is the window with its and its panes' directories moved from
// one directory to another: an absolute directory at or under from
// becomes the same path under to, as when from is to with its symlinks
// resolved. Relative directories, and ones elsewhere, are kept as they
// are.
func (w Window) Rebased(from, to string) Window {
	w.dir = w.dir.rebased(from, to)
	w.panes = slices.Clone(w.panes)
	for i := range w.panes {
		w.panes[i].dir = w.panes[i].dir.rebased(from, to)
	}
	return w
}

// AddingPane is the window with pane added last: it splits one of the
// window's panes. An unnamed pane gets the first free default name,
// since a window with several panes needs one for each border.
func (w Window) AddingPane(pane Pane) (Window, error) {
	if w.gridded {
		return Window{}, ErrChangingGridPanes
	}
	return w.withPanes(append(slices.Clone(w.panes), pane))
}

// ReplacingPane is the window with the pane at index replaced, as when
// one of its fields changes or it splits differently. Unnamed among
// several, it gets the first free default name. In a window laid out by
// a grid, the pane keeps its split, which the grid gives, and renaming it
// renames it in the grid's areas.
func (w Window) ReplacingPane(index int, pane Pane) (Window, error) {
	if err := w.checkPaneIndex(index); err != nil {
		return Window{}, err
	}
	old := w.panes[index]
	if w.gridded && (pane.split != old.split || pane.splits != old.splits || pane.name.IsZero()) {
		return Window{}, ErrChangingGridPanes
	}
	panes := slices.Clone(w.panes)
	panes[index] = pane
	if w.gridded && pane.name != old.name {
		grid, err := w.grid.renaming(old.name, pane.name)
		if err != nil {
			return Window{}, err
		}
		w.grid = grid
	}
	return w.withPanes(panes)
}

// WithoutPane is the window with the pane at index removed, its space
// going back to the pane it split: the panes splitting it split that
// pane instead. Without the first pane, the second fills the window and
// the panes splitting the first split the second. A window keeps at
// least one pane.
func (w Window) WithoutPane(index int) (Window, error) {
	if err := w.checkPaneIndex(index); err != nil {
		return Window{}, err
	}
	if w.gridded {
		return Window{}, ErrChangingGridPanes
	}
	if len(w.panes) == 1 {
		return Window{}, ErrLastPane
	}
	panes := slices.Clone(w.panes)
	heir := panes[index].split.of
	if index == 0 {
		heir = 1
		panes[1].split, panes[1].splits = Split{}, false
	}
	for i := range panes {
		if panes[i].splits && panes[i].split.of == index {
			panes[i].split.of = heir
		}
	}
	order := slices.Delete(seq(len(panes)), index, index+1)
	return w.withPanes(reorder(panes, order))
}

// nameUnnamed gives each unnamed pane the first default name no pane
// has: "pane 1", "pane 2"…
func nameUnnamed(panes []Pane) []Pane {
	out := slices.Clone(panes)
	taken := make(map[PaneName]bool, len(out))
	for _, pane := range out {
		taken[pane.name] = true
	}
	next := 0
	for i := range out {
		for out[i].name.IsZero() {
			if name := DefaultPaneName(next); !taken[name] {
				out[i].name, taken[name] = name, true
			}
			next++
		}
	}
	return out
}

// reorder lists panes[order[0]], panes[order[1]]…, each split pointing
// at its pane's new index.
func reorder(panes []Pane, order []int) []Pane {
	index := make(map[int]int, len(order))
	for i, old := range order {
		index[old] = i
	}
	out := make([]Pane, len(order))
	for i, old := range order {
		out[i] = panes[old]
		if out[i].splits {
			out[i].split.of = index[out[i].split.of]
		}
	}
	return out
}

// seq is 0, 1… n-1.
func seq(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func (w Window) checkPaneIndex(index int) error {
	if index < 0 || index >= len(w.panes) {
		return fmt.Errorf("%w: #%d of %d", ErrPaneNotFound, index+1, len(w.panes))
	}
	return nil
}

// withPanes is the window holding panes instead, checked as NewWindow
// checks them, keeping its dir, before commands, env and grid. Among several
// panes, the unnamed ones get the first free default names.
func (w Window) withPanes(panes []Pane) (Window, error) {
	if len(panes) > 1 {
		panes = nameUnnamed(panes)
	}
	checked, err := NewWindow(w.name, panes...)
	if err != nil {
		return Window{}, err
	}
	w.panes = checked.panes
	return w, nil
}
