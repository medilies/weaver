package infra

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// TeaBrowser shows the registered projects full screen: their names and
// roots on the left, narrowed by a filter, the selected one's summary and
// a drawing of its windows in the middle, or the preview of the selected
// window of its owned session, its tasks on the right, and what the last
// action did below. It only shows and picks: the use case acts. Shown
// again on the same project, it keeps the window, pane and task selected,
// and the cursor in the task list; shown again at all, the drawing when it
// was switched to.
type TeaBrowser struct {
	kept *browserPlace
	look look
}

// browserPlace is what the browser had selected when it last ended.
type browserPlace struct {
	project            domain.ProjectName
	window, pane, task int
	tasking            bool
	drawings           bool // the drawing switched to in place of the preview
}

// NewTeaBrowser constructs a TeaBrowser showing what is selected in the
// settings' selected colour.
func NewTeaBrowser(settings Settings) TeaBrowser {
	return TeaBrowser{kept: &browserPlace{}, look: settings.look()}
}

// Browse implements app.ProjectBrowser.
func (b TeaBrowser) Browse(projects []app.ProjectOverview, selected domain.ProjectName, notice string, live app.LiveSessions) (app.Choice, error) {
	model := browser{projects: projects, notice: notice, look: b.look, live: live, drawings: b.kept.drawings}
	for i, project := range projects {
		if project.Name() == selected {
			model.cursor = i
		}
	}
	if kept := *b.kept; kept.project == selected {
		model.window, model.pane, model.task = kept.window, kept.pane, kept.task
		if kept.tasking && len(model.tasksShown()) > 0 {
			// The config may have changed: the task decides what is drawn.
			model.tasking, model = true, model.taskSelected(kept.task)
		}
	}
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return app.Choice{}, err
	}
	last := final.(browser)
	b.kept.drawings = last.drawings
	if shown := last.shown(); len(shown) > 0 {
		*b.kept = browserPlace{project: shown[last.cursor].Name(), window: last.window, pane: last.pane,
			task: last.task, tasking: last.tasking, drawings: last.drawings}
	}
	choice := last.choice
	if choice.Action == 0 {
		return app.Choice{}, app.ErrAborted
	}
	return choice, nil
}

// browserKeys are the keys making a choice.
var browserKeys = map[string]app.Action{
	"enter": app.ActionOpen, "s": app.ActionStart, "b": app.ActionBind, "e": app.ActionEdit, "E": app.ActionEditRaw,
	"x": app.ActionKill, "d": app.ActionDelete,
}

