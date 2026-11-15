package infra

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// TeaBrowser shows the registered projects full screen: their names on
// the left, narrowed by a filter, the selected one's summary and
// a drawing of its windows in the middle, or the preview of the selected
// window of its owned session, its tasks on the right, and what the last
// action did below. It only shows and picks: the use case acts. It stays
// on screen from one choice to the next, until it leaves. Shown again on
// the same project, it keeps the window, pane and task selected, and the
// cursor in the task list; shown again at all, the drawing when it was
// switched to.
type TeaBrowser struct {
	kept   *browserPlace
	theme  theme
	every  time.Duration // how often the browser finds what it shows again
	screen *browserScreen
	held   *HeldOutput
}

// browserScreen is the browser while it is on screen: its program, the
// choices it makes and its end. Zero while it is off screen.
type browserScreen struct {
	program *tea.Program
	chosen  chan app.Choice
	ended   chan browserEnded
}

// browserEnded is how the browser's program ended.
type browserEnded struct {
	final tea.Model
	err   error
}

// reshown is the projects shown again in the browser on screen, after an
// action.
type reshown struct {
	projects []app.ProjectOverview
	selected domain.ProjectName
	notice   app.Notice
}

// asked is a question the browser on screen asks in its status line,
// answered on answer.
type asked struct {
	question string
	answer   chan<- bool
}

// doing is what the action the browser on screen waits for is doing.
type doing struct{ text string }

// browserPlace is what the browser had selected when it last ended.
type browserPlace struct {
	project            domain.ProjectName
	window, pane, task int
	tasking            bool
	filter             string
	config             bool // the config shown instead of the drawing
	drawings           bool // the drawing switched to in place of the preview
	abandoned          bool // quit while an action ran: not shown again
}

// NewTeaBrowser constructs a TeaBrowser styled by the settings' theme,
// starting on the preview or the drawing as they say, holding what is
// written to held while it is on screen.
func NewTeaBrowser(settings Settings, held *HeldOutput) TeaBrowser {
	return TeaBrowser{
		kept: &browserPlace{drawings: !settings.preview}, theme: settings.theme(), every: settings.previewEvery,
		screen: &browserScreen{}, held: held,
	}
}

// HeldOutput writes through to its writer, except while the browser is on
// screen, where it would show over it: it then holds what is written,
// each write once, until the browser leaves.
type HeldOutput struct {
	mu      sync.Mutex
	w       io.Writer
	holding bool
	held    []string
}

// NewHeldOutput constructs a HeldOutput writing to w.
func NewHeldOutput(w io.Writer) *HeldOutput {
	return &HeldOutput{w: w}
}

func (h *HeldOutput) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.holding {
		return h.w.Write(p)
	}
	if !slices.Contains(h.held, string(p)) {
		h.held = append(h.held, string(p))
	}
	return len(p), nil
}

// hold holds what is written from now on, if h is set.
func (h *HeldOutput) hold() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.holding = true
}

// release writes what was held, and writes through again, if h is set.
func (h *HeldOutput) release() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, text := range h.held {
		_, _ = io.WriteString(h.w, text) // as the writes held would have, with nowhere to tell a failure
	}
	h.holding, h.held = false, nil
}

// Browse implements app.ProjectBrowser. Once the user quit while an
// action ran, it is ErrAborted at once.
func (b TeaBrowser) Browse(projects []app.ProjectOverview, selected domain.ProjectName, notice app.Notice, live app.LiveSessions) (app.Choice, error) {
	if b.kept.abandoned {
		return app.Choice{}, app.ErrAborted
	}
	if b.screen.program != nil {
		b.screen.program.Send(reshown{projects: projects, selected: selected, notice: notice})
	} else {
		b.show(projects, selected, notice, live)
	}
	select {
	case choice := <-b.screen.chosen:
		return choice, nil
	case end := <-b.screen.ended:
		return app.Choice{}, b.ended(end)
	}
}

// Confirm implements app.ProjectBrowser: in the status line while the
// browser is on screen, else in the confirmation dialog.
func (b TeaBrowser) Confirm(question string) (bool, error) {
	if b.kept.abandoned {
		return false, app.ErrAborted
	}
	if b.screen.program == nil {
		return TeaConfirmer{theme: b.theme}.Confirm(question)
	}
	answer := make(chan bool, 1)
	b.screen.program.Send(asked{question: question, answer: answer})
	select {
	case yes := <-answer:
		return yes, nil
	case end := <-b.screen.ended:
		return false, b.ended(end)
	}
}

