package infra

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// TeaConfigForm is the config form: a project shown full screen as
// cards, one for the project, then the window list and one for the
// selected window, its panes drawn, the pane list and the selected
// pane's card below, windows and panes picked by key or click, changed
// in place. Each field is checked by the domain's constructors
// as it is typed, and each change is made by the domain's own methods:
// the form only collects them.
type TeaConfigForm struct {
	workspace domain.Workspace
	documents app.ProjectDocuments
}

// NewTeaConfigForm constructs a TeaConfigForm checking directories in
// workspace, and showing what saving a new project under a registered
// name replaces in its config, read from documents.
func NewTeaConfigForm(workspace domain.Workspace, documents app.ProjectDocuments) TeaConfigForm {
	return TeaConfigForm{workspace: workspace, documents: documents}
}

// Fill implements app.ProjectForm. The draft's name stays when it says
// so, and is typed otherwise. It returns the project as ctrl+s saves it,
// once the user has seen what saving changes in the draft's config, or
// ErrAborted when the user leaves it unchanged or discards it. A draft
// without a Config, as freezing gives, is unsaved until then, and saving
// it under a registered name shows what it changes in that config. A name to
// type that isn't valid, or that the draft says to retype, opens the form
// typing it.
func (f TeaConfigForm) Fill(draft app.ProjectDraft) (domain.Project, error) {
	name, nameErr := domain.NewProjectName(draft.Name)
	if nameErr != nil && draft.NameStays {
		return domain.Project{}, nameErr
	}
	if nameErr != nil {
		name = standInName
	}
	root, err := parseRoot(draft.Root)
	if err != nil {
		return domain.Project{}, err
	}
	project, err := domain.NewProject(name, root).WithEnv(draft.Env).RunningFirst(draft.Before...).
		WithClearing(draft.Clearing).WithWindows(draft.Windows...)
	if err != nil {
		return domain.Project{}, err
	}
	var original []byte
	if len(draft.Config) > 0 {
		if original, err = encodeProject(project); err != nil {
			return domain.Project{}, err
		}
	}
	model := configForm{
		project: project, original: original, stored: draft.Config, nameStays: draft.NameStays,
		workspace: f.workspace, documents: f.documents, focus: spot{window: -1, pane: -1, field: fieldRoot},
	}
	if !draft.NameStays {
		model.focus.field = fieldName
	}
	if nameErr != nil {
		model.unnamed = true
		model, model.start = model.typeInto(model.focus, draft.Name, true)
		model.edit.err = nameErr
	} else if draft.Retype != nil && !draft.NameStays {
		model, model.start = model.typeInto(model.focus, draft.Name, true)
		model.edit.err = draft.Retype
	}
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return domain.Project{}, err
	}
	form := final.(configForm)
	if !form.saved {
		return domain.Project{}, app.ErrAborted
	}
	return form.project, nil
}

// field is what a row of a card shows.
type field int

const (
	fieldName field = iota + 1
	fieldRoot
	fieldDir
	fieldBefore
	fieldEnv
	fieldProjectEnv // a variable the project sets, shown in a pane, which enter sets for the pane
	fieldNote
	fieldStarts
	fieldClear
	fieldCommand
	fieldWindowList // the window list, the window being the one selected
	fieldPaneList   // the pane list, the pane being the one selected
	fieldDrawing    // the window's panes drawn, the pane being the one selected
)

// isList reports whether f holds a list, a row per item.
func isList(f field) bool {
	return f == fieldBefore || f == fieldEnv || f == fieldNote
}

// spot is a row the cursor can be on: a field of the project (window
// -1), of a window (pane -1) or of a pane, and in a list, the item, its
// length for the row adding one. The name of the window after the last
// is the row adding one, as is its entry in the window list.
type spot struct {
	window, pane int
	field        field
	item         int
}

// configForm is TeaConfigForm's Bubble Tea model. The project is always
// valid: a field being typed changes it only once enter keeps it.
type configForm struct {
	project       domain.Project
	original      []byte // the project as its config writes it, to tell changes; none when unsaved
	stored        []byte // the config as stored, which saving replaces
	nameStays     bool   // the project's name can't be typed
	unnamed       bool   // the project has standInName until a name typed is kept
	start         tea.Cmd
	workspace     domain.Workspace
	documents     app.ProjectDocuments
	focus         spot
	picked        spot // the window and pane shown, kept while the cursor is above them
	top           int  // the first line shown
	editing       bool
	edit          typing
	asking        *question
	reviewing     *review
	undos, redos  []state // the last change last
	problem       string  // why the last change was refused
	width, height int
	saved         bool
}

// state is the project as it was and the row the cursor was on, which
// undo and redo go back to.
type state struct {
	project domain.Project
	focus   spot
	unnamed bool
}

// standInName names a project being frozen until a valid name is typed,
// when the one suggested isn't.
var standInName, _ = domain.NewProjectName("project")

// projectNameAt is the row typing the project's name.
var projectNameAt = spot{window: -1, pane: -1, field: fieldName}

// review is what saving changes in the stored config, shown before
// saving, and its first line shown. A new project saved under a
// registered name replaces that project's config.
type review struct {
	diff     []string
	top      int
	unstored bool // no config is stored under the name
	replaces bool // a new project's config replaces a registered one's
}

// typing is a field being typed.
type typing struct {
	at    spot
	from  spot // where the cursor was, to go back to when the row typed into goes
	input textinput.Model
	run   bool // a command typed is run, rather than prefilled
	// preview is the project with what is typed, when it is valid; err
	// is why it isn't.
	preview domain.Project
	err     error
}

// question is a yes-or-no question in the status line, and what yes
// does.
type question struct {
	text string
	yes  func(configForm) (configForm, tea.Cmd)
}

var checkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

func (m configForm) Init() tea.Cmd {
	return m.start
}

// Update is m after msg, scrolled to keep the cursor in view.
func (m configForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	next.picked = next.selection()
	next.top = next.scrolled()
	return next, cmd
}

func (m configForm) update(msg tea.Msg) (configForm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.edit.input.SetWidth(m.inputWidth(m.edit.at))
		if m.reviewing != nil {
			m.reviewing = m.reviewing.scrolledTo(m.reviewing.top, m.bodyHeight())
		}
		return m, nil
	case tea.MouseClickMsg:
		// A click picks a window, a pane or how a pane starts: not while
		// typing, asked or shown the diff, which the keyboard answers.
		if msg.Button == tea.MouseLeft && m.reviewing == nil && m.asking == nil && !m.editing {
			m.problem = ""
			return m.clicked(msg.X, msg.Y)
		}
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case m.reviewing != nil:
			return m.reviewed(msg.String())
		case m.asking != nil:
			return m.answer(msg.String())
		case m.editing:
			return m.typed(msg)
		}
		m.problem = ""
		return m.press(msg.String())
	}
	if m.editing { // the cursor blinking
		var cmd tea.Cmd
		m.edit.input, cmd = m.edit.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// answer is m after key answers the question asked: y does what yes
// does, n or esc leaves things as they are.
func (m configForm) answer(key string) (configForm, tea.Cmd) {
	switch key {
	case "y", "Y":
		yes := m.asking.yes
		m.asking = nil
		return yes(m)
	case "n", "N", "esc", "ctrl+c":
		m.asking = nil
	}
	return m, nil
}

// typed is m after msg, pressed while a field is typed: enter keeps
// what is typed if it is valid, esc cancels it.
func (m configForm) typed(msg tea.KeyPressMsg) (configForm, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.kept(), nil
	case "esc":
		m.editing = false
		return m.settledOn(m.edit.from), nil
	case "ctrl+s":
		if m = m.kept(); m.editing {
			return m, nil
		}
		return m.save()
	case "ctrl+c":
		m.editing = false
		return m.leave()
	case "up", "down", "tab", "shift+tab":
		return m, nil
	case "left", "right":
		if m.edit.at.field == fieldCommand && m.edit.input.Value() == "" {
			return m.startTyped(msg.String() == "right"), nil
		}
	}
	var cmd tea.Cmd
	m.edit.input, cmd = m.edit.input.Update(msg)
	m.edit.preview, m.edit.err = m.applied(m.edit.at, m.edit.input.Value())
	return m, cmd
}