var (
	runningMark  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	rootStyle    = lipgloss.NewStyle().Faint(true)
	titleStyle   = lipgloss.NewStyle().Bold(true)
	keysStyle    = lipgloss.NewStyle().Faint(true)
	problemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

// browser is TeaBrowser's Bubble Tea model. The cursor is in the
// projects the filter shows, or in the task list when tasking, top is the
// first project the list shows, the window is the selected project's one
// drawn, the pane is its selected one, and the task the selected one in
// the task list.
type browser struct {
	look       look
	projects   []app.ProjectOverview
	cursor     int
	top        int
	window     int
	pane       int
	task       int
	tasking    bool
	filter     string
	filtering  bool // typing the filter
	config     bool // the selected project's config shown instead of its drawing
	drawings   bool // the drawing shown in place of the preview
	live       app.LiveSessions
	preview    previewed // the last preview captured, if hasPreview
	hasPreview bool
	// previewPane is the selected pane of the preview, by its index in
	// it; pane follows it when the config has it.
	previewPane     int
	previewFailure  string       // why the last preview failed, told once
	previewAsked    int          // the asks for a preview
	previewAnswered int          // the last ask answered
	running         tasksRunning // the tasks last found running, if hasRunning
	hasRunning      bool
	tasksAsked      int // the asks for the running tasks
	tasksAnswered   int // the last ask answered
	notice          string
	width, height   int
	choice          app.Choice
}

// shown are the projects whose names contain the filter, ignoring case.
func (m browser) shown() []app.ProjectOverview {
	if m.filter == "" {
		return m.projects
	}
	var shown []app.ProjectOverview
	for _, project := range m.projects {
		if strings.Contains(strings.ToLower(project.Name().String()), strings.ToLower(m.filter)) {
			shown = append(shown, project)
		}
	}
	return shown
}

// filtered is m with filter, keeping the selected project when it is
// still shown, else selecting the first.
func (m browser) filtered(filter string) browser {
	shown := m.shown()
	if len(shown) == 0 {
		m.filter = filter
		return m
	}
	selected := shown[m.cursor].Name()
	m.filter, m.cursor = filter, 0
	for i, project := range m.shown() {
		if project.Name() == selected {
			m.cursor = i
		}
	}
	return m.reselected(selected)
}

// reselected is m drawing the first window again, its first pane and
// first task selected and the cursor out of the task list, unless the
// project selected before is still.
func (m browser) reselected(before domain.ProjectName) browser {
	if shown := m.shown(); len(shown) == 0 || shown[m.cursor].Name() != before {
		m.window, m.pane, m.task, m.tasking = 0, 0, 0, false
	}
	return m
}

// windowMoved is m drawing the window by windows right of the one it
// draws, or left when by is negative, staying in the selected project's.
func (m browser) windowMoved(by int) browser {
	return m.windowSelected(max(min(m.window+by, len(m.windowsShown())-1), 0))
}

// windowSelected is m drawing the window at index, its first pane
// selected unless it was drawn already.
func (m browser) windowSelected(index int) browser {
	if index != m.window {
		m.window, m.pane = index, 0
	}
	return m
}

// paneMoved is m with the pane by panes after the selected one selected,
// or before when by is negative, going round the drawn window's.
func (m browser) paneMoved(by int) browser {
	windows := m.windowsShown()
	if len(windows) == 0 {
		return m
	}
	if preview, ok := m.shownPreview(); ok {
		return m.previewPaneSelected(steppedIn(previewOrder(preview), m.previewPane, by))
	}
	window := windows[m.drawn(windows)]
	order := drawnOrder(window, m.width, max(m.height-1, 1))
	m.pane = steppedIn(order, m.selectedPane(len(window.Panes())), by)
	return m
}

// drawn is the index of the window m draws of windows, the selected
// project's.
func (m browser) drawn(windows []domain.WindowPlan) int {
	return min(m.window, len(windows)-1)
}

// selectedPane is the selected pane's index in a window of count panes.
func (m browser) selectedPane(count int) int {
	return min(m.pane, count-1)
}

// windowsShown are the selected project's windows, as it opens them.
func (m browser) windowsShown() []domain.WindowPlan {
	shown := m.shown()
	if len(shown) == 0 {
		return nil
	}
	return shown[m.cursor].Project().Plan().Windows()
}

// Init starts the ticks asking for the preview and the running tasks
// again; the first asks follow the terminal's size.
func (m browser) Init() tea.Cmd {
	return tick()
}

// Update is m after msg, its list scrolled to keep the cursor in view,
// and the preview captured at once when another one is wanted.
func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	next := model.(browser)
	_, _, _, height, _ := next.frame()
	next.top = next.firstShown(height)
	before, beforeWindow, wanted := m.previewKey()
	after, afterWindow, wants := next.previewKey()
	if wants != wanted || wants && (before.Name() != after.Name() || beforeWindow != afterWindow) {
		next.hasPreview = false // kept, it would bring back what was selected in it
		var fetch tea.Cmd
		next, fetch = next.fetch()
		cmd = tea.Batch(cmd, fetch)
	}
	if next.tasksAsked == 0 || next.selectedName() != m.selectedName() {
		var fetch tea.Cmd
		next, fetch = next.fetchTasks()
		cmd = tea.Batch(cmd, fetch)
	}
	return next, cmd
}

// selectedName is the selected project's name, if one is shown.
func (m browser) selectedName() domain.ProjectName {
	if shown := m.shown(); len(shown) > 0 {
		return shown[m.cursor].Name()
	}
	return domain.ProjectName{}
}

func (m browser) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case previewTick:
		// An ask still unanswered isn't asked again, so slow answers can't
		// pile up.
		var preview, tasks tea.Cmd
		if m.previewAsked == m.previewAnswered {
			m, preview = m.fetch()
		}
		if m.tasksAsked == m.tasksAnswered {
			m, tasks = m.fetchTasks()
		}
		return m, tea.Batch(preview, tasks, tick())
	case tasksRunning:
		m.tasksAnswered = max(m.tasksAnswered, msg.asked)
		if msg.asked == m.tasksAsked {
			m.running, m.hasRunning = msg, true
		}
		return m, nil
	case previewed:
		return m.withPreview(msg), nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return m.clicked(msg.X, msg.Y)
		}
	case tea.KeyPressMsg:
		if m.filtering && msg.String() != "ctrl+c" {
			return m.typeFilter(msg), nil
		}
		return m.press(msg.String())
	}
	return m, nil
}

