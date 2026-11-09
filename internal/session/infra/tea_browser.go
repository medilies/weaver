package infra

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// TeaBrowser shows the registered projects full screen: their names and
// roots on the left, narrowed by a filter, the selected one's summary and
// a drawing of its windows in the middle, its tasks on the right, and
// what the last action did below. It only shows and picks: the use case
// acts. Shown again on the same project, it keeps the window, pane and
// task selected, and the cursor in the task list.
type TeaBrowser struct {
	kept *browserPlace
}

// browserPlace is what the browser had selected when it last ended.
type browserPlace struct {
	project            domain.ProjectName
	window, pane, task int
	tasking            bool
}

// NewTeaBrowser constructs a TeaBrowser.
func NewTeaBrowser() TeaBrowser {
	return TeaBrowser{kept: &browserPlace{}}
}

// Browse implements app.ProjectBrowser.
func (b TeaBrowser) Browse(projects []app.ProjectOverview, selected domain.ProjectName, notice string) (app.Choice, error) {
	model := browser{projects: projects, notice: notice}
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
	if shown := last.shown(); len(shown) > 0 {
		*b.kept = browserPlace{project: shown[last.cursor].Name(), window: last.window, pane: last.pane,
			task: last.task, tasking: last.tasking}
	}
	choice := last.choice
	if choice.Action == 0 {
		return app.Choice{}, app.ErrAborted
	}
	return choice, nil
}

// browserKeys are the keys making a choice.
var browserKeys = map[string]app.Action{
	"enter": app.ActionOpen, "f": app.ActionFreeze, "e": app.ActionEdit, "E": app.ActionEditRaw,
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
	pickedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	// selectedStyle is the selected item of a list or choice: a cyan block.
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("6")).Foreground(lipgloss.Color("0"))
)

// browser is TeaBrowser's Bubble Tea model. The cursor is in the
// projects the filter shows, or in the task list when tasking, top is the
// first project the list shows, the window is the selected project's one
// drawn, the pane is its selected one, and the task the selected one in
// the task list.
type browser struct {
	projects      []app.ProjectOverview
	cursor        int
	top           int
	window        int
	pane          int
	task          int
	tasking       bool
	filter        string
	filtering     bool // typing the filter
	notice        string
	width, height int
	choice        app.Choice
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
	count := len(windows[m.drawn(windows)].Panes())
	m.pane = ((m.selectedPane(count)+by)%count + count) % count
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

func (m browser) Init() tea.Cmd {
	return nil
}

// Update is m after msg, its list scrolled to keep the cursor in view.
func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	next := model.(browser)
	_, _, _, height, _ := next.frame()
	next.top = next.firstShown(height)
	return next, cmd
}

func (m browser) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	case "t":
		if len(m.tasksShown()) > 0 {
			m.tasking = true
			m = m.taskSelected(m.task)
		}
	case "esc":
		if m.filter == "" {
			return m, tea.Quit
		}
		m = m.filtered("")
	case "q", "ctrl+c":
		return m, tea.Quit
	default:
		shown := m.shown()
		action, ok := browserKeys[key]
		if !ok || (len(shown) == 0 && action != app.ActionFreeze) {
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
	notice, list, _, height, width := m.frame()
	listWidth := lipgloss.Width(list)
	if y == height+lipgloss.Height(notice) {
		_, spots := m.keys()
		if spot, ok := hotspotAt(spots, x); ok {
			return m.act(spot)
		}
		return m, nil
	}
	if len(shown) == 0 || y >= height {
		return m, nil
	}
	if x >= listWidth+detailsPadding+width { // in the task list's column
		_, tasks := m.taskList(taskListWidth(m.projects, m.width), height)
		if y < len(tasks) && tasks[y] >= 0 {
			if next, chosen := m.runningTask(tasks[y]); chosen {
				return next, tea.Quit
			}
		}
		return m, nil
	}
	if x < listWidth-1 { // left of the list's border
		if i := m.firstShown(height) + y; i < len(shown) {
			return m.moved(i - m.cursor), nil
		}
		return m, nil
	}
	project := shown[m.cursor]
	windows := project.Project().Plan().Windows()
	if len(windows) == 0 {
		return m, nil
	}
	m.tasking = false // a window or pane clicked is no longer the task's
	laid := m.laidOut(project, windows, width, height)
	x -= listWidth + detailsPadding
	var spots []hotspot
	switch y {
	case windowListRow(laid.top):
		_, spots = windowList(project, windows, laid.index, width)
	case windowListRow(laid.top) + 1 + len(laid.panes):
		if laid.paneList == "" {
			return m, nil
		}
		if spot, ok := hotspotAt(laid.paneSpots, x); ok {
			m.pane = spot.item
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
	details := lipgloss.NewStyle().PaddingLeft(detailsPadding).Width(width + detailsPadding).
		Height(height).MaxHeight(height).Render(m.details(width, height))
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, list, details, tasks), notice, keysLine)
}