// Working implements app.ProjectBrowser.
func (b TeaBrowser) Working(text string) {
	if b.screen.program != nil {
		b.screen.program.Send(doing{text: text})
	}
}

// Leave implements app.ProjectBrowser.
func (b TeaBrowser) Leave() error {
	if b.screen.program == nil {
		return nil
	}
	b.screen.program.Quit()
	if err := b.ended(<-b.screen.ended); !errors.Is(err, app.ErrAborted) {
		return err
	}
	return nil
}

// show puts the browser on screen.
func (b TeaBrowser) show(projects []app.ProjectOverview, selected domain.ProjectName, notice app.Notice, live app.LiveSessions) {
	chosen := make(chan app.Choice, 1) // the browser waits after a choice, so one at most is unread
	model := browser{projects: projects, notice: notice, theme: b.theme, every: b.every, live: live, drawings: b.kept.drawings, chosen: chosen, spin: spinner.New(spinner.WithSpinner(spinner.MiniDot))}
	model.filter, model.config = b.kept.filter, b.kept.config
	if !slices.ContainsFunc(model.shown(), func(p app.ProjectOverview) bool { return p.Name() == selected }) {
		model.filter = ""
	}
	for i, project := range model.shown() {
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
	program := tea.NewProgram(model)
	ended := make(chan browserEnded, 1)
	b.held.hold()
	go func() {
		final, err := program.Run()
		ended <- browserEnded{final: final, err: err}
	}()
	*b.screen = browserScreen{program: program, chosen: chosen, ended: ended}
}

// ended is the browser off screen, what it had selected kept, and the
// error it ended with, or ErrAborted when the user quit.
func (b TeaBrowser) ended(end browserEnded) error {
	*b.screen = browserScreen{}
	b.held.release()
	if end.err != nil {
		return end.err
	}
	last := end.final.(browser)
	b.kept.drawings, b.kept.filter, b.kept.config, b.kept.abandoned = last.drawings, last.filter, last.config, last.abandoned
	if shown := last.shown(); len(shown) > 0 {
		*b.kept = browserPlace{project: shown[last.cursor].Name(), window: last.window, pane: last.pane,
			task: last.task, tasking: last.tasking, filter: last.filter, config: last.config,
			drawings: last.drawings, abandoned: last.abandoned}
	}
	return app.ErrAborted
}

// browserKeys are the keys making a choice.
var browserKeys = map[string]app.Action{
	"enter": app.ActionOpen, "s": app.ActionStart, "b": app.ActionBind, "e": app.ActionEdit, "E": app.ActionEditRaw,
	"x": app.ActionKill, "d": app.ActionDelete,
}

// browser is TeaBrowser's Bubble Tea model. The cursor is in the
// projects the filter shows, or in the task list when tasking, top is the
// first project the list shows, the window is the selected project's one
// drawn, the pane is its selected one, and the task the selected one in
// the task list.
type browser struct {
	theme      theme
	every      time.Duration
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
	previewFailure  string // why the last preview failed, told once
	previewAsked    int    // the asks for a preview
	previewAnswered int    // the last ask answered
	refreshStarted  bool   // asked, or found there is nothing to ask
	refreshAsked    int    // the asks for the projects as they are now
	refreshAnswered int    // the last ask answered
	notice          app.Notice
	width, height   int
	// choice is the one just made, sent on chosen; the browser then waits
	// for the projects to be shown again, while it may ask the question
	// whose answer goes on answer.
	choice   app.Choice
	chosen   chan<- app.Choice
	waiting  bool
	spin     spinner.Model // turning while waiting
	doing    string        // what the action waited for is doing, once told
	question string
	answer   chan<- bool
	// reloading is the question R asks, answered with p, w or any other
	// key cancelling: what to reload, the pane at reloadPane when
	// reloadsPane, or its whole window.
	reloading   string
	reloadPane  app.PaneAt
	reloadsPane bool
	// interrupted tells the user, who pressed ctrl+c while waiting, that
	// the action still runs; a second ctrl+c abandons it, ending the
	// browser.
	interrupted bool
	abandoned   bool
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

// Init starts the ticks asking for the preview and the selected project
// as its session is now; the first asks follow the terminal's size, read again once
// resizes are listened for.
func (m browser) Init() tea.Cmd {
	return tea.Batch(tick(m.every), sizeRecheck())
}

// Update is m after msg, its list scrolled to keep the cursor in view,
// and the preview captured at once when another one is wanted.
func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	start := m // as it was, to tell what changed
	switch msg := msg.(type) {
	case reshown:
		m = m.reshown(msg)
	case asked:
		m.question, m.answer = msg.question, msg.answer
	case doing:
		m.doing = msg.text
		return m, nil
	case spinner.TickMsg:
		if !m.waiting {
			return m, nil // stops turning
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if m.question != "" {
			return m.answered(msg.String())
		}
		if m.waiting {
			return m.pressedWaiting(msg.String())
		}
	case tea.MouseClickMsg:
		if m.question != "" || m.waiting || m.reloading != "" {
			return m, nil
		}
	}
	model, cmd := m.update(msg)
	next := model.(browser)
	if next.choice.Action != 0 {
		choice, chosen := next.choice, next.chosen
		next.choice, next.waiting = app.Choice{}, true
		cmd = tea.Batch(cmd, next.spin.Tick, func() tea.Msg {
			chosen <- choice
			return nil
		})
	}
	m = start
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
	if !next.refreshStarted || next.selectedName() != m.selectedName() {
		next.refreshStarted = true // the first ask, once the browser shows
		var fetch tea.Cmd
		next, fetch = next.refresh()
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
		var preview, refresh tea.Cmd
		if m.previewAsked == m.previewAnswered {
			m, preview = m.fetch()
		}
		if m.refreshAsked == m.refreshAnswered {
			m, refresh = m.refresh()
		}
		return m, tea.Batch(preview, refresh, tick(m.every))
	case refreshed:
		m.refreshAnswered = max(m.refreshAnswered, msg.asked)
		if msg.asked == m.refreshAsked {
			m = m.withRefreshed(msg.projects)
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
	if m.reloading != "" {
		return m.reloadAnswered(key), nil
	}
	if m.tasking && key != "q" && key != "ctrl+c" {
		next, _ := m.pressInTasks(key)
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
		return m, nil
	case "i":
		windows := m.windowsShown()
		if len(windows) == 0 || !m.shown()[m.cursor].Previewable() {
			break // only an owned session's panes are interrupted
		}
		window := windows[m.drawn(windows)]
		pane, inConfig := m.selectedInConfig(len(window.Panes()))
		if !inConfig {
			break
		}
		m.choice = app.Choice{Action: app.ActionInterruptPane, Project: m.shown()[m.cursor].Name(),
			Pane: app.PaneAt{Window: window.Name(), Pane: pane}}
		return m, nil
	case "R":
		return m.asksReload(), nil
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
		return m, nil
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
				next.choice.Action = app.ActionRunTaskStaying // a click leaves the user in the browser
				return next, nil
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
		_, spots = windowList(m.theme, project, windows, laid.index, width, laid.miniWidth(), !m.tasking)
	case row > 0 && row <= len(laid.miniatures):
		// A miniature is clicked as the name above it.
		_, spots = windowList(m.theme, project, windows, laid.index, width, laid.miniWidth(), !m.tasking)
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
	details := column(m.detailsTitle(), m.details(width, height), width+cardSides, height, m.theme.edge(false))
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, list, tasks, details), notice, keysLine)
}

// frame is the notice, the project list's card and the task list's
// beside it, as render lays them out, the lines they leave inside the
// cards above the notice and the keys, and the width the details' card
// has for its content right of the task list's.
func (m browser) frame() (notice, list, tasks string, height, width int) {
	notice = m.noticeLine()
	height = max(m.height-lipgloss.Height(notice)-1-2, 1)
	if len(m.projects) > 0 {
		projects := m.list(height)
		list = column(projectListTitle, projects, lipgloss.Width(projects)+cardSides, height, m.theme.edge(!m.tasking))
	}
	tasks, _ = m.taskColumn(taskListWidth(m.projects, m.width), height)
	return notice, list, tasks, height, max(m.width-lipgloss.Width(list)-lipgloss.Width(tasks)-cardSides, 1)
}

// noticeLine is the notice as frame lays it out, on as many lines as
// the width makes it: the question asked in the selected colour, as is
// the spinner while waiting with what the action is doing, and a failure
// in red after ✗.
func (m browser) noticeLine() string {
	style := lipgloss.NewStyle().Width(m.width)
	text := m.notice.Text()
	switch {
	case m.question != "":
		style, text = style.Inherit(m.theme.picked), m.question+" (y/n)"
	case m.reloading != "":
		style, text = style.Inherit(m.theme.picked), m.reloading
	case m.waiting:
		text = m.spin.View() + " " + m.doing
		if m.interrupted {
			text += " ctrl+c again leaves the browser."
		}
		style = style.Inherit(m.theme.picked)
	case m.notice.Failure():
		style, text = style.Inherit(m.theme.problem), "✗ "+text
	}
	return style.Render(strings.Join(strings.Fields(text), " "))
}

// reshown is m showing the projects again after an action, no longer
// waiting: the filter kept while it shows the project selected, the
// window, pane and task kept while that project is the one selected
// before.
func (m browser) reshown(msg reshown) browser {
	before := m.selectedName()
	m.projects, m.notice, m.waiting, m.doing, m.interrupted = msg.projects, msg.notice, false, "", false
	// An answer still on its way is about the projects as they were.
	m.refreshStarted, m.refreshAsked = false, m.refreshAsked+1
	m.refreshAnswered = m.refreshAsked
	if !slices.ContainsFunc(m.shown(), func(p app.ProjectOverview) bool { return p.Name() == msg.selected }) {
		m.filter, m.filtering = "", false
	}
	m.cursor = 0
	for i, project := range m.shown() {
		if project.Name() == msg.selected {
			m.cursor = i
		}
	}
	m = m.reselected(before)
	if m.tasking {
		if len(m.tasksShown()) == 0 {
			m.tasking = false
		} else {
			m = m.taskSelected(m.task) // the config may have changed: the task decides what is drawn
		}
	}
	return m
}

// pressedWaiting is m after key, pressed while an action runs: ctrl+c
// says the action still runs, and a second one ends the browser,
// abandoning it.
func (m browser) pressedWaiting(key string) (tea.Model, tea.Cmd) {
	if key != "ctrl+c" {
		return m, nil
	}
	if !m.interrupted {
		m.interrupted = true
		return m, nil
	}
	m.abandoned = true
	return m, tea.Quit
}

// asksReload is m asking what to reload in the selected project's owned
// session: the selected pane, when it is in the config, or the window
// drawn.
func (m browser) asksReload() browser {
	windows := m.windowsShown()
	if len(windows) == 0 || !m.shown()[m.cursor].Previewable() {
		return m
	}
	window := windows[m.drawn(windows)]
	pane, inConfig := m.selectedInConfig(len(window.Panes()))
	m.reloadPane, m.reloadsPane = app.PaneAt{Window: window.Name(), Pane: pane}, inConfig
	if !inConfig {
		m.reloading = fmt.Sprintf("Reload the whole %s window? (w, anything else cancels)", window.Name())
		return m
	}
	named := "pane " + strconv.Itoa(pane+1)
	if name := window.Panes()[pane].Name(); !name.IsZero() {
		named = "pane " + name.String()
	}
	m.reloading = fmt.Sprintf("Reload %s, or the whole %s window? (p/w, anything else cancels)", named, window.Name())
	return m
}

// reloadAnswered is m after key answers what to reload: p the pane, w its
// window, anything else nothing.
func (m browser) reloadAnswered(key string) browser {
	m.reloading = ""
	project := m.shown()[m.cursor].Name()
	switch {
	case key == "p" && m.reloadsPane:
		m.choice = app.Choice{Action: app.ActionReloadPane, Project: project, Pane: m.reloadPane}
	case key == "w":
		m.choice = app.Choice{Action: app.ActionReloadWindow, Project: project, Pane: m.reloadPane}
	default:
		m.notice = app.Telling("Nothing done.")
	}
	return m
}

// answered is m after key answers the question it asks: y or enter yes,
// anything else no.
func (m browser) answered(key string) (tea.Model, tea.Cmd) {
	yes, answer := key == "y" || key == "enter", m.answer
	m.question, m.answer = "", nil
	return m, func() tea.Msg {
		answer <- yes
		return nil
	}
}

// cardSides is the cells across a card's borders and the spaces inside
// them take.
const cardSides = 4

// column is content in a card titled title, width cells wide, its lines
// cut or filled to height, its border and title as shown says.
func column(title, content string, width, height int, shown edge) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), height)]
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(card(title, lines, width, shown), "\n")
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
	paneKey  = keyTold{"tab", "pane"}
	rerunKey = keyTold{"r", "rerun"}
	// interruptKey i when the selected project's session can be previewed,
	// or, in the task list, when the selected task runs.
	interruptKey = keyTold{"i", "interrupt"}
	// reloadKey R when the selected project's session can be previewed.
	reloadKey  = keyTold{"R", "reload"}
	taskKey    = keyTold{"t", "tasks"}
	configKey  = keyTold{"y", "config"}
	drawingKey = keyTold{"y", "drawing"}
	// previewKey p, or previewDrawingKey while the preview is shown, when
	// the selected project's session can be previewed.
	previewKey        = keyTold{"p", "preview"}
	previewDrawingKey = keyTold{"p", "drawing"}
)