// press moves the cursor, starts the filter or clears it, or ends with
// the choice the key makes.
func (m browser) press(key string) (tea.Model, tea.Cmd) {
	if m.tasking && key != "q" && key != "ctrl+c" {
		next, chosen := m.pressInTasks(key)
		if chosen {
			return next, tea.Quit
		}
		return next, nil
	}
	switch key {
	case "up", "k":
		m = m.moved(-1)
	case "down", "j":
		m = m.moved(1)
	case "left":
		m = m.windowMoved(-1)
	case "right":
		m = m.windowMoved(1)
	case "tab":
		m = m.paneMoved(1)
	case "shift+tab":
		m = m.paneMoved(-1)
	case "/":
		m.filtering = true
	case "y":
		m.config = !m.config
	case "p":
		return m.switchedView(), nil
	case "r":
		windows := m.windowsShown()
		if !m.selectedExited() {
			break // only an exited command reruns
		}
		window := windows[m.drawn(windows)]
		pane, _ := m.selectedInConfig(len(window.Panes()))
		m.choice = app.Choice{Action: app.ActionRerun, Project: m.shown()[m.cursor].Name(),
			Pane: app.PaneAt{Window: window.Name(), Pane: pane}}
		return m, tea.Quit
	case "t":
		if len(m.tasksShown()) > 0 {
			m.tasking = true
			m = m.taskSelected(m.task)
		}
	case "esc":
		switch {
		case m.config:
			m.config = false
		case m.filter == "":
			return m, tea.Quit
		default:
			m = m.filtered("")
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	default:
		shown := m.shown()
		action, ok := browserKeys[key]
		if !ok || (len(shown) == 0 && action != app.ActionBind) {
			break
		}
		m.choice = app.Choice{Action: action}
		if len(shown) > 0 {
			m.choice.Project = shown[m.cursor].Name()
		}
		return m, tea.Quit
	}
	return m, nil
}

// typeFilter edits the filter as it is typed.
func (m browser) typeFilter(msg tea.KeyPressMsg) browser {
	if next, ok := m.filterKey(msg.String()); ok {
		return next
	}
	if msg.Text == "" {
		return m
	}
	return m.filtered(m.filter + msg.Text)
}

// filterKey is m after key, pressed while the filter is typed: the
// arrows and tab select, enter keeps the filter for the keys to act on
// what it shows, esc clears it. False when key types text instead.
func (m browser) filterKey(key string) (browser, bool) {
	switch key {
	case "up":
		return m.moved(-1), true
	case "down":
		return m.moved(1), true
	case "left":
		return m.windowMoved(-1), true
	case "right":
		return m.windowMoved(1), true
	case "tab":
		return m.paneMoved(1), true
	case "shift+tab":
		return m.paneMoved(-1), true
	case "enter":
		m.filtering = false
		return m, true
	case "esc":
		m.filtering = false
		return m.filtered(""), true
	case "backspace":
		runes := []rune(m.filter)
		return m.filtered(string(runes[:max(len(runes)-1, 0)])), true
	}
	return m, false
}

// moved is m with the cursor by up to by projects down, or up when by is
// negative, staying in the projects shown.
func (m browser) moved(by int) browser {
	shown := m.shown()
	if len(shown) == 0 {
		return m
	}
	before := shown[m.cursor].Name()
	m.cursor = max(min(m.cursor+by, len(shown)-1), 0)
	return m.reselected(before)
}

func (m browser) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// clicked is m after a click at (x, y): on a project, a window in the
// window list or a pane in the pane list, selecting it as the arrow keys
// and tab do, on a task, running it, or on a key told, doing what the key
// does.
func (m browser) clicked(x, y int) (tea.Model, tea.Cmd) {
	shown := m.shown()
	notice, list, taskColumn, height, width := m.frame()
	listWidth, tasksWidth := lipgloss.Width(list), lipgloss.Width(taskColumn)
	if y == height+2+lipgloss.Height(notice) { // below the cards
		_, spots := m.keys()
		if spot, ok := hotspotAt(spots, x); ok {
			return m.act(spot)
		}
		return m, nil
	}
	y-- // below the cards' top borders
	if len(shown) == 0 || y < 0 || y >= height {
		return m, nil
	}
	if x >= listWidth && x < listWidth+tasksWidth { // in the task list's column
		_, tasks := m.taskList(taskListWidth(m.projects, m.width), height)
		if y < len(tasks) && tasks[y] >= 0 {
			if next, chosen := m.runningTask(tasks[y]); chosen {
				return next, tea.Quit
			}
		}
		return m, nil
	}
	if x < listWidth { // in the project list's card
		if i := m.firstShown(height) + y; i < len(shown) {
			return m.moved(i - m.cursor), nil
		}
		return m, nil
	}
	project := shown[m.cursor]
	windows := project.Project().Plan().Windows()
	if len(windows) == 0 || m.config {
		return m, nil
	}
	m.tasking = false // a window or pane clicked is no longer the task's
	laid := m.laidOut(project, windows, width, height)
	x -= listWidth + tasksWidth + cardSides/2
	var spots []hotspot
	switch row := y - windowListRow(laid.top); {
	case row == 0:
		_, spots = windowList(m.look, project, windows, laid.index, width, laid.miniWidth())
	case row > 0 && row <= len(laid.miniatures):
		// A miniature is clicked as the name above it.
		_, spots = windowList(m.look, project, windows, laid.index, width, laid.miniWidth())
		spots = slices.DeleteFunc(spots, func(s hotspot) bool { return s.key != "" })
	case row == 1+len(laid.miniatures)+len(laid.panes):
		if laid.paneList == "" {
			return m, nil
		}
		if spot, ok := hotspotAt(laid.paneSpots, x); ok {
			if _, previewing := m.shownPreview(); previewing {
				m = m.previewPaneSelected(spot.item)
			} else {
				m.pane = spot.item
			}
		}
		return m, nil
	}
	if spot, ok := hotspotAt(spots, x); ok {
		return m.act(spot)
	}
	return m, nil
}

// act is m after spot is clicked: what its key does, or its window
// selected.
func (m browser) act(spot hotspot) (tea.Model, tea.Cmd) {
	if spot.key == "" {
		return m.windowSelected(spot.item), nil
	}
	if m.filtering {
		if next, ok := m.filterKey(spot.key); ok {
			return next, nil
		}
	}
	return m.press(spot.key)
}

// render lays out the list beside the details, above the notice and the
// keys.
func (m browser) render() string {
	if m.width == 0 {
		return "" // until the terminal's size is known
	}
	notice, list, tasks, height, width := m.frame()
	keysLine, _ := m.keys()
	details := column(m.detailsTitle(), m.details(width, height), width+cardSides, height, m.look.edge(false))
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, list, tasks, details), notice, keysLine)
}