// frame is the notice, the list with its border and the task list's
// column, as render lays them out, the height they leave the list, the
// details and the task list above the notice and the keys, and the width
// the details' content has between the list and the task list.
func (m browser) frame() (notice, list, tasks string, height, width int) {
	notice = lipgloss.NewStyle().Width(m.width).Render(strings.Join(strings.Fields(m.notice), " "))
	height = max(m.height-lipgloss.Height(notice)-1, 1)
	list = lipgloss.NewStyle().Height(height).MaxHeight(height).
		Border(lipgloss.NormalBorder(), false, true, false, false).Render(m.list(height))
	tasks, _ = m.taskColumn(taskListWidth(m.projects, m.width), height)
	return notice, list, tasks, height, max(m.width-lipgloss.Width(list)-detailsPadding-lipgloss.Width(tasks), 1)
}

// detailsPadding is the cells between the list's border and the details.
const detailsPadding = 1

// keyTold is a key the keys line tells, and what it does.
type keyTold struct {
	key, does string
}

var (
	typingKeys   = []keyTold{{"enter", "keep"}, {"esc", "clear"}}
	filteredKeys = []keyTold{{"enter", "open"}, {"f", "freeze"}, {"e", "edit"}, {"E", "raw"}, {"x", "kill"}, {"d", "delete"}, {"esc", "clear"}, {"q", "quit"}}
	browsingKeys = []keyTold{{"enter", "open"}, {"f", "freeze"}, {"e", "edit"}, {"E", "raw"}, {"x", "kill"}, {"d", "delete"}, {"/", "filter"}, {"q", "quit"}}
)

// paneKey is tab told, before the last two keys, when the drawn window
// has panes to switch between, and taskKey t, when the selected project
// has tasks.
var (
	paneKey = keyTold{"tab", "pane"}
	taskKey = keyTold{"t", "tasks"}
)

// taskingKeys are told while the cursor is in the task list.
var taskingKeys = []keyTold{{"enter", "run"}, {"esc", "back"}, {"q", "quit"}}

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
	if windows := m.windowsShown(); !m.filtering && len(windows) > 0 && len(windows[m.drawn(windows)].Panes()) > 1 {
		told = slices.Insert(slices.Clone(told), len(told)-2, paneKey)
	}
	if !m.filtering && len(m.tasksShown()) > 0 {
		told = slices.Insert(slices.Clone(told), len(told)-2, taskKey)
	}
	if m.tasking {
		line, spots, told = "", nil, taskingKeys
	}
	for i, k := range told {
		if i > 0 {
			line += keysStyle.Render(" · ")
		}
		text := k.key + " " + k.does
		from := ansi.StringWidth(line)
		spots = append(spots, hotspot{from: from, to: from + ansi.StringWidth(text), key: k.key})
		line += keysStyle.Render(text)
	}
	return cut(line, spots, m.width)
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
	// " ● name root " and the border fit in half the width.
	rootWidth = min(rootWidth, max(m.width/2-nameWidth-6, 0))
	first := m.firstShown(height)
	lines := make([]string, 0, len(shown)-first)
	for i, project := range shown[first:] {
		mark := " "
		if project.Running() {
			mark = runningMark
		}
		name := fmt.Sprintf(" %s %-*s ", mark, nameWidth, project.Name())
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
	return lipgloss.NewStyle().Width(nameWidth + rootWidth + 5).Render(strings.Join(lines, "\n"))
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
		return "No projects yet: press f to freeze the current directory."
	case len(shown) == 0:
		return fmt.Sprintf("No project's name contains %q.", m.filter)
	}
	project := shown[m.cursor]
	windows := project.Project().Plan().Windows()
	if len(windows) == 0 {
		return detailsTop(project, width)
	}
	laid := m.laidOut(project, windows, width, height)
	panes := laid.panes
	if laid.paneList != "" {
		panes = append(panes, laid.paneList)
	}
	listLine, _ := windowList(project, windows, laid.index, width)
	return laid.top + "\n\n" + listLine + "\n" + strings.Join(append(panes, laid.levels...), "\n")
}

