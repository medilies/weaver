package infra

import (
	"bytes"
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
// cards, one for the project, then one per window holding one per pane,
// changed in place. Each field is checked by the domain's constructors
// as it is typed, and each change is made by the domain's own methods:
// the form only collects them.
type TeaConfigForm struct {
	workspace domain.Workspace
}

// NewTeaConfigForm constructs a TeaConfigForm checking directories in
// workspace.
func NewTeaConfigForm(workspace domain.Workspace) TeaConfigForm {
	return TeaConfigForm{workspace: workspace}
}

// Fill implements app.ProjectForm. The draft's name stays. It returns
// the project as ctrl+s saves it, or ErrAborted when the user leaves it
// unchanged or discards their changes.
func (f TeaConfigForm) Fill(draft app.ProjectDraft) (domain.Project, error) {
	name, err := domain.NewProjectName(draft.Name)
	if err != nil {
		return domain.Project{}, err
	}
	root, err := parseRoot(draft.Root)
	if err != nil {
		return domain.Project{}, err
	}
	project, err := domain.NewProject(name, root).WithEnv(draft.Env).RunningFirst(draft.Before...).
		WithWindows(draft.Windows...)
	if err != nil {
		return domain.Project{}, err
	}
	original, err := encodeProject(project)
	if err != nil {
		return domain.Project{}, err
	}
	model := configForm{
		project: project, original: original, workspace: f.workspace,
		focus: spot{window: -1, pane: -1, field: fieldRoot},
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
	fieldSplitOf
	fieldDirection
	fieldSize
	fieldStarts
	fieldCommand
	fieldFolded // a pane folded to one line
)

// isList reports whether f holds a list, a row per item.
func isList(f field) bool {
	return f == fieldBefore || f == fieldEnv || f == fieldNote
}

// spot is a row the cursor can be on: a field of the project (window
// -1), of a window (pane -1) or of a pane, and in a list, the item, its
// length for the row adding one. The name of the window after the last,
// or of the pane after a window's last, is the row adding one.
type spot struct {
	window, pane int
	field        field
	item         int
}

// configForm is TeaConfigForm's Bubble Tea model. The project is always
// valid: a field being typed changes it only once enter keeps it.
type configForm struct {
	project       domain.Project
	original      []byte // the project as its config writes it, to tell changes
	workspace     domain.Workspace
	focus         spot
	top           int // the first line shown
	editing       bool
	edit          typing
	asking        *question
	problem       string // why the last change was refused
	width, height int
	saved         bool
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
	return nil
}

// Update is m after msg, scrolled to keep the cursor in view.
func (m configForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	next.top = next.scrolled()
	return next, cmd
}

func (m configForm) update(msg tea.Msg) (configForm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.edit.input.SetWidth(m.inputWidth(m.edit.at))
		return m, nil
	case tea.KeyPressMsg:
		switch {
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
	m.project, m.editing = project, false
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
	case "ctrl+s":
		return m.save()
	case "esc", "ctrl+c":
		return m.leave()
	}
	return m, nil
}

// save ends the form, with the project saved if it changed.
func (m configForm) save() (configForm, tea.Cmd) {
	m.saved = m.dirty()
	return m, tea.Quit
}

// leave ends the form without saving, asking first when it changed.
func (m configForm) leave() (configForm, tea.Cmd) {
	if !m.dirty() {
		return m, tea.Quit
	}
	m.asking = &question{text: "Discard your changes?", yes: func(m configForm) (configForm, tea.Cmd) {
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
	if err != nil {
		m.problem = err.Error()
		return m
	}
	m.project = project
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
		if m.focus.field == fieldFolded {
			return m.settled(-1) // in the pane, now open
		}
	}
	return m
}

// stepped is m after ← (by -1) or → (by 1) on a choice: the pane split,
// the direction, the size by 5%, or how the pane starts.
func (m configForm) stepped(by int) (configForm, tea.Cmd) {
	at := m.focus
	switch at.field {
	case fieldSplitOf, fieldDirection, fieldSize:
		return m.changedTo(m.paneChanged(at, func(pane domain.Pane) (domain.Pane, error) {
			split, _ := pane.Split()
			of, direction, size := split.Of(), split.Direction(), split.Size()
			switch at.field {
			case fieldSplitOf:
				of = ((of+by)%at.pane + at.pane) % at.pane
			case fieldDirection:
				direction = map[domain.Direction]domain.Direction{domain.Right: domain.Below, domain.Below: domain.Right}[direction]
			case fieldSize:
				var err error
				if size, err = domain.Percent(steppedPercent(size, by)); err != nil {
					return domain.Pane{}, err
				}
			}
			changed, err := domain.NewSplit(of, direction, size)
			return pane.Splitting(changed), err
		})), nil
	case fieldStarts:
		return m.started(by)
	}
	return m, nil
}

// steppedPercent is size moved to the next multiple of 5% up, or down
// when by is negative, from half when it isn't a percentage.
func steppedPercent(size domain.Size, by int) int {
	n := 50
	if digits, ok := strings.CutSuffix(size.String(), "%"); ok {
		n, _ = strconv.Atoi(digits)
	}
	if by > 0 {
		n = (n/5 + 1) * 5
	} else {
		n = ((n+4)/5 - 1) * 5
	}
	return max(min(n, 99), 1)
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

// entered is m after enter on a row: a choice goes to its next option,
// the row adding a pane adds one, and other rows are typed into.
func (m configForm) entered() (configForm, tea.Cmd) {
	at := m.focus
	switch at.field {
	case fieldSplitOf, fieldDirection, fieldStarts:
		return m.stepped(1)
	}
	if window, ok := m.windowAt(at.window); ok && at.field == fieldName && at.pane == len(window.Panes()) {
		return m.paneAdded(at.window, at.pane), nil
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

// added is m after o: an item typed into the list the cursor is in, a
// pane added to the window it is in, or a window typed after the last.
func (m configForm) added() (configForm, tea.Cmd) {
	at := m.focus
	windows := m.project.Windows()
	switch {
	case isList(at.field) || at.field == fieldProjectEnv:
		list := spot{window: at.window, pane: at.pane, field: at.field}
		if list.field == fieldProjectEnv {
			list.field = fieldEnv
		}
		list.item = len(m.items(list))
		return m.typeInto(list, "", true)
	case at.window < 0 || at.window >= len(windows):
		return m.typeInto(spot{window: len(windows), pane: -1, field: fieldName}, "", true)
	}
	return m.paneAdded(at.window, at.pane), nil
}

// paneAdded is m with a pane added to the window at index, splitting the
// pane at after, or the last when after isn't a pane, with the cursor on
// its name.
func (m configForm) paneAdded(index, after int) configForm {
	window, _ := m.windowAt(index)
	panes := window.Panes()
	of := len(panes) - 1
	if after >= 0 && after < len(panes) {
		of = after
	}
	split, err := domain.NewSplit(of, domain.Right, domain.DefaultSize())
	if err != nil {
		return m.changedTo(domain.Project{}, err)
	}
	added, err := window.AddingPane(domain.NewPane().Splitting(split))
	if err != nil {
		return m.changedTo(domain.Project{}, err)
	}
	m.focus = spot{window: index, pane: len(panes), field: fieldName}
	return m.changedTo(m.project.ReplacingWindow(index, added))
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
		m.asking = &question{text: fmt.Sprintf("Delete %s?", paneHeading(pane)), yes: func(m configForm) (configForm, tea.Cmd) {
			m.focus = spot{window: at.window, pane: max(at.pane-1, 0), field: fieldName}
			return m.changedTo(project, nil), nil
		}}
	case inWindow && at.pane < 0:
		project, err := m.project.WithoutWindow(at.window)
		if err != nil {
			return m.changedTo(project, err)
		}
		m.asking = &question{
			text: fmt.Sprintf("Delete window %s and its panes?", window.Name()),
			yes:  func(m configForm) (configForm, tea.Cmd) { return m.changedTo(project, nil), nil },
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

// reordered is m after J (by 1) or K (by -1): the item, the pane or the
// window the cursor is on moved down or up, the cursor following it.
func (m configForm) reordered(by int) configForm {
	at := m.focus
	window, inWindow := m.windowAt(at.window)
	_, _, inPane := m.paneAt(at)
	switch {
	case at.field == fieldEnv || at.field == fieldProjectEnv:
		m.problem = "Variables are listed by name."
	case isList(at.field):
		items, to := m.items(at), at.item+by
		if at.item >= len(items) || to < 0 || to >= len(items) {
			return m
		}
		items[at.item], items[to] = items[to], items[at.item]
		m.focus.item = to
		return m.changedTo(m.withItems(at, items))
	case inPane:
		to := at.pane + by
		if to < 0 || to >= len(window.Panes()) {
			return m
		}
		moved, err := window.MovingPane(at.pane, to)
		project, err := replaced(m.project, at.window, moved, err)
		if err == nil {
			m.focus.pane = to
		}
		return m.changedTo(project, err)
	case inWindow && at.pane < 0:
		to := at.window + by
		if to < 0 || to >= len(m.project.Windows()) {
			return m
		}
		project, err := m.project.MovingWindow(at.window, to)
		if err == nil {
			m.focus.window = to
		}
		return m.changedTo(project, err)
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
		case fieldSize:
			size, err := domain.ParseSize(text)
			if err != nil {
				return domain.Pane{}, err
			}
			split, _ := pane.Split()
			changed, err := domain.NewSplit(split.Of(), split.Direction(), size)
			return pane.Splitting(changed), err
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
	case at.field == fieldSize:
		split, _ := pane.Split()
		return split.Size().String()
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
// them, in order, and the line the cursor is on, or -1.
type section struct {
	lines  []string
	spots  []spot
	cursor int
}

func newSection() section {
	return section{cursor: -1}
}

// row adds a line the cursor can be on.
func (s *section) row(line string, at spot, focused bool) {
	if focused {
		s.cursor = len(s.lines)
	}
	s.lines = append(s.lines, line)
	s.spots = append(s.spots, at)
}

// text adds a line the cursor can't be on.
func (s *section) text(line string) {
	s.lines = append(s.lines, line)
}

// boxed adds child in a card titled title, width cells wide.
func (s *section) boxed(title string, child section, width int, heavy bool) {
	if child.cursor >= 0 {
		s.cursor = len(s.lines) + 1 + child.cursor // below the card's top border
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
	gutter, shown := "  ", padded(label, labelWidth)
	if focused {
		gutter = "› "
		shown = cursorStyle.Render(label) + strings.Repeat(" ", max(labelWidth-ansi.StringWidth(label), 0))
	}
	if typed {
		value = m.edit.input.View()
	}
	line := gutter + shown + value
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

// layout is the page: the project's card, then a card per window, then
// the row adding one.
func (m configForm) layout() section {
	shown := m
	shown.project = m.shown()
	project := shown.project
	width := max(m.width, 30)
	page := newSection()
	page.boxed("project", shown.projectCard(), width, m.focus.window < 0)
	windows := project.Windows()
	for i, window := range windows {
		heavy := m.focus.window == i && (m.focus.pane < 0 || m.focus.pane >= len(window.Panes()))
		page.boxed("window "+window.Name().String(), shown.windowCard(i, width-4), width, heavy)
	}
	shown.put(&page, "+ window", "", "", spot{window: len(windows), pane: -1, field: fieldName})
	return page
}

// projectCard is the project's name, root, before commands and env.
func (m configForm) projectCard() section {
	p := m.project
	s := newSection()
	shownText(&s, "name", keysStyle.Render(p.Name().String()))
	m.put(&s, "root", contractHome(p.Root().String()), m.mark(p.Root().String()), spot{window: -1, pane: -1, field: fieldRoot})
	m.list(&s, "before", lines(p.Before()), spot{window: -1, pane: -1, field: fieldBefore})
	label := "env"
	vars := p.Env().Vars()
	for k, v := range vars {
		m.put(&s, label, varText(v), "", spot{window: -1, pane: -1, field: fieldEnv, item: k})
		label = ""
	}
	m.put(&s, label, keysStyle.Render("+ add"), "", spot{window: -1, pane: -1, field: fieldEnv, item: len(vars)})
	return s
}

// windowCard is the window at index's name, dir and before commands,
// its grid, then its panes, the one with the cursor open, then the row
// adding one, all within width cells.
func (m configForm) windowCard(index, width int) section {
	root := m.project.Root()
	window := m.project.Windows()[index]
	at := func(f field) spot { return spot{window: index, pane: -1, field: f} }
	s := newSection()
	m.put(&s, "name", window.Name().String(), "", at(fieldName))
	m.put(&s, "dir", shownDir(root, window.Dir()), m.mark(root.Resolve(window.Dir())), at(fieldDir))
	m.list(&s, "before", lines(window.Before()), at(fieldBefore))
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
	for j, pane := range window.Panes() {
		if m.focus.window == index && m.focus.pane == j {
			s.boxed(paneHeading(pane), m.paneCard(index, j), width, true)
			continue
		}
		folded := newSection()
		folded.row(keysStyle.Render("▸ ")+foldedPane(root, window, pane), spot{window: index, pane: j, field: fieldFolded}, false)
		s.boxed(paneHeading(pane), folded, width, false)
	}
	if !gridded {
		m.put(&s, "+ pane", "", "", spot{window: index, pane: len(window.Panes()), field: fieldName})
	}
	return s
}

// paneCard is a pane's fields, its project's and window's before
// commands and its project's variables dimmed among its own, with the
// level they come from.
func (m configForm) paneCard(index, j int) section {
	p := m.project
	root := p.Root()
	window := p.Windows()[index]
	pane := window.Panes()[j]
	_, gridded := window.Grid()
	at := func(f field) spot { return spot{window: index, pane: j, field: f} }
	s := newSection()

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

	if split, ok := pane.Split(); ok && !gridded {
		m.put(&s, "split", "of "+paneHeading(window.Panes()[split.Of()]), "", at(fieldSplitOf))
		m.put(&s, "direction", choices([]string{"→ right", "↓ below"}, int(split.Direction())-1), "", at(fieldDirection))
		m.put(&s, "size", sizeBar(split.Size()), "", at(fieldSize))
	}
	m.put(&s, "starts", choices(startModes, startMode(pane)), "", at(fieldStarts))
	if cmd, ok := pane.Startup().Command(); ok || (m.editing && m.edit.at == at(fieldCommand)) {
		m.put(&s, "command", cmd.String(), "", at(fieldCommand))
	}

	label := "before"
	for _, line := range higherBefore(p, window) {
		shownText(&s, label, keysStyle.Render(line.value+"  "+line.level))
		label = ""
	}
	m.list(&s, label, lines(pane.Before()), at(fieldBefore))

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

// foldedPane is a pane on one line: the dir it starts in, what it
// starts, and how many before commands, variables and note lines it has.
func foldedPane(root domain.ProjectRoot, window domain.Window, pane domain.Pane) string {
	dir, own := pane.Dir()
	if !own {
		dir = window.Dir()
	}
	parts := []string{shownDir(root, dir)}
	switch cmd, ok := pane.Startup().Command(); {
	case !ok:
		parts = append(parts, "shell")
	case pane.Startup().Submits():
		parts = append(parts, "$ "+cmd.String())
	default:
		parts = append(parts, "prefill "+cmd.String())
	}
	for _, count := range []struct {
		n    int
		what string
	}{{len(pane.Before()), "before"}, {len(pane.Env().Vars()), "env"}, {len(pane.Note()), "note"}} {
		if count.n > 0 {
			parts = append(parts, strconv.Itoa(count.n)+" "+count.what)
		}
	}
	return strings.Join(parts, " · ")
}

// choices are options side by side, the selected one highlighted.
func choices(options []string, selected int) string {
	shown := make([]string, len(options))
	for i, option := range options {
		if i == selected {
			shown[i] = cursorStyle.Render(" " + option + " ")
		} else {
			shown[i] = keysStyle.Render(" " + option + " ")
		}
	}
	return strings.Join(shown, " ")
}

// barWidth is the cells of a size's bar.
const barWidth = 14

// sizeBar draws a percentage as a bar, and other sizes as words.
func sizeBar(size domain.Size) string {
	digits, percent := strings.CutSuffix(size.String(), "%")
	switch {
	case size.IsDefault():
		return keysStyle.Render("◂ " + strings.Repeat("█", barWidth/2) + strings.Repeat("░", barWidth-barWidth/2) + " half ▸")
	case !percent:
		return size.String() + " cells"
	}
	n, _ := strconv.Atoi(digits)
	filled := n * barWidth / 100
	return "◂ " + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + " " + size.String() + " ▸"
}

// scrolled is the first line shown, keeping the cursor and the line
// below it, where an error shows, in view.
func (m configForm) scrolled() int {
	page := m.layout()
	height := m.bodyHeight()
	cursor := max(page.cursor, 0)
	top := min(m.top, cursor, max(len(page.lines)-height, 0))
	return max(top, cursor+2-height, 0)
}

// bodyHeight is the lines left for the cards, below the trail and above
// the status and the keys.
func (m configForm) bodyHeight() int {
	return max(m.height-3, 1)
}

func (m configForm) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

// render is the trail, the cards in view, the status line and the keys.
func (m configForm) render() string {
	if m.width == 0 {
		return "" // until the terminal's size is known
	}
	page := m.layout()
	height := m.bodyHeight()
	shown := page.lines[min(m.top, len(page.lines)):min(m.top+height, len(page.lines))]
	body := strings.Join(shown, "\n") + strings.Repeat("\n", height-len(shown))
	return m.trail() + "\n" + body + "\n" + ansi.Truncate(m.status(), m.width, "…") + "\n" + m.keys()
}

// trail is where the cursor is, and whether there are changes to save.
func (m configForm) trail() string {
	project := m.shown()
	parts := []string{project.Name().String()}
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

// status is the question asked, or why the last change was refused.
func (m configForm) status() string {
	switch {
	case m.asking != nil:
		return " " + m.asking.text + keysStyle.Render("  y yes · n no")
	case m.problem != "":
		return " " + problemStyle.Render("✗ "+m.problem)
	}
	return ""
}

// keys are what the keys do, while typing or on a row.
func (m configForm) keys() string {
	told := "↑↓ move · enter edit · ←→ change · o add · d delete · J/K reorder · ctrl+s save · esc leave"
	if m.editing {
		told = "enter keep · esc cancel · ctrl+s save"
	}
	return ansi.Truncate(keysStyle.Render(" "+told), m.width, "…")
}