// frame is the notice, the project list's card and the task list's
// beside it, as render lays them out, the lines they leave inside the
// cards above the notice and the keys, and the width the details' card
// has for its content right of the task list's.
func (m browser) frame() (notice, list, tasks string, height, width int) {
	notice = lipgloss.NewStyle().Width(m.width).Render(strings.Join(strings.Fields(m.notice), " "))
	height = max(m.height-lipgloss.Height(notice)-1-2, 1)
	if len(m.projects) > 0 {
		projects := m.list(height)
		list = column("projects", projects, lipgloss.Width(projects)+cardSides, height, m.look.edge(!m.tasking))
	}
	tasks, _ = m.taskColumn(taskListWidth(m.projects, m.width), height)
	return notice, list, tasks, height, max(m.width-lipgloss.Width(list)-lipgloss.Width(tasks)-cardSides, 1)
}

// cardSides is the cells across a card's borders and the spaces inside
// them take.
const cardSides = 4

// column is content in a card titled title, width cells wide, its lines
// cut or filled to height, its border drawn in edge.
func column(title, content string, width, height int, edge lipgloss.Style) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), height)]
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(card(title, lines, width, edge), "\n")
}

// keyTold is a key the keys line tells, and what it does.
type keyTold struct {
	key, does string
}

// toldKeys is keys told side by side, faint, with the cells each covers.
func toldKeys(told []keyTold) (string, []hotspot) {
	var line string
	var spots []hotspot
	for i, k := range told {
		if i > 0 {
			line += keysStyle.Render(" · ")
		}
		text := k.key + " " + k.does
		from := ansi.StringWidth(line)
		spots = append(spots, hotspot{from: from, to: from + ansi.StringWidth(text), key: k.key})
		line += keysStyle.Render(text)
	}
	return line, spots
}

var (
	typingKeys   = []keyTold{{"enter", "keep"}, {"esc", "clear"}}
	filteredKeys = []keyTold{{"enter", "open"}, {"s", "start"}, {"b", "bind"}, {"e", "edit"}, {"E", "raw"}, {"x", "kill"}, {"d", "delete"}, {"esc", "clear"}, {"q", "quit"}}
	browsingKeys = []keyTold{{"enter", "open"}, {"s", "start"}, {"b", "bind"}, {"e", "edit"}, {"E", "raw"}, {"x", "kill"}, {"d", "delete"}, {"/", "filter"}, {"q", "quit"}}
)