// detailsLayout is the parts of the details below the window list.
type detailsLayout struct {
	top       string // the config file and the summary
	index     int    // the drawn window's
	panes     []string
	paneList  string // under a drawing, empty when the panes are listed
	paneSpots []hotspot
	levels    []string
}

// laidOut is how the details of project, whose windows are windows, fit
// in width×height: its top, then below the window list, the drawn
// window's panes drawn or listed, its pane list and the levels of the
// selected pane in what is left.
func (m browser) laidOut(project app.ProjectOverview, windows []domain.WindowPlan, width, height int) detailsLayout {
	top := detailsTop(project, width)
	index := m.drawn(windows)
	pane := m.selectedPane(len(windows[index].Panes()))
	below := height - windowListRow(top) - 1 // the window list
	levelLines := levels(project.Project(), index, pane, width)
	panes, paneList, spots := m.panesShown(project, index, width, below, len(levelLines))
	shown := len(panes)
	if paneList != "" {
		shown++
	}
	return detailsLayout{top: top, index: index, panes: panes, paneList: paneList, paneSpots: spots,
		levels: fitted(levelLines, below-shown)}
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
	// A session opens at the browser's size, less tmux's status line.
	drawn := drawing(window, root, alerts(project, window), pane, false, m.width, max(m.height-1, 1),
		width, max(below-1-levelHeight, minDrawingHeight))
	line, spots := paneList(project.Project().Windows()[index], pane, width)
	return drawn, line, spots
}

// windowListRow is the details' line the window list is on, below top and a blank
// line.
func windowListRow(top string) int {
	return lipgloss.Height(top) + 1
}

// alerts are, by pane, the worst of project's warnings about window's
// panes: a directory missing before a program not found.
func alerts(project app.ProjectOverview, window domain.WindowPlan) []alert {
	panes := make([]alert, len(window.Panes()))
	for _, warning := range project.Warnings() {
		a := programAlert
		if errors.Is(warning, app.ErrMissingDirectory) {
			a = dirAlert
		}
		for i := range panes {
			if warning.Concerns(window.Name(), i) {
				panes[i] = max(panes[i], a)
			}
		}
	}
	return panes
}

// detailsTop is project's config file, then its summary or why its
// config can't be read, wrapped to width.
func detailsTop(project app.ProjectOverview, width int) string {
	title := project.Config().File
	if title == "" {
		title = project.Name().String()
	}
	header := summary(project)
	if err := project.Problem(); err != nil {
		header = problemStyle.Render(err.Error())
	}
	return lipgloss.NewStyle().Width(width).Render(titleStyle.Render(title) + "\n\n" + header)
}

// windowList is the names of windows, the selected one in a cyan block, ○ marking
// those the project's owned session lacks, and which one of how many is
// drawn, after ←→ when there are several to switch between; with the
// cells each window's name and each arrow covers, to click them. When they
// don't fit in width, the first ones make way for the selected one.
func windowList(project app.ProjectOverview, windows []domain.WindowPlan, selected, width int) (string, []hotspot) {
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
			label = selectedStyle.Render(label)
		}
		labels[i] = label
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

	var before []string
	for _, cmd := range project.Project().Before() {
		before = append(before, cmd.String())
	}
	if len(before) > 0 {
		lines = append(lines, keysStyle.Render("before "+strings.Join(before, " · ")))
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