// taskingKeys are told while the cursor is in the task list.
// answeringKeys are the keys told while the browser asks a question, and
// waitingKeys while an action runs.
var (
	answeringKeys = []keyTold{{"y or enter", "yes"}, {"any other key", "no"}}
	waitingKeys   = []keyTold{{"ctrl+c twice", "leave the browser"}}
	reloadingKeys = []keyTold{{"p", "pane"}, {"w", "window"}, {"any other key", "cancel"}}
)

var taskingKeys = []keyTold{{"enter", "run, go"}, {"r", "run"}, {"w", "throwaway"}, {"esc", "back"}, {"q", "quit"}}

// taskingKeys are taskingKeys, enter going where the selected task runs
// when it does.
func (m browser) taskingKeys() []keyTold {
	listed := m.tasksShown()
	if len(listed) == 0 || !m.taskRuns(listed[m.selectedTask(len(listed))].at) {
		return taskingKeys
	}
	told := slices.Clone(taskingKeys)
	told[0] = keyTold{"enter", "go"}
	return slices.Insert(told, 3, interruptKey)
}

// keys are the filter being typed and what its keys do, or the filter
// kept and the keys acting on what it shows, cut to the width; with the
// cells each key told covers, and the filter kept, which / edits again,
// to click them. ←→ is told beside the window list.
func (m browser) keys() (string, []hotspot) {
	var line string
	var spots []hotspot
	told := browsingKeys
	switch {
	case m.question != "":
		line, _ := m.theme.keys(answeringKeys) // keys aren't clicked while asking
		return cut(line, nil, m.width)
	case m.waiting:
		line, _ := m.theme.keys(waitingKeys) // nor while waiting
		return cut(line, nil, m.width)
	case m.reloading != "":
		told := reloadingKeys
		if !m.reloadsPane {
			told = told[1:]
		}
		line, _ := m.theme.keys(told) // nor while asking what to reload
		return cut(line, nil, m.width)
	case m.filtering:
		line, told = "/"+m.filter+m.theme.muted.Render("▏")+keysGap, typingKeys
	case m.filter != "":
		spots = append(spots, hotspot{from: 0, to: ansi.StringWidth("/" + m.filter), key: "/"})
		line, told = "/"+m.filter+keysGap, filteredKeys
	}
	preview, previewing := m.shownPreview()
	_, _, wanted := m.previewKey()
	if windows := m.windowsShown(); !m.filtering && len(windows) > 0 &&
		(previewing && len(preview.Panes()) > 1 || !previewing && len(windows[m.drawn(windows)].Panes()) > 1) {
		told = slices.Insert(slices.Clone(told), len(told)-2, paneKey)
	}
	if !m.filtering && m.selectedExited() {
		told = slices.Insert(slices.Clone(told), len(told)-2, rerunKey)
	}
	if !m.filtering && len(m.shown()) > 0 && m.shown()[m.cursor].Previewable() {
		told = slices.Insert(slices.Clone(told), len(told)-2, interruptKey, reloadKey)
	}
	if !m.filtering && len(m.tasksShown()) > 0 {
		told = slices.Insert(slices.Clone(told), len(told)-2, taskKey)
	}
	if !m.filtering && len(m.shown()) > 0 && m.shown()[m.cursor].Previewable() && !m.config {
		key := previewKey
		if wanted && m.previewFailure == "" {
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
		line, spots, told = "", nil, m.taskingKeys()
	}
	keys, keySpots := m.theme.keys(told)
	offset := ansi.StringWidth(line)
	for _, spot := range keySpots {
		spots = append(spots, hotspot{from: offset + spot.from, to: offset + spot.to, key: spot.key})
	}
	return cut(line+keys, spots, m.width)
}

// list is the shown projects' names, those whose session runs in green
// after a mark, the selected one bright after the cursor's mark, in the
// selected colour while the list holds the cursor, and kept in view. It
// keeps its width when the filter shows no project.
func (m browser) list(height int) string {
	shown := m.shown()
	nameWidth := m.nameWidth()
	first := m.firstShown(height)
	lines := make([]string, 0, len(shown)-first)
	for i, project := range shown[first:] {
		at := first+i == m.cursor
		mark, style := " ", lipgloss.NewStyle()
		if project.Running() {
			mark, style = m.theme.runningMark(), m.theme.ok
		}
		if at {
			style = style.Bold(true)
		}
		lines = append(lines, m.theme.gutter(at, !m.tasking)+mark+" "+style.Render(fmt.Sprintf("%-*s", nameWidth, project.Name())))
	}
	if len(m.projects) == 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(m.listWidth()).Render(strings.Join(lines, "\n"))
}

// projectListTitle is the title of the project list's card.
const projectListTitle = "projects"

// listWidth is the width of the project list: the cursor's mark, a mark
// and a name, or what its card's title needs when wider.
func (m browser) listWidth() int {
	return max(gutterWidth+2+m.nameWidth(), ansi.StringWidth(projectListTitle)+1)
}

// nameWidth is the width of the list's names: of all projects, so that
// filtering keeps the column still.
func (m browser) nameWidth() int {
	width := 0
	for _, project := range m.projects {
		width = max(width, len(project.Name().String()))
	}
	return width
}

// cardsBeside is the width the project list's card and the task list's
// take left of the details, as frame lays them out.
func (m browser) cardsBeside() int {
	width := 0
	if len(m.projects) > 0 {
		width += m.listWidth() + cardSides
	}
	if tasks := taskListWidth(m.projects, m.width); tasks > 0 {
		width += tasks + cardSides
	}
	return width
}

// firstShown is the first project the list shows in height lines: top,
// unless the cursor left the view, which scrolls the list as little as
// brings it back, or the list would end before the last line.
func (m browser) firstShown(height int) int {
	return max(min(m.top, m.cursor, len(m.shown())-height), m.cursor-height+1, 0)
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
		return m.detailsTop(project, width) + "\n\n" + configText(project.Config().Content, width)
	}
	windows := project.Project().Plan().Windows()
	if len(windows) == 0 {
		return m.detailsTop(project, width)
	}
	laid := m.laidOut(project, windows, width, height)
	panes := laid.panes
	if laid.paneList != "" {
		panes = append(panes, laid.paneList)
	}
	listLine, _ := windowList(m.theme, project, windows, laid.index, width, laid.miniWidth(), !m.tasking)
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
	top := m.detailsTop(project, width)
	index := m.drawn(windows)
	below := height - windowListRow(top) - 1 // the window list
	levelLines := []string{m.theme.muted.Render("not in the config")}
	if pane, inConfig := m.selectedInConfig(len(windows[index].Panes())); inConfig {
		levelLines = levels(m.theme, project.Project(), index, pane, width)
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
	_, spots := windowList(m.theme, project, windows, selected, width, miniatureWidth, !m.tasking)
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
		drawn := drawing(m.theme, window, root, alerts(project, window), -1, false, m.width, max(m.height-1, 1),
			miniatureWidth, miniatureHeight)
		for y := range rows {
			line := drawn[y]
			if spot.item == selected {
				line = m.theme.picked.Render(ansi.Strip(line))
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
		drawn := previewDrawing(m.theme, preview, previewNames(preview, window), previewAlerts(preview, alerts(project, window)),
			m.previewPane, width, max(below-1-levelHeight, minDrawingHeight))
		line, spots := namesList(m.theme, previewNames(preview, window), previewOrder(preview), m.previewPane, width,
			!m.tasking, previewAlerts(preview, alerts(project, window)))
		return drawn, line, spots
	}
	// A session opens at the browser's size, less tmux's status line.
	drawn := drawing(m.theme, window, root, alerts(project, window), pane, false, m.width, max(m.height-1, 1),
		width, max(below-1-levelHeight, minDrawingHeight))
	line, spots := paneList(m.theme, project.Project().Windows()[index], drawnOrder(window, m.width, max(m.height-1, 1)), pane, width,
		!m.tasking, alerts(project, window))
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
		a := alertOfWarning(warning)
		for i := range panes {
			if warning.Concerns(window.Name(), i) {
				panes[i] = max(panes[i], a)
			}
		}
	}
	return panes
}

// alertOfWarning is the alert a warning puts on the panes it concerns.
func alertOfWarning(warning app.Warning) alert {
	switch {
	case errors.Is(warning, app.ErrMissingDirectory):
		return dirAlert
	case errors.Is(warning, app.ErrCommandExited):
		return exitedAlert
	}
	return programAlert
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
func (m browser) detailsTop(project app.ProjectOverview, width int) string {
	header := m.summary(project)
	if err := project.Problem(); err != nil {
		header = m.theme.problem.Render(err.Error())
	}
	return lipgloss.NewStyle().Width(width).Render(header)
}

// windowList is the names of windows, the selected one chosen, in the
// selected colour while the list holds the cursor, as focused says, ○
// marking those the project's owned session lacks, each padded to pad
// cells to sit above its miniature, and which one of
// how many is drawn, after ←→ when there are several to switch between;
// with the cells each window's name and each arrow covers, to click
// them. When they don't fit in width, the first ones make way for the
// selected one.
func windowList(theme theme, project app.ProjectOverview, windows []domain.WindowPlan, selected, width, pad int, focused bool) (string, []hotspot) {
	missing := map[domain.WindowName]bool{}
	for _, name := range project.MissingWindows() {
		missing[name] = true
	}
	labels := make([]string, len(windows))
	for i, window := range windows {
		name := window.Name().String()
		if i == selected {
			name = theme.chosen(name, focused)
		}
		label := " " + name + " "
		if missing[window.Name()] {
			label = " ○" + label
		}
		labels[i] = label + strings.Repeat(" ", max(pad-ansi.StringWidth(label), 0))
	}
	count := fmt.Sprintf("%d/%d", selected+1, len(windows))
	if len(windows) > 1 {
		count = "←→ " + count
	}
	count = theme.muted.Render(count)
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
func (m browser) summary(project app.ProjectOverview) string {
	var lines []string
	if session, ok := project.Session(); ok {
		lines = append(lines, m.sessionLine(project, session))
	}
	rootLine := root(project)
	if !project.RootExists() {
		rootLine += " " + m.theme.problem.Render("✗ missing")
	}
	lines = append(lines, rootLine)

	for _, warning := range project.Warnings() {
		a := alertOfWarning(warning)
		style, _ := m.theme.alertStyle(a)
		lines = append(lines, style.Render(alertGlyph(a)+" "+warning.Error()))
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
			lines = append(lines, m.theme.muted.Render(list.label+" "+strings.Join(texts, " · ")))
		}
	}
	var env []string
	for _, v := range project.Project().Env().Vars() {
		env = append(env, v.Name()+"="+v.Value())
	}
	if len(env) > 0 {
		lines = append(lines, m.theme.muted.Render("env "+strings.Join(env, " · ")))
	}
	return strings.Join(lines, "\n")
}

// sessionLine is how the running session compares with the config, and
// so what enter does: "● running · 2/3 windows · "logs" missing, enter
// restores it".
func (m browser) sessionLine(project app.ProjectOverview, session domain.RunningSession) string {
	line := m.theme.runningMark() + " running"
	if !session.Owned() {
		return line + m.theme.muted.Render(" · not started by weaver, enter attaches as it is")
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
	return line + " · " + m.theme.warning.Render(strings.Join(names, ", ")+" missing, enter restores "+pronoun)
}