// paneKey is tab told, before the last two keys, when the drawn window
// has panes to switch between, rerunKey r when the selected pane's
// command exited, taskKey t, when the selected project has tasks, and
// configKey y, or drawingKey while the config is shown, when there is a
// project.
var (
	paneKey    = keyTold{"tab", "pane"}
	rerunKey   = keyTold{"r", "rerun"}
	taskKey    = keyTold{"t", "tasks"}
	configKey  = keyTold{"y", "config"}
	drawingKey = keyTold{"y", "drawing"}
	// previewKey p, or previewDrawingKey while the preview is shown, when
	// the selected project's session can be previewed.
	previewKey        = keyTold{"p", "preview"}
	previewDrawingKey = keyTold{"p", "drawing"}
)

// taskingKeys are told while the cursor is in the task list.
var taskingKeys = []keyTold{{"enter", "run, go"}, {"r", "run"}, {"esc", "back"}, {"q", "quit"}}

// keys are the filter being typed and what its keys do, or the filter
// kept and the keys acting on what it shows, cut to the width; with the
// cells each key told covers, and the filter kept, which / edits again,
// to click them. ←→ is told beside the window list.
func (m browser) keys() (string, []hotspot) {
	var line string
	var spots []hotspot
	told := browsingKeys
	switch {
	case m.filtering:
		line, told = "/"+m.filter+keysStyle.Render("▏ "), typingKeys
	case m.filter != "":
		spots = append(spots, hotspot{from: 0, to: ansi.StringWidth("/" + m.filter), key: "/"})
		line, told = "/"+m.filter+keysStyle.Render(" · "), filteredKeys
	}
	preview, previewing := m.shownPreview()
	if windows := m.windowsShown(); !m.filtering && len(windows) > 0 &&
		(previewing && len(preview.Panes()) > 1 || !previewing && len(windows[m.drawn(windows)].Panes()) > 1) {
		told = slices.Insert(slices.Clone(told), len(told)-2, paneKey)
	}
	if !m.filtering && m.selectedExited() {
		told = slices.Insert(slices.Clone(told), len(told)-2, rerunKey)
	}
	if !m.filtering && len(m.tasksShown()) > 0 {
		told = slices.Insert(slices.Clone(told), len(told)-2, taskKey)
	}
	if !m.filtering && len(m.shown()) > 0 && m.shown()[m.cursor].Previewable() && !m.config {
		key := previewKey
		if previewing {
			key = previewDrawingKey
		}
		told = slices.Insert(slices.Clone(told), len(told)-2, key)
	}
	if !m.filtering && len(m.shown()) > 0 {
		key := configKey
		if m.config {
			key = drawingKey
		}
		told = slices.Insert(slices.Clone(told), len(told)-2, key)
	}
	if m.tasking {
		line, spots, told = "", nil, taskingKeys
	}
	keys, keySpots := toldKeys(told)
	offset := ansi.StringWidth(line)
	for _, spot := range keySpots {
		spots = append(spots, hotspot{from: offset + spot.from, to: offset + spot.to, key: spot.key})
	}
	return cut(line+keys, spots, m.width)
}

// list is the shown projects' names and dimmed roots, the selected one
// highlighted and kept in view, and a mark for those whose session runs.
// It takes at most half the width, cutting roots short from the left,
// and keeps its width when the filter shows no project.
func (m browser) list(height int) string {
	shown := m.shown()
	nameWidth, rootWidth := 0, 0 // of all projects, so filtering keeps the column still
	for _, project := range m.projects {
		nameWidth = max(nameWidth, len(project.Name().String()))
		rootWidth = max(rootWidth, ansi.StringWidth(root(project)))
	}
	// "● name root " and the card fit in half the width.
	rootWidth = min(rootWidth, max(m.width/2-nameWidth-4-cardSides, 0))
	first := m.firstShown(height)
	lines := make([]string, 0, len(shown)-first)
	for i, project := range shown[first:] {
		mark := " "
		if project.Running() {
			mark = runningMark
		}
		name := fmt.Sprintf("%s %-*s ", mark, nameWidth, project.Name())
		path := lipgloss.NewStyle().Width(rootWidth + 1).Render(shortened(root(project), rootWidth))
		if first+i == m.cursor {
			lines = append(lines, cursorStyle.Render(name)+cursorStyle.Faint(true).Render(path))
		} else {
			lines = append(lines, name+rootStyle.Render(path))
		}
	}
	if len(m.projects) == 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(nameWidth + rootWidth + 4).Render(strings.Join(lines, "\n"))
}