// kept is m with what is typed kept, or still typing it when it isn't
// valid.
func (m configForm) kept() configForm {
	project, err := m.applied(m.edit.at, m.edit.input.Value())
	if err != nil {
		m.edit.err = err
		return m
	}
	m = m.became(project)
	m.editing = false
	if m.edit.at == projectNameAt {
		m.unnamed = false
	}
	if at, ok := m.typedVar(); ok {
		return m.settledOn(at)
	}
	return m.settledOn(m.edit.at)
}

// settledOn is m with the cursor on the row at, else back where typing
// started from.
func (m configForm) settledOn(at spot) configForm {
	m.focus = at
	if !slices.Contains(m.layout().spots, at) {
		m.focus = m.edit.from
	}
	return m.settled(1)
}

// typedVar is the row of the variable just typed, in the list it is now
// in: a project's variable set for a pane, or one renamed, moves.
func (m configForm) typedVar() (spot, bool) {
	at := m.edit.at
	if at.field != fieldEnv && at.field != fieldProjectEnv {
		return spot{}, false
	}
	name, _, _ := strings.Cut(m.edit.input.Value(), "=")
	index := slices.Index(m.ownEnv(at).Names(), strings.TrimSpace(name))
	if index < 0 {
		return spot{}, false
	}
	return spot{window: at.window, pane: at.pane, field: fieldEnv, item: index}, true
}

// press is m after key, pressed on a row.
func (m configForm) press(key string) (configForm, tea.Cmd) {
	if m.focus.field == fieldDrawing {
		if next, cmd, ok := m.pressedOnDrawing(key); ok {
			return next, cmd
		}
	}
	switch key {
	case "up", "k":
		return m.moved(-1), nil
	case "down", "j":
		return m.moved(1), nil
	case "left", "h":
		return m.stepped(-1)
	case "right", "l":
		return m.stepped(1)
	case "enter":
		return m.entered()
	case "o":
		return m.added()
	case "d":
		return m.deleting(), nil
	case "J":
		return m.reordered(1), nil
	case "K":
		return m.reordered(-1), nil
	case "u":
		return m.undone(), nil
	case "ctrl+r":
		return m.redone(), nil
	case "ctrl+s":
		return m.save()
	case "esc", "ctrl+c":
		return m.leave()
	}
	return m, nil
}

// save ends the form when nothing changed, and otherwise shows what
// saving changes in the stored config first.
func (m configForm) save() (configForm, tea.Cmd) {
	if m.unnamed {
		m.focus, m.problem = projectNameAt, "Type the project's name first."
		return m, nil
	}
	if !m.dirty() {
		return m, tea.Quit
	}
	doc, err := encodeProject(m.project)
	if err != nil {
		m.problem = err.Error()
		return m, nil
	}
	stored, replaces, err := m.replaced()
	if err != nil {
		m.problem = err.Error()
		return m, nil
	}
	diff := configDiff(string(stored), string(doc))
	if len(diff) == 0 && !replaces {
		m.saved = true
		return m, tea.Quit
	}
	m.reviewing = &review{diff: diff, unstored: len(stored) == 0, replaces: replaces}
	return m, nil
}