// firstShown is the first project the list shows in height lines: top,
// unless the cursor left the view, which scrolls the list as little as
// brings it back, or the list would end before the last line.
func (m browser) firstShown(height int) int {
	return max(min(m.top, m.cursor, len(m.shown())-height), m.cursor-height+1, 0)
}

// shortened is path cut from the left to fit width cells, "…" taking the
// place of what is cut.
func shortened(path string, width int) string {
	over := ansi.StringWidth(path) - width
	switch {
	case over <= 0:
		return path
	case width == 0:
		return ""
	}
	return ansi.TruncateLeft(path, over+1, "…")
}

// root is project's root with the home directory as "~", or nothing when
// its config can't be read.
func root(project app.ProjectOverview) string {
	return contractHome(project.Project().Root().String())
}

// Below this size, the details list the selected window's panes
// instead of drawing them.
const minDrawingWidth, minDrawingHeight = 24, 6

// details are the selected project's summary, or why its config can't
// be read, then its window list, the selected one drawn in what is left
// of width×height above its pane list, and the levels of its selected
// pane below.
func (m browser) details(width, height int) string {
	shown := m.shown()
	switch {
	case len(m.projects) == 0:
		return "No projects yet: press b to bind the current directory."
	case len(shown) == 0:
		return fmt.Sprintf("No project's name contains %q.", m.filter)
	}
	project := shown[m.cursor]
	if m.config {
		return detailsTop(project, width) + "\n\n" + configText(project.Config().Content, width)
	}
	windows := project.Project().Plan().Windows()
	if len(windows) == 0 {
		return detailsTop(project, width)
	}
	laid := m.laidOut(project, windows, width, height)
	panes := laid.panes
	if laid.paneList != "" {
		panes = append(panes, laid.paneList)
	}
	listLine, _ := windowList(m.look, project, windows, laid.index, width, laid.miniWidth())
	below := append(slices.Clone(laid.miniatures), panes...)
	return laid.top + "\n\n" + listLine + "\n" + strings.Join(append(below, laid.levels...), "\n")
}

// configText is a config's YAML as stored, each line cut to width, tabs
// and other control characters shown as spaces so they can't break the
// lines.
func configText(content []byte, width int) string {
	var shown []string
	for line := range strings.Lines(string(content)) {
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, strings.TrimRight(line, "\r\n"))
		shown = append(shown, ansi.Truncate(line, width, "…"))
	}
	return strings.Join(shown, "\n")
}

// detailsLayout is the parts of the details below the window list.
type detailsLayout struct {
	top        string   // the config file and the summary
	miniatures []string // small drawings of every window under the window list, when they fit
	index      int      // the drawn window's
	panes      []string
	paneList   string // under a drawing, empty when the panes are listed
	paneSpots  []hotspot
	levels     []string
}

// laidOut is how the details of project, whose windows are windows, fit
// in width×height: its top, then below the window list, the drawn
// window's panes drawn or listed, its pane list and the levels of the
// selected pane in what is left.
func (m browser) laidOut(project app.ProjectOverview, windows []domain.WindowPlan, width, height int) detailsLayout {
	top := detailsTop(project, width)
	index := m.drawn(windows)
	below := height - windowListRow(top) - 1 // the window list
	levelLines := []string{keysStyle.Render("not in the config")}
	if pane, inConfig := m.selectedInConfig(len(windows[index].Panes())); inConfig {
		levelLines = levels(project.Project(), index, pane, width)
	}
	var miniatures []string
	if below-miniatureHeight-1 >= minDrawingHeight+1+min(len(levelLines), 3) {
		miniatures = m.miniatures(project, windows, index, width)
		below -= len(miniatures)
	}
	panes, paneList, spots := m.panesShown(project, index, width, below, len(levelLines))
	shown := len(panes)
	if paneList != "" {
		shown++
	}
	return detailsLayout{top: top, miniatures: miniatures, index: index, panes: panes, paneList: paneList, paneSpots: spots,
		levels: fitted(levelLines, below-shown)}
}

// miniatureWidth and miniatureHeight are the size of a window's
// miniature, the window list's names padded to its width above it.
const miniatureWidth, miniatureHeight = 14, 4

// miniWidth is the width the window list pads each name to: a
// miniature's when they are drawn under it, else none.
func (l detailsLayout) miniWidth() int {
	if len(l.miniatures) == 0 {
		return 0
	}
	return miniatureWidth
}

// miniatures are project's windows drawn small, side by side, each under
// its name in the window list, the selected one in the selected colour:
// none for a single window, which its drawing shows, nor when they don't
// all fit in width.
func (m browser) miniatures(project app.ProjectOverview, windows []domain.WindowPlan, selected, width int) []string {
	if len(windows) < 2 {
		return nil
	}
	_, spots := windowList(m.look, project, windows, selected, width, miniatureWidth)
	var placed []hotspot
	for _, spot := range spots {
		if spot.key == "" {
			placed = append(placed, spot)
		}
	}
	if len(placed) != len(windows) || placed[0].item != 0 || placed[len(placed)-1].to > width {
		return nil // scrolled or cut: the names alone
	}
	root := project.Project().Root().String()
	rows := make([]string, miniatureHeight)
	for _, spot := range placed {
		window := windows[spot.item]
		drawn := drawing(m.look, window, root, alerts(project, window), -1, false, m.width, max(m.height-1, 1),
			miniatureWidth, miniatureHeight)
		for y := range rows {
			line := drawn[y]
			if spot.item == selected {
				line = m.look.picked.Render(ansi.Strip(line))
			}
			rows[y] += strings.Repeat(" ", max(spot.from-ansi.StringWidth(rows[y]), 0)) + line
		}
	}
	return rows
}

// panesShown are the panes of the window at index of project's, drawn
// in width×below less the levels' height, the drawing keeping its least
// height, then the pane list under the drawing, with the cells each pane
// covers, to click it; with no room to draw them, they are listed and
// there is no pane list.
func (m browser) panesShown(project app.ProjectOverview, index, width, below, levelHeight int) ([]string, string, []hotspot) {
	window := project.Project().Plan().Windows()[index]
	root := project.Project().Root().String()
	pane := m.selectedPane(len(window.Panes()))
	if width < minDrawingWidth || below-1 < minDrawingHeight {
		return undrawnPanes(window, root, alerts(project, window), pane, width, below), "", nil
	}
	if preview, ok := m.shownPreview(); ok {
		drawn := previewDrawing(m.look, preview, previewNames(preview, window), previewAlerts(preview, alerts(project, window)),
			m.previewPane, width, max(below-1-levelHeight, minDrawingHeight))
		line, spots := namesList(m.look, previewNames(preview, window), previewOrder(preview), m.previewPane, width)
		return drawn, line, spots
	}
	// A session opens at the browser's size, less tmux's status line.
	drawn := drawing(m.look, window, root, alerts(project, window), pane, false, m.width, max(m.height-1, 1),
		width, max(below-1-levelHeight, minDrawingHeight))
	line, spots := paneList(m.look, project.Project().Windows()[index], drawnOrder(window, m.width, max(m.height-1, 1)), pane, width)
	return drawn, line, spots
}

// windowListRow is the details' line the window list is on, below top and a blank
// line.
func windowListRow(top string) int {
	return lipgloss.Height(top) + 1
}

// selectedExited reports whether the selected pane's command exited, so
// that r can rerun it.
func (m browser) selectedExited() bool {
	windows := m.windowsShown()
	if len(windows) == 0 {
		return false
	}
	window := windows[m.drawn(windows)]
	pane, inConfig := m.selectedInConfig(len(window.Panes()))
	if !inConfig {
		return false
	}
	for _, warning := range m.shown()[m.cursor].Warnings() {
		if errors.Is(warning, app.ErrCommandExited) && warning.Concerns(window.Name(), pane) {
			return true
		}
	}
	return false
}

// alerts are, by pane, the worst of project's warnings about window's
// panes: a directory missing before a program not found, and both before
// a command that exited.
func alerts(project app.ProjectOverview, window domain.WindowPlan) []alert {
	panes := make([]alert, len(window.Panes()))
	for _, warning := range project.Warnings() {
		a := programAlert
		switch {
		case errors.Is(warning, app.ErrMissingDirectory):
			a = dirAlert
		case errors.Is(warning, app.ErrCommandExited):
			a = exitedAlert
		}
		for i := range panes {
			if warning.Concerns(window.Name(), i) {
				panes[i] = max(panes[i], a)
			}
		}
	}
	return panes
}

// detailsTitle titles the details' card: the selected project's config
// file, or its name; nothing when no project is shown.
func (m browser) detailsTitle() string {
	shown := m.shown()
	if len(shown) == 0 {
		return ""
	}
	title := shown[m.cursor].Name().String()
	if file := shown[m.cursor].Config().File; file != "" {
		title = file
	}
	if _, ok := m.shownPreview(); ok {
		title += " · preview"
	}
	return title
}

// detailsTop is project's summary, or why its config can't be read,
// wrapped to width.
func detailsTop(project app.ProjectOverview, width int) string {
	header := summary(project)
	if err := project.Problem(); err != nil {
		header = problemStyle.Render(err.Error())
	}
	return lipgloss.NewStyle().Width(width).Render(header)
}