// replaced is the config saving replaces: the one stored, else, for a new
// project, the config registered under its name, if any.
func (m configForm) replaced() ([]byte, bool, error) {
	if len(m.stored) > 0 || m.nameStays || m.documents == nil {
		return m.stored, false, nil
	}
	document, err := m.documents.Document(m.project.Name())
	if errors.Is(err, domain.ErrProjectNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return document.Content, true, nil
}

// reviewed is m after key, pressed on what saving changes: y, enter or
// ctrl+s saves, n or esc goes back to the form, ↑↓ scroll.
func (m configForm) reviewed(key string) (configForm, tea.Cmd) {
	top := m.reviewing.top
	switch key {
	case "y", "Y", "enter", "ctrl+s":
		m.saved = true
		return m, tea.Quit
	case "n", "N", "esc", "ctrl+c":
		m.reviewing = nil
		return m, nil
	case "up", "k":
		top--
	case "down", "j":
		top++
	case "pgup":
		top -= m.bodyHeight()
	case "pgdown", "space":
		top += m.bodyHeight()
	}
	m.reviewing = m.reviewing.scrolledTo(top, m.bodyHeight())
	return m, nil
}

// scrolledTo is r showing from the line top, as near as height lines
// shown allow.
func (r review) scrolledTo(top, height int) *review {
	r.top = max(min(top, len(r.diff)-height), 0)
	return &r
}

// became is m with project, the project before it kept for undo, unless
// its config is the same.
func (m configForm) became(project domain.Project) configForm {
	before, errBefore := encodeProject(m.project)
	after, errAfter := encodeProject(project)
	if errBefore != nil || errAfter != nil || !bytes.Equal(before, after) || m.project.Name() != project.Name() {
		m.undos = append(slices.Clip(m.undos), state{project: m.project, focus: m.focus, unnamed: m.unnamed})
		m.redos = nil
	}
	m.project = project
	return m
}

// undone is m after u: the project as it was before its last change, the
// cursor back where it was then.
func (m configForm) undone() configForm {
	if len(m.undos) == 0 {
		m.problem = "Nothing to undo."
		return m
	}
	last := m.undos[len(m.undos)-1]
	m.undos = m.undos[:len(m.undos)-1]
	m.redos = append(slices.Clip(m.redos), state{project: m.project, focus: m.focus, unnamed: m.unnamed})
	m.project, m.focus, m.unnamed = last.project, last.focus, last.unnamed
	return m.settled(1)
}

// redone is m after ctrl+r: the last change undone made again.
func (m configForm) redone() configForm {
	if len(m.redos) == 0 {
		m.problem = "Nothing to redo."
		return m
	}
	last := m.redos[len(m.redos)-1]
	m.redos = m.redos[:len(m.redos)-1]
	m.undos = append(slices.Clip(m.undos), state{project: m.project, focus: m.focus, unnamed: m.unnamed})
	m.project, m.focus, m.unnamed = last.project, last.focus, last.unnamed
	return m.settled(1)
}

// leave ends the form without saving, asking first when it changed.
func (m configForm) leave() (configForm, tea.Cmd) {
	if !m.dirty() {
		return m, tea.Quit
	}
	text := "Discard your changes?"
	if m.original == nil {
		text = "Leave without saving the new project?"
	}
	m.asking = &question{text: text, yes: func(m configForm) (configForm, tea.Cmd) {
		return m, tea.Quit
	}}
	return m, nil
}

// dirty reports whether the project's config would change.
func (m configForm) dirty() bool {
	doc, err := encodeProject(m.project)
	return err != nil || !bytes.Equal(doc, m.original)
}

// changedTo is m with project, or with why it was refused.
func (m configForm) changedTo(project domain.Project, err error) configForm {
	return m.changedAt(project, err, m.focus)
}

// changedAt is m with project and the cursor on the row at, or with why
// project was refused.
func (m configForm) changedAt(project domain.Project, err error, at spot) configForm {
	if err != nil {
		m.problem = err.Error()
		return m
	}
	m = m.became(project)
	m.focus = at
	return m.settled(1)
}

// moved is m with the cursor by rows down, or up when by is negative.
func (m configForm) moved(by int) configForm {
	spots := m.layout().spots
	i := slices.Index(spots, m.focus)
	if i < 0 {
		return m.settled(by)
	}
	m.focus = spots[max(min(i+by, len(spots)-1), 0)]
	return m.settled(by)
}

// settled is m with the cursor on a row shown: where it is, else the
// nearest item of its list, else the first row of its pane or window,
// or the last when moving up, else the row before where it was.
func (m configForm) settled(prefer int) configForm {
	spots := m.layout().spots
	if len(spots) == 0 || slices.Contains(spots, m.focus) {
		return m
	}
	if m.focus.field == fieldCommand { // the pane starts a shell now
		m.focus.field = fieldStarts
		return m.settled(prefer)
	}
	f := m.focus
	var same []spot
	for _, s := range spots {
		if s.window == f.window && s.pane == f.pane {
			same = append(same, s)
		}
	}
	nearest := -1
	for i, s := range same {
		if s.field == f.field && s.item <= f.item {
			nearest = i
		}
	}
	switch {
	case nearest >= 0:
		m.focus = same[nearest]
	case len(same) > 0 && prefer < 0:
		m.focus = same[len(same)-1]
	case len(same) > 0:
		m.focus = same[0]
	default:
		m.focus = spots[0]
		for _, s := range spots {
			if s.window < f.window || (s.window == f.window && s.pane < f.pane) {
				m.focus = s
			}
		}
	}
	return m
}

// stepped is m after ← (by -1) or → (by 1) on a choice: the window
// shown or + window, the pane shown, or how the pane starts.
func (m configForm) stepped(by int) (configForm, tea.Cmd) {
	switch m.focus.field {
	case fieldWindowList:
		count := len(m.project.Windows()) + 1
		m.focus.window = ((m.focus.window+by)%count + count) % count
		return m, nil
	case fieldPaneList:
		if window, ok := m.windowAt(m.focus.window); ok {
			count := len(window.Panes())
			m.focus.pane = ((m.focus.pane+by)%count + count) % count
		}
		return m, nil
	case fieldStarts:
		return m.started(by)
	case fieldClear:
		count := len(clearOptions(m.focus))
		next := ((clearIndex(m.clearingAt(m.focus))+by)%count + count) % count
		return m.changedTo(m.cleared(m.focus, clearings[next])), nil
	}
	return m, nil
}

// clearings are the settings a clear row goes through, in the order of
// its options.
var clearings = []domain.Clearing{domain.ClearingUnset, domain.ClearingOn, domain.ClearingOff}

// clearOptions are the options of the clear row at: the project's no or
// yes, or leaving it to the level above, yes or no for a window or a
// pane.
func clearOptions(at spot) []string {
	switch {
	case at.window < 0:
		return []string{"no", "yes"}
	case at.pane < 0:
		return []string{"project's", "yes", "no"}
	}
	return []string{"window's", "yes", "no"}
}

// clearIndex is the index in clearings, and in its row's options, of c.
func clearIndex(c domain.Clearing) int {
	return slices.Index(clearings, c)
}

// clearingAt is the clearing the project, window or pane of at's row
// sets, the project's off shown as its no, unset.
func (m configForm) clearingAt(at spot) domain.Clearing {
	c := m.project.Clearing()
	if window, ok := m.windowAt(at.window); ok && at.pane < 0 {
		c = window.Clearing()
	} else if _, pane, ok := m.paneAt(at); ok {
		c = pane.Clearing()
	}
	if at.window < 0 && c == domain.ClearingOff {
		return domain.ClearingUnset
	}
	return c
}

// cleared is the project with the project, window or pane of at's row
// setting c.
func (m configForm) cleared(at spot, c domain.Clearing) (domain.Project, error) {
	switch {
	case at.window < 0:
		return m.project.WithClearing(c), nil
	case at.pane < 0:
		return m.windowChanged(at, func(w domain.Window) (domain.Window, error) { return w.WithClearing(c), nil })
	}
	return m.paneChanged(at, func(p domain.Pane) (domain.Pane, error) { return p.WithClearing(c), nil })
}

// pressedOnDrawing is m after key, pressed on the drawing, with tmux's
// keys: arrows select the pane next to the selected one, going to the
// rows above or below at the window's edge, % and " split it right or
// below, x removes it, and H and L make it smaller or larger. Enter goes
// to its card. It is false for the other keys.
func (m configForm) pressedOnDrawing(key string) (configForm, tea.Cmd, bool) {
	switch key {
	case "up", "k":
		return m.selectedNext(0, -1), nil, true
	case "down", "j":
		return m.selectedNext(0, 1), nil, true
	case "left", "h":
		return m.selectedNext(-1, 0), nil, true
	case "right", "l":
		return m.selectedNext(1, 0), nil, true
	case "%":
		return m.split(domain.Right), nil, true
	case "\"":
		return m.split(domain.Below), nil, true
	case "x":
		return m.deleting(), nil, true
	case "H":
		return m.resized(-1), nil, true
	case "L":
		return m.resized(1), nil, true
	case "enter":
		m.focus.field = fieldName
		return m, nil, true
	}
	return m, nil, false
}

// selectedNext is m with the pane next to the selected one selected, to
// the right for dx 1, the left for -1, below for dy 1 or above for -1,
// as the session opens at the terminal's size. With none there, ↑ and ↓
// go to the row above or below the drawing.
func (m configForm) selectedNext(dx, dy int) configForm {
	plan := m.project.Plan().Windows()[m.focus.window]
	if next, ok := neighbour(layOut(plan, m.width, max(m.height-1, 1)), m.focus.pane, dx, dy); ok {
		m.focus.pane = next
		return m
	}
	if dy != 0 {
		return m.moved(dy)
	}
	return m
}

// split is m with the selected pane split in direction, the new pane,
// taking half, selected.
func (m configForm) split(direction domain.Direction) configForm {
	at := m.focus
	window, ok := m.windowAt(at.window)
	if !ok {
		return m
	}
	split, err := domain.NewSplit(at.pane, direction, domain.DefaultSize())
	if err != nil {
		return m.changedTo(domain.Project{}, err)
	}
	added, err := window.AddingPane(domain.NewPane().Splitting(split))
	project, err := replaced(m.project, at.window, added, err)
	return m.changedAt(project, err, spot{window: at.window, pane: len(window.Panes()), field: fieldDrawing})
}

// resized is m with the selected pane taking 5% or 5 cells more of the
// pane it splits, or less when by is negative.
func (m configForm) resized(by int) configForm {
	at := m.focus
	window, pane, _ := m.paneAt(at)
	split, splits := pane.Split()
	if _, gridded := window.Grid(); gridded {
		m.problem = domain.ErrChangingGridPanes.Error()
		return m
	}
	if !splits {
		m.problem = "The first pane takes what the others leave: resize them."
		return m
	}
	return m.changedTo(m.paneChanged(at, func(pane domain.Pane) (domain.Pane, error) {
		size, err := steppedSize(split.Size(), by)
		if err != nil {
			return domain.Pane{}, err
		}
		changed, err := domain.NewSplit(split.Of(), split.Direction(), size)
		return pane.Splitting(changed), err
	}))
}

// steppedSize is size moved up, or down when by is negative: a number of
// cells by 5 cells, else to the next multiple of 5%, from half by
// default.
func steppedSize(size domain.Size, by int) (domain.Size, error) {
	digits, percent := strings.CutSuffix(size.String(), "%")
	n, _ := strconv.Atoi(digits)
	switch {
	case size.IsDefault():
		n = 50
	case !percent:
		return domain.ParseSize(strconv.Itoa(max(n+5*by, 1)))
	}
	if by > 0 {
		n = (n/5 + 1) * 5
	} else {
		n = ((n+4)/5 - 1) * 5
	}
	return domain.Percent(max(min(n, 99), 1))
}

// startModes are how a pane starts, in the order ←→ goes through them.
var startModes = []string{"shell", "command", "prefill"}

// startMode is the index in startModes of how pane starts.
func startMode(pane domain.Pane) int {
	if _, ok := pane.Startup().Command(); !ok {
		return 0
	}
	if pane.Startup().Submits() {
		return 1
	}
	return 2
}

// started is m with the pane starting the next way, or the previous when
// by is negative: a command it has is kept, run or prefilled, and one it
// lacks is typed first.
func (m configForm) started(by int) (configForm, tea.Cmd) {
	at := m.focus
	_, pane, _ := m.paneAt(at)
	cmd, has := pane.Startup().Command()
	next := (startMode(pane) + by + len(startModes)) % len(startModes)
	if next > 0 && !has {
		return m.typeInto(spot{window: at.window, pane: at.pane, field: fieldCommand}, "", next == 1)
	}
	return m.changedTo(m.paneChanged(at, func(pane domain.Pane) (domain.Pane, error) {
		switch next {
		case 1:
			return pane.Running(cmd), nil
		case 2:
			return pane.Prefilling(cmd), nil
		}
		return pane.WithoutCommand(), nil
	})), nil
}

// startTyped is m after ← or → (forward) while the command typed is
// empty: it is run or prefilled, going on through startModes, and shell
// keeps the pane without one.
func (m configForm) startTyped(forward bool) configForm {
	by := -1
	if forward {
		by = 1
	}
	next := (m.typedMode() + by + len(startModes)) % len(startModes)
	if next == 0 {
		return m.kept()
	}
	m.edit.run = next == 1
	return m
}

// typedMode is the index in startModes of how the pane whose command is
// typed starts.
func (m configForm) typedMode() int {
	if m.edit.run {
		return 1
	}
	return 2
}

// startsClicked is m after a click on option of how the pane at starts.
func (m configForm) startsClicked(at spot, option int) (configForm, tea.Cmd) {
	m.focus = at
	if _, pane, ok := m.paneAt(at); ok {
		if by := option - startMode(pane); by != 0 {
			return m.started(by)
		}
	}
	return m, nil
}

// entered is m after enter on a row: a choice goes to its next option,
// the window list types the window's name, or a new one's on
// "+ window", the pane list the pane's, and other rows are typed into.
func (m configForm) entered() (configForm, tea.Cmd) {
	at := m.focus
	switch at.field {
	case fieldStarts, fieldClear:
		return m.stepped(1)
	case fieldWindowList:
		at.field = fieldName
		if _, ok := m.windowAt(at.window); !ok {
			return m.typeInto(at, "", true)
		}
	case fieldPaneList:
		at.field = fieldName
	}
	run := true
	if _, pane, ok := m.paneAt(at); ok {
		if _, has := pane.Startup().Command(); has {
			run = pane.Startup().Submits()
		}
	}
	return m.typeInto(at, m.current(at), run)
}

// typeInto is m typing into the row at, starting from text.
func (m configForm) typeInto(at spot, text string, run bool) (configForm, tea.Cmd) {
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(text)
	input.CursorEnd()
	input.SetWidth(m.inputWidth(at))
	cmd := input.Focus()
	m.editing = true
	m.edit = typing{at: at, from: m.focus, input: input, run: run, preview: m.project}
	m.focus = at
	return m, cmd
}

// inputWidth is the cells a field typed at gets: what its card leaves
// beside the label, less room for a check mark.
func (m configForm) inputWidth(at spot) int {
	depth := 1
	if at.pane >= 0 && at.window >= 0 {
		depth = 2
	}
	return max(m.width-4*depth-gutterWidth-labelWidth-12, 10)
}

// added is m after o: an item typed into the list the cursor is in, the
// pane it is in split right, or a window typed after the last.
func (m configForm) added() (configForm, tea.Cmd) {
	at := m.focus
	switch {
	case isList(at.field) || at.field == fieldProjectEnv:
		list := spot{window: at.window, pane: at.pane, field: at.field}
		if list.field == fieldProjectEnv {
			list.field = fieldEnv
		}
		list.item = len(m.items(list))
		return m.typeInto(list, "", true)
	case at.window >= 0 && at.pane >= 0:
		return m.split(domain.Right), nil
	}
	return m.typeInto(spot{window: len(m.project.Windows()), pane: -1, field: fieldName}, "", true)
}

// deleting is m after d: the item removed from its list, or the pane or
// window the cursor is in, once the user confirms.
func (m configForm) deleting() configForm {
	at := m.focus
	window, inWindow := m.windowAt(at.window)
	_, pane, inPane := m.paneAt(at)
	switch {
	case at.field == fieldEnv:
		if at.item < len(m.items(at)) {
			return m.changedTo(m.withVar(at, ""))
		}
	case isList(at.field):
		if items := m.items(at); at.item < len(items) {
			return m.changedTo(m.withItems(at, slices.Delete(items, at.item, at.item+1)))
		}
	case at.field == fieldProjectEnv:
		m.problem = "The project sets this variable: change it in the project's card."
	case inPane:
		changed, err := window.WithoutPane(at.pane)
		project, err := replaced(m.project, at.window, changed, err)
		if err != nil {
			return m.changedTo(project, err)
		}
		heir := 0 // the pane taking its space
		if split, ok := pane.Split(); ok {
			heir = split.Of()
		}
		back := fieldDrawing // or the pane list, if deleted from there
		if at.field == fieldPaneList {
			back = fieldPaneList
		}
		m.asking = &question{text: fmt.Sprintf("Delete %s?", paneHeading(pane)), yes: func(m configForm) (configForm, tea.Cmd) {
			return m.changedAt(project, nil, spot{window: at.window, pane: heir, field: back}), nil
		}}
	case inWindow && at.pane < 0:
		project, err := m.project.WithoutWindow(at.window)
		if err != nil {
			return m.changedTo(project, err)
		}
		next := at
		next.window = min(at.window, len(project.Windows())-1)
		m.asking = &question{
			text: fmt.Sprintf("Delete window %s and its panes?", window.Name()),
			yes:  func(m configForm) (configForm, tea.Cmd) { return m.changedAt(project, nil, next), nil },
		}
	}
	return m
}

// replaced is project with the window at index replaced by window, unless
// building window failed with err.
func replaced(project domain.Project, index int, window domain.Window, err error) (domain.Project, error) {
	if err != nil {
		return domain.Project{}, err
	}
	return project.ReplacingWindow(index, window)
}

// reordered is m after J (by 1) or K (by -1): the item or the window
// the cursor is on moved down or up, the cursor following it.
func (m configForm) reordered(by int) configForm {
	at := m.focus
	_, inWindow := m.windowAt(at.window)
	switch {
	case at.field == fieldEnv || at.field == fieldProjectEnv:
		m.problem = "Variables are listed by name."
	case isList(at.field):
		items, to := m.items(at), at.item+by
		if at.item >= len(items) || to < 0 || to >= len(items) {
			return m
		}
		items[at.item], items[to] = items[to], items[at.item]
		project, err := m.withItems(at, items)
		at.item = to
		return m.changedAt(project, err, at)
	case inWindow && at.pane < 0:
		to := at.window + by
		if to < 0 || to >= len(m.project.Windows()) {
			return m
		}
		project, err := m.project.MovingWindow(at.window, to)
		at.window = to
		return m.changedAt(project, err, at)
	}
	return m
}

// windowAt is the project's window at index, if there is one.
func (m configForm) windowAt(index int) (domain.Window, bool) {
	windows := m.project.Windows()
	if index < 0 || index >= len(windows) {
		return domain.Window{}, false
	}
	return windows[index], true
}

// paneAt is the window and pane at's row is in, if it is in a pane.
func (m configForm) paneAt(at spot) (domain.Window, domain.Pane, bool) {
	window, ok := m.windowAt(at.window)
	if !ok || at.pane < 0 || at.pane >= len(window.Panes()) {
		return domain.Window{}, domain.Pane{}, false
	}
	return window, window.Panes()[at.pane], true
}

// applied is the project with text typed at, or why text isn't valid
// there.
func (m configForm) applied(at spot, text string) (domain.Project, error) {
	switch {
	case at.window < 0 && at.field == fieldName:
		name, err := domain.NewProjectName(text)
		if err != nil {
			return domain.Project{}, err
		}
		return m.project.Named(name), nil
	case at.field == fieldRoot:
		root, err := parseRoot(text)
		if err != nil {
			return domain.Project{}, err
		}
		return m.project.RootedAt(root), nil
	case at.field == fieldEnv:
		return m.withVar(at, text)
	case at.field == fieldProjectEnv:
		return m.withVar(spot{window: at.window, pane: at.pane, field: fieldEnv, item: len(m.ownEnv(at).Names())}, text)
	case isList(at.field):
		return m.withItems(at, withItem(m.items(at), at.item, text))
	case at.window == len(m.project.Windows()):
		name, err := domain.NewWindowName(text)
		if err != nil {
			return domain.Project{}, err
		}
		window, err := domain.NewWindow(name)
		if err != nil {
			return domain.Project{}, err
		}
		return m.project.AddingWindow(window)
	case at.pane < 0:
		return m.windowChanged(at, func(window domain.Window) (domain.Window, error) {
			if at.field == fieldName {
				name, err := domain.NewWindowName(text)
				return window.Named(name), err
			}
			dir, err := toDir(strings.TrimSpace(text))
			return window.In(dir), err
		})
	}
	return m.paneChanged(at, func(pane domain.Pane) (domain.Pane, error) {
		switch at.field {
		case fieldName:
			name, err := optionalPaneName(text)
			return pane.Named(name), err
		case fieldDir:
			if strings.TrimSpace(text) == "" {
				return pane.InWindowDir(), nil
			}
			dir, err := toDir(strings.TrimSpace(text))
			return pane.In(dir), err
		}
		if strings.TrimSpace(text) == "" {
			return pane.WithoutCommand(), nil
		}
		cmd, err := domain.NewCommand(text)
		if m.edit.run {
			return pane.Running(cmd), err
		}
		return pane.Prefilling(cmd), err
	})
}

// windowChanged is the project with the window at's row is in changed.
func (m configForm) windowChanged(at spot, change func(domain.Window) (domain.Window, error)) (domain.Project, error) {
	window, ok := m.windowAt(at.window)
	if !ok {
		return domain.Project{}, fmt.Errorf("%w: #%d", domain.ErrWindowNotFound, at.window+1)
	}
	changed, err := change(window)
	return replaced(m.project, at.window, changed, err)
}

// paneChanged is the project with the pane at's row is in changed.
func (m configForm) paneChanged(at spot, change func(domain.Pane) (domain.Pane, error)) (domain.Project, error) {
	return m.windowChanged(at, func(window domain.Window) (domain.Window, error) {
		panes := window.Panes()
		if at.pane < 0 || at.pane >= len(panes) {
			return domain.Window{}, fmt.Errorf("%w: #%d", domain.ErrPaneNotFound, at.pane+1)
		}
		changed, err := change(panes[at.pane])
		if err != nil {
			return domain.Window{}, err
		}
		return window.ReplacingPane(at.pane, changed)
	})
}

// items are the items of the list at's row is in, as typed.
func (m configForm) items(at spot) []string {
	window, _ := m.windowAt(at.window)
	_, pane, _ := m.paneAt(at)
	switch {
	case at.field == fieldEnv:
		return envItems(m.ownEnv(at))
	case at.window < 0:
		return lines(m.project.Before())
	case at.pane < 0:
		return lines(window.Before())
	case at.field == fieldNote:
		return pane.Note()
	}
	return lines(pane.Before())
}

// envItems are env's variables as typed, "NAME=value", by name.
func envItems(env domain.Env) []string {
	var items []string
	for _, v := range env.Vars() {
		items = append(items, v.Name()+"="+v.Value())
	}
	return items
}

// withVar is the project with the variable on the row at replaced by the
// one text sets, "NAME=value": blank removes it, and past the last
// variable, text adds one. The value is kept as typed, and the other
// variables as they are.
func (m configForm) withVar(at spot, text string) (domain.Project, error) {
	env := m.ownEnv(at)
	if names := env.Names(); at.item < len(names) {
		env = env.Without(names[at.item])
	}
	if strings.TrimSpace(text) != "" {
		name, value, ok := strings.Cut(text, "=")
		name = strings.TrimSpace(name)
		switch {
		case !ok:
			return domain.Project{}, fmt.Errorf("%q: want NAME=value", strings.TrimSpace(text))
		case slices.Contains(env.Names(), name):
			return domain.Project{}, fmt.Errorf("%s: set twice", name)
		}
		var err error
		if env, err = env.Setting(name, value); err != nil {
			return domain.Project{}, err
		}
	}
	if at.window < 0 {
		return m.project.WithEnv(env), nil
	}
	return m.paneChanged(at, func(pane domain.Pane) (domain.Pane, error) { return pane.WithEnv(env), nil })
}

// ownEnv is the variables the project sets, for a row of its card, or
// the pane at's row is in sets itself.
func (m configForm) ownEnv(at spot) domain.Env {
	if at.window < 0 {
		return m.project.Env()
	}
	_, pane, _ := m.paneAt(at)
	return pane.Env()
}

// withItems is the project with the list at's row is in holding items.
func (m configForm) withItems(at spot, items []string) (domain.Project, error) {
	switch at.field {
	case fieldNote:
		return m.paneChanged(at, func(pane domain.Pane) (domain.Pane, error) { return pane.WithNote(items...), nil })
	}
	cmds, err := parseCommands(strings.Join(items, "\n"))
	switch {
	case err != nil:
		return domain.Project{}, err
	case at.window < 0:
		return m.project.RunningFirst(cmds...), nil
	case at.pane < 0:
		return m.windowChanged(at, func(window domain.Window) (domain.Window, error) { return window.RunningFirst(cmds...), nil })
	}
	return m.paneChanged(at, func(pane domain.Pane) (domain.Pane, error) { return pane.RunningFirst(cmds...), nil })
}

// withItem is items with the one at index replaced by text, or text added
// when index is past the last; blank text removes the item.
func withItem(items []string, index int, text string) []string {
	items = slices.Clone(items)
	blank := strings.TrimSpace(text) == ""
	switch {
	case blank && index < len(items):
		return slices.Delete(items, index, index+1)
	case blank:
		return items
	case index < len(items):
		items[index] = text
		return items
	}
	return append(items, text)
}

// current is what the row at holds, as typed.
func (m configForm) current(at spot) string {
	root := m.project.Root()
	window, _ := m.windowAt(at.window)
	_, pane, inPane := m.paneAt(at)
	switch {
	case at.window < 0 && at.field == fieldName:
		return m.project.Name().String()
	case at.field == fieldRoot:
		return contractHome(root.String())
	case isList(at.field):
		if items := m.items(at); at.item < len(items) {
			return items[at.item]
		}
		return ""
	case at.field == fieldProjectEnv:
		return envItems(m.projectEnv(at))[at.item]
	case !inPane && at.field == fieldName:
		return window.Name().String()
	case !inPane:
		return dirOf(root, window.Dir())
	case at.field == fieldName:
		return pane.Name().String()
	case at.field == fieldDir:
		return paneDirOf(root, window, pane)
	}
	cmd, _ := pane.Startup().Command()
	return cmd.String()
}

// projectEnv are the variables the project sets that the pane at's row
// is in doesn't set itself.
func (m configForm) projectEnv(at spot) domain.Env {
	return m.project.Env().Without(m.ownEnv(at).Names()...)
}

// section is lines of the page, the rows the cursor can be on among
// them, in order, and the first and last lines of the row the cursor is
// on, or -1.
type section struct {
	lines             []string
	spots             []spot
	cursor, cursorEnd int
	hits              []hit
}

// hit is a row's line a click acts on: the spots of the items on it,
// each picking its window or pane at the row, or how its pane starts.
type hit struct {
	line  int
	at    spot
	spots []hotspot
}

func newSection() section {
	return section{cursor: -1, cursorEnd: -1}
}

// row adds a line the cursor can be on.
func (s *section) row(line string, at spot, focused bool) {
	s.block([]string{line}, at, focused)
}

// block adds lines the cursor can be on as one row.
func (s *section) block(lines []string, at spot, focused bool) {
	if focused {
		s.cursor, s.cursorEnd = len(s.lines), len(s.lines)+len(lines)-1
	}
	s.lines = append(s.lines, lines...)
	s.spots = append(s.spots, at)
}

// listRow adds a row of a list, its items at spots, offset cells in.
func (s *section) listRow(line string, at spot, focused bool, offset int, spots []hotspot) {
	s.hits = append(s.hits, hit{line: len(s.lines), at: at, spots: shifted(spots, offset)})
	s.row(line, at, focused)
}

// shifted is spots moved by cells to the right.
func shifted(spots []hotspot, cells int) []hotspot {
	moved := make([]hotspot, len(spots))
	for i, spot := range spots {
		spot.from, spot.to = spot.from+cells, spot.to+cells
		moved[i] = spot
	}
	return moved
}

// text adds a line the cursor can't be on.
func (s *section) text(line string) {
	s.lines = append(s.lines, line)
}

// boxed adds child in a card titled title, width cells wide.
func (s *section) boxed(title string, child section, width int, heavy bool) {
	if child.cursor >= 0 { // below the card's top border
		s.cursor, s.cursorEnd = len(s.lines)+1+child.cursor, len(s.lines)+1+child.cursorEnd
	}
	for _, h := range child.hits { // inside the card's top and left borders
		h.line += len(s.lines) + 1
		h.spots = shifted(h.spots, 2)
		s.hits = append(s.hits, h)
	}
	s.lines = append(s.lines, card(title, child.lines, width, heavy)...)
	s.spots = append(s.spots, child.spots...)
}

// card is body in a box width cells wide, title in its top border, heavy
// for the card being edited.
func card(title string, body []string, width int, heavy bool) []string {
	border := lipgloss.RoundedBorder()
	if heavy {
		border = lipgloss.ThickBorder()
	}
	inner := max(width-4, 1)
	head := ansi.Truncate(border.TopLeft+border.Top+" "+title+" ", max(width-1, 0), "")
	head += strings.Repeat(border.Top, max(width-1-ansi.StringWidth(head), 0)) + border.TopRight
	out := []string{head}
	for _, line := range body {
		out = append(out, border.Left+" "+padded(ansi.Truncate(line, inner, "…"), inner)+" "+border.Right)
	}
	return append(out, border.BottomLeft+strings.Repeat(border.Bottom, max(width-2, 0))+border.BottomRight)
}

// gutterWidth and labelWidth are the cells before a row's value: the
// cursor's mark, then the field's label.
const gutterWidth, labelWidth = 2, 10

// put adds a row labelled label showing value, or the field typed into
// it, then after, and below it why what is typed isn't valid.
func (m configForm) put(s *section, label, value, after string, at spot) {
	focused := m.focus == at
	typed := m.editing && m.edit.at == at
	shown := padded(label, labelWidth)
	if focused {
		shown = cursorStyle.Render(label) + strings.Repeat(" ", max(labelWidth-ansi.StringWidth(label), 0))
	}
	if typed {
		value = m.edit.input.View()
	}
	line := gutter(focused) + shown + value
	if after != "" {
		line += "  " + after
	}
	s.row(line, at, focused)
	if typed && m.edit.err != nil {
		s.text(strings.Repeat(" ", gutterWidth+labelWidth) + problemStyle.Render("✗ "+m.edit.err.Error()))
	}
}

// shownText adds a line under label the cursor can't be on.
func shownText(s *section, label, value string) {
	s.text(strings.Repeat(" ", gutterWidth) + padded(label, labelWidth) + value)
}

// list adds items under label, a row each, then the row adding one.
func (m configForm) list(s *section, label string, items []string, at spot) {
	for i, item := range items {
		at.item = i
		m.put(s, label, item, "", at)
		label = ""
	}
	at.item = len(items)
	m.put(s, label, keysStyle.Render("+ add"), "", at)
}

// shown is the project with what is typed, when it is valid and shows
// where it is typed, else the project.
func (m configForm) shown() domain.Project {
	if m.editing && m.edit.err == nil && !isList(m.edit.at.field) && m.edit.at.field != fieldProjectEnv {
		return m.edit.preview
	}
	return m.project
}

// mark tells whether dir exists, or nothing when it can't be told.
func (m configForm) mark(dir string) string {
	exists, err := m.workspace.Exists(dir)
	switch {
	case err != nil:
		return ""
	case exists:
		return checkStyle.Render("✓")
	}
	return problemStyle.Render("✗ missing")
}

// selection is the window shown, the entry after the last when there is
// none or the cursor adds one, and its pane shown: those the cursor is
// in, else those it was in last.
func (m configForm) selection() spot {
	picked := m.picked
	if m.focus.window >= 0 {
		if m.focus.window != picked.window {
			picked.pane = 0
		}
		picked.window = m.focus.window
		if m.focus.pane >= 0 {
			picked.pane = m.focus.pane
		}
	}
	windows := m.shown().Windows()
	picked.window = max(min(picked.window, len(windows)), 0)
	if picked.window < len(windows) {
		picked.pane = max(min(picked.pane, len(windows[picked.window].Panes())-1), 0)
	}
	return picked
}

// layout is the page: the project's card, then the window list and the
// windows' card.
func (m configForm) layout() section {
	shown := m
	shown.project = m.shown()
	width := max(m.width, 30)
	page := newSection()
	page.boxed("project", shown.projectCard(), width, m.focus.window < 0)
	picked := shown.selection()
	listAt := spot{window: picked.window, pane: -1, field: fieldWindowList}
	focused := m.focus == listAt
	line, spots := formWindowList(shown.project.Windows(), picked.window, width-gutterWidth)
	page.listRow(gutter(focused)+line, listAt, focused, gutterWidth, spots)
	page.boxed("windows", shown.windowsCard(width-4), width, m.focus.window >= 0)
	return page
}

// gutter is the cells before a row: the cursor's mark when focused.
func gutter(focused bool) string {
	if focused {
		return "› "
	}
	return "  "
}

// clicked is m after a click at (x, y) on a window in the window list
// or a pane in the pane list, the cursor going there as the arrows take
// it, or on how a pane starts, as if picked with ←/→; elsewhere, m as it
// was.
func (m configForm) clicked(x, y int) (configForm, tea.Cmd) {
	line := m.top + y - 1 // below the trail
	if y < 1 || y > m.bodyHeight() {
		return m, nil
	}
	for _, h := range m.layout().hits {
		if h.line != line {
			continue
		}
		spot, ok := hotspotAt(h.spots, x)
		if !ok {
			return m, nil
		}
		at := h.at
		switch at.field {
		case fieldStarts:
			return m.startsClicked(at, spot.item)
		case fieldClear:
			m.focus = at
			return m.changedTo(m.cleared(at, clearings[spot.item])), nil
		case fieldWindowList:
			at.window = spot.item
		default:
			at.pane = spot.item
		}
		m.focus = at
		return m.settled(1), nil
	}
	return m, nil
}

// projectCard is the project's name, root, before commands and env.
func (m configForm) projectCard() section {
	p := m.project
	s := newSection()
	if m.nameStays {
		shownText(&s, "name", keysStyle.Render(p.Name().String()))
	} else {
		name := p.Name().String()
		if m.unnamed {
			name = keysStyle.Render("none yet")
		}
		m.put(&s, "name", name, "", projectNameAt)
	}
	m.put(&s, "root", contractHome(p.Root().String()), m.mark(p.Root().String()), spot{window: -1, pane: -1, field: fieldRoot})
	m.list(&s, "before", lines(p.Before()), spot{window: -1, pane: -1, field: fieldBefore})
	m.putClear(&s, spot{window: -1, pane: -1, field: fieldClear})
	label := "env"
	vars := p.Env().Vars()
	for k, v := range vars {
		m.put(&s, label, varText(v), "", spot{window: -1, pane: -1, field: fieldEnv, item: k})
		label = ""
	}
	m.put(&s, label, keysStyle.Render("+ add"), "", spot{window: -1, pane: -1, field: fieldEnv, item: len(vars)})
	return s
}

// windowsCard is the selected window's name, dir and before commands,
// its grid, its panes drawn, the pane list, then the selected pane's
// card, all within width cells. On "+ window", it is the row typing a
// new window's name, or, before typing, how to start.
func (m configForm) windowsCard(width int) section {
	picked := m.selection()
	windows := m.project.Windows()
	s := newSection()
	if picked.window == len(windows) {
		if at := (spot{window: picked.window, pane: -1, field: fieldName}); m.editing && m.edit.at == at {
			m.put(&s, "name", "", "", at)
		} else {
			s.text(strings.Repeat(" ", gutterWidth) + keysStyle.Render("enter names a new window"))
		}
		return s
	}

	root := m.project.Root()
	window := windows[picked.window]
	at := func(f field) spot { return spot{window: picked.window, pane: -1, field: f} }
	m.put(&s, "name", window.Name().String(), "", at(fieldName))
	m.put(&s, "dir", shownDir(root, window.Dir()), m.mark(root.Resolve(window.Dir())), at(fieldDir))
	m.list(&s, "before", lines(window.Before()), at(fieldBefore))
	m.putClear(&s, at(fieldClear))
	grid, gridded := window.Grid()
	if gridded {
		rows := make([]string, 0, len(grid.Areas()))
		for _, row := range grid.Areas() {
			names := make([]string, 0, len(row))
			for _, name := range row {
				names = append(names, name.String())
			}
			rows = append(rows, strings.Join(names, " "))
		}
		shownText(&s, "grid", keysStyle.Render(strings.Join(rows, " / ")))
	}
	s.text("")

	drawingAt := spot{window: picked.window, pane: picked.pane, field: fieldDrawing}
	focused := m.focus == drawingAt
	plan := m.project.Plan().Windows()[picked.window]
	// The page is at least 30 cells wide, which leaves the drawing at least
	// its least width. A session opens at the terminal's size, less tmux's
	// status line.
	drawWidth := width - gutterWidth
	height := max(min(drawWidth*(m.height-1)/max(m.width, 1), m.bodyHeight()/3), minDrawingHeight)
	drawn := drawing(plan, root.String(), nil, picked.pane, focused, m.width, max(m.height-1, 1), drawWidth, height)
	for i, line := range drawn {
		drawn[i] = strings.Repeat(" ", gutterWidth) + line
	}
	s.block(drawn, drawingAt, focused)
	listAt := spot{window: picked.window, pane: picked.pane, field: fieldPaneList}
	focused = m.focus == listAt
	line, spots := paneList(window, picked.pane, width-gutterWidth)
	s.listRow(gutter(focused)+line, listAt, focused, gutterWidth, spots)
	s.text("")
	pane := window.Panes()[picked.pane]

	inPane := m.focus.window == picked.window && m.focus.pane == picked.pane && m.focus.field != fieldDrawing
	s.boxed(paneHeading(pane), m.paneCard(picked.window, picked.pane), width, inPane)
	return s
}

// formWindowList is the windows' names, then "+ window" adding one, the
// selected one in a cyan block, with the cells each covers, to click it.
// When they don't fit in width, the first ones make way for the selected
// one.
func formWindowList(windows []domain.Window, selected int, width int) (string, []hotspot) {
	labels := make([]string, 0, len(windows)+1)
	for _, window := range windows {
		labels = append(labels, " "+window.Name().String()+" ")
	}
	labels = append(labels, " + window ")
	for i, label := range labels {
		switch {
		case i == selected:
			labels[i] = selectedStyle.Render(label)
		case i == len(windows):
			labels[i] = keysStyle.Render(label)
		}
	}
	return listed(labels, selected, width)
}

// paneList is the names of window's panes, or their numbers for
// those without, the selected one in a cyan block, with the cells each
// covers, to click it. When they don't fit in width, the first ones make
// way for the selected one.
func paneList(window domain.Window, selected int, width int) (string, []hotspot) {
	labels := make([]string, 0, len(window.Panes()))
	for i, pane := range window.Panes() {
		name := pane.Name().String()
		if name == "" {
			name = strconv.Itoa(i + 1)
		}
		if i == selected {
			labels = append(labels, selectedStyle.Render(" "+name+" "))
		} else {
			labels = append(labels, " "+name+" ")
		}
	}
	return listed(labels, selected, width)
}

// placement is how pane is placed in window: by its grid, or splitting
// another pane, which way and by how much.
func placement(window domain.Window, pane domain.Pane) string {
	if _, gridded := window.Grid(); gridded {
		return "placed by the grid"
	}
	split, ok := pane.Split()
	switch {
	case !ok && len(window.Panes()) == 1:
		return "fills the window"
	case !ok:
		return "takes what the others leave"
	}
	size := split.Size().String()
	switch {
	case split.Size().IsDefault():
		size = "half"
	case !strings.HasSuffix(size, "%"):
		size += " cells"
	}
	arrow := "→"
	if split.Direction() == domain.Below {
		arrow = "↓"
	}
	return fmt.Sprintf("splits %s %s %s", window.Panes()[split.Of()].Name(), arrow, size)
}

// paneCard is a pane's fields, its project's and window's before
// commands and its project's variables dimmed among its own, with the
// level they come from.
func (m configForm) paneCard(index, j int) section {
	p := m.project
	root := p.Root()
	window := p.Windows()[index]
	pane := window.Panes()[j]
	at := func(f field) spot { return spot{window: index, pane: j, field: f} }
	s := newSection()
	s.text(strings.Repeat(" ", gutterWidth) + keysStyle.Render(placement(window, pane)))

	name := pane.Name().String()
	if name == "" {
		name = keysStyle.Render("none: the window's only pane")
	}
	m.put(&s, "name", name, "", at(fieldName))
	dir, own := pane.Dir()
	value := keysStyle.Render("window's: " + shownDir(root, window.Dir()))
	if !own {
		dir = window.Dir()
	} else {
		value = paneDirOf(root, window, pane)
		if value == "" {
			value = shownDir(root, dir)
		}
	}
	m.put(&s, "dir", value, m.mark(root.Resolve(dir)), at(fieldDir))
	mode := startMode(pane)
	if m.editing && m.edit.at == at(fieldCommand) {
		mode = m.typedMode()
	}
	m.putChoices(&s, "starts", startModes, mode, at(fieldStarts))
	if cmd, ok := pane.Startup().Command(); ok || (m.editing && m.edit.at == at(fieldCommand)) {
		m.put(&s, "command", cmd.String(), "", at(fieldCommand))
	}

	label := "before"
	for _, line := range higherBefore(p, window) {
		shownText(&s, label, keysStyle.Render(line.value+"  "+line.level))
		label = ""
	}
	m.list(&s, label, lines(pane.Before()), at(fieldBefore))
	m.putClear(&s, at(fieldClear))

	label = "env"
	vars := pane.Env().Vars()
	for k, v := range vars {
		m.put(&s, label, varText(v), keysStyle.Render(overProject(p, v.Name())), spot{window: index, pane: j, field: fieldEnv, item: k})
		label = ""
	}
	for k, v := range m.projectEnv(at(fieldEnv)).Vars() {
		m.put(&s, label, keysStyle.Render(varText(v)), keysStyle.Render("project"),
			spot{window: index, pane: j, field: fieldProjectEnv, item: k})
		label = ""
	}
	m.put(&s, label, keysStyle.Render("+ add"), "", spot{window: index, pane: j, field: fieldEnv, item: len(vars)})

	m.list(&s, "note", pane.Note(), at(fieldNote))
	return s
}

// varText is v as shown, "NAME=value", its value quoted when it would
// break the line.
func varText(v domain.EnvVar) string {
	return v.Name() + "=" + shownValue(v.Value())
}

// paneHeading is "pane" and the pane's name, if it has one, without
// saying "pane" twice for a default name such as "pane 2".
func paneHeading(pane domain.Pane) string {
	name := pane.Name().String()
	switch {
	case name == "":
		return "pane"
	case strings.HasPrefix(name, "pane "):
		return name
	}
	return "pane " + name
}

// shownDir is dir as a config writes it, or "root" for the root.
func shownDir(root domain.ProjectRoot, dir domain.PaneDir) string {
	if written := dirOf(root, dir); written != "" {
		return written
	}
	return "root"
}

// putClear adds the clear row at, for the project, a window or a pane.
func (m configForm) putClear(s *section, at spot) {
	m.putChoices(s, "clear", clearOptions(at), clearIndex(m.clearingAt(at)), at)
}

// putChoices adds a row labelled label showing options side by side,
// the selected one highlighted, each clicked to pick it.
func (m configForm) putChoices(s *section, label string, options []string, selected int, at spot) {
	focused := m.focus == at
	shown := padded(label, labelWidth)
	if focused {
		shown = cursorStyle.Render(label) + strings.Repeat(" ", max(labelWidth-ansi.StringWidth(label), 0))
	}
	labels := make([]string, len(options))
	for i, option := range options {
		if i == selected {
			labels[i] = selectedStyle.Render(" " + option + " ")
		} else {
			labels[i] = keysStyle.Render(" " + option + " ")
		}
	}
	line, spots := listed(labels, selected, max(m.width-4*2-gutterWidth-labelWidth, 1))
	s.listRow(gutter(focused)+shown+line, at, focused, gutterWidth+labelWidth, spots)
}

// scrolled is the first line shown, keeping the row the cursor is on and
// the line below it, where an error shows, in view, or its first lines
// when it is too tall for that.
func (m configForm) scrolled() int {
	page := m.layout()
	height := m.bodyHeight()
	first, last := max(page.cursor, 0), max(page.cursorEnd, 0)
	top := min(m.top, first, max(len(page.lines)-height, 0))
	return max(min(max(top, last+2-height), first), 0)
}

// bodyHeight is the lines left for the cards, below the trail and above
// the status and the keys.
func (m configForm) bodyHeight() int {
	return max(m.height-3, 1)
}

func (m configForm) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// render is the trail, the cards in view, the status line and the keys.
func (m configForm) render() string {
	if m.width == 0 {
		return "" // until the terminal's size is known
	}
	height := m.bodyHeight()
	lines, top := m.layout().lines, m.top
	if m.reviewing != nil {
		lines, top = m.reviewing.styled(m.width), m.reviewing.top
	}
	shown := lines[min(top, len(lines)):min(top+height, len(lines))]
	body := strings.Join(shown, "\n") + strings.Repeat("\n", height-len(shown))
	return m.trail() + "\n" + body + "\n" + ansi.Truncate(m.status(), m.width, "…") + "\n" + m.keys()
}

// styled is the diff's lines, removed ones red and added ones green,
// within width cells, without the carriage returns of a config with
// Windows line ends.
func (r review) styled(width int) []string {
	lines := make([]string, 0, len(r.diff))
	for _, line := range r.diff {
		style := keysStyle
		switch line[0] {
		case '-':
			style = problemStyle
		case '+':
			style = checkStyle
		}
		lines = append(lines, style.Render(ansi.Truncate(" "+strings.TrimSuffix(line, "\r"), width, "…")))
	}
	return lines
}

// trail is where the cursor is, and whether there are changes to save.
func (m configForm) trail() string {
	project := m.shown()
	parts := []string{project.Name().String()}
	if m.unnamed && project.Name() == standInName {
		parts[0] = "new project"
	}
	windows := project.Windows()
	if f := m.focus; f.window >= 0 && f.window < len(windows) {
		window := windows[f.window]
		parts = append(parts, "window "+window.Name().String())
		if panes := window.Panes(); f.pane >= 0 && f.pane < len(panes) {
			parts = append(parts, paneHeading(panes[f.pane]))
		}
	}
	line := titleStyle.Render(" " + strings.Join(parts, " › "))
	if !m.dirty() {
		return ansi.Truncate(line, m.width, "…")
	}
	unsaved := warningStyle.Render("● unsaved ")
	gap := max(m.width-ansi.StringWidth(line)-ansi.StringWidth(unsaved), 1)
	return ansi.Truncate(line+strings.Repeat(" ", gap)+unsaved, m.width, "…")
}

// status is what saving changes being shown, the question asked, or why
// the last change was refused.
func (m configForm) status() string {
	switch {
	case m.reviewing != nil && m.reviewing.replaces:
		return fmt.Sprintf(" Replace %s's registered config with this?", m.project.Name()) + keysStyle.Render("  y save · n back")
	case m.reviewing != nil && m.reviewing.unstored:
		return " Save the new project's config?" + keysStyle.Render("  y save · n back")
	case m.reviewing != nil:
		return " Save these changes to the config?" + keysStyle.Render("  y save · n back")
	case m.asking != nil:
		return " " + m.asking.text + keysStyle.Render("  y yes · n no")
	case m.problem != "":
		return " " + problemStyle.Render("✗ "+m.problem)
	}
	return ""
}

// keys are what the keys do, while typing or on a row.
func (m configForm) keys() string {
	told := "↑↓ move · enter edit · ←→ change · o add · d delete · J/K reorder · u undo · ctrl+r redo · ctrl+s save · esc leave"
	switch {
	case m.focus.field == fieldDrawing && !m.editing && m.reviewing == nil:
		told = "←→↑↓ select · % split right · \" split below · x remove · H/L resize · enter edit · u undo · ctrl+s save · esc leave"
		if window, ok := m.windowAt(m.focus.window); ok {
			if _, gridded := window.Grid(); gridded {
				told = "←→↑↓ select · enter edit · u undo · ctrl+s save · esc leave · the grid places these panes"
			}
		}
	case m.reviewing != nil:
		told = "↑↓ scroll · y save · n back to the form"
	case m.editing:
		told = "enter keep · esc cancel · ctrl+s save"
		if m.edit.at.field == fieldCommand && m.edit.input.Value() == "" {
			told = "←→ starts · " + told
		}
	}
	return ansi.Truncate(keysStyle.Render(" "+told), m.width, "…")
}