// windowList is the names of windows, the selected one in a block of the
// selected colour, ○ marking those the project's owned session lacks,
// each padded to pad cells to sit above its miniature, and which one of
// how many is drawn, after ←→ when there are several to switch between;
// with the cells each window's name and each arrow covers, to click
// them. When they don't fit in width, the first ones make way for the
// selected one.
func windowList(look look, project app.ProjectOverview, windows []domain.WindowPlan, selected, width, pad int) (string, []hotspot) {
	missing := map[domain.WindowName]bool{}
	for _, name := range project.MissingWindows() {
		missing[name] = true
	}
	labels := make([]string, len(windows))
	for i, window := range windows {
		label := " " + window.Name().String() + " "
		if missing[window.Name()] {
			label = " ○" + label
		}
		if i == selected {
			label = look.selected.Render(label)
		}
		labels[i] = label + strings.Repeat(" ", max(pad-ansi.StringWidth(label), 0))
	}
	count := fmt.Sprintf("%d/%d", selected+1, len(windows))
	if len(windows) > 1 {
		count = "←→ " + count
	}
	count = keysStyle.Render(count)
	first := 0
	line := strings.Join(labels, "  ")
	for first < selected && ansi.StringWidth(line)+1+ansi.StringWidth(count) > width {
		first++
		line = "…" + strings.Join(labels[first:], "  ")
	}
	limit := max(width-1-ansi.StringWidth(count), 0)
	var spots []hotspot
	from := 0
	if first > 0 {
		from = 1 // the "…"
	}
	for i := first; i < len(labels); i++ {
		to := from + ansi.StringWidth(labels[i])
		spots = append(spots, hotspot{from: from, to: to, item: i})
		from = to + 2
	}
	line, spots = cut(line, spots, limit)
	gap := max(width-ansi.StringWidth(line)-ansi.StringWidth(count), 1)
	if len(windows) > 1 {
		arrows := ansi.StringWidth(line) + gap
		spots = append(spots, hotspot{from: arrows, to: arrows + 1, key: "left"}, hotspot{from: arrows + 1, to: arrows + 2, key: "right"})
	}
	return line + strings.Repeat(" ", gap) + count, spots
}

// summary is where the project stands: its running session against its
// config, its root, what would likely go wrong opening
// it, and what every pane starts with.
func summary(project app.ProjectOverview) string {
	var lines []string
	if session, ok := project.Session(); ok {
		lines = append(lines, sessionLine(project, session))
	}
	rootLine := root(project)
	if !project.RootExists() {
		rootLine += " " + problemStyle.Render("✗ missing")
	}
	lines = append(lines, rootLine)

	for _, warning := range project.Warnings() {
		lines = append(lines, warningStyle.Render("⚠ "+warning.Error()))
	}

	for _, list := range []struct {
		label string
		cmds  []domain.Command
	}{{"setup", project.Project().Setup()}, {"teardown", project.Project().Teardown()}, {"before", project.Project().Before()}} {
		texts := make([]string, 0, len(list.cmds))
		for _, cmd := range list.cmds {
			texts = append(texts, cmd.String())
		}
		if len(texts) > 0 {
			lines = append(lines, keysStyle.Render(list.label+" "+strings.Join(texts, " · ")))
		}
	}
	var env []string
	for _, v := range project.Project().Env().Vars() {
		env = append(env, v.Name()+"="+v.Value())
	}
	if len(env) > 0 {
		lines = append(lines, keysStyle.Render("env "+strings.Join(env, " · ")))
	}
	return strings.Join(lines, "\n")
}

// sessionLine is how the running session compares with the config, and
// so what enter does: "● running · 2/3 windows · "logs" missing, enter
// restores it".
func sessionLine(project app.ProjectOverview, session domain.RunningSession) string {
	line := runningMark + " running"
	if !session.Owned() {
		return line + keysStyle.Render(" · not started by weaver, enter attaches as it is")
	}
	present, configured := project.WindowCount()
	line += fmt.Sprintf(" · %d/%d windows", present, configured)
	missing := project.MissingWindows()
	if len(missing) == 0 {
		return line
	}
	names := make([]string, 0, len(missing))
	for _, name := range missing {
		names = append(names, fmt.Sprintf("%q", name.String()))
	}
	pronoun := "it"
	if len(missing) > 1 {
		pronoun = "them"
	}
	return line + " · " + warningStyle.Render(strings.Join(names, ", ")+" missing, enter restores "+pronoun)
}
