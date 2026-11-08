package infra

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// TeaBrowser shows the registered projects full screen: their names and
// roots on the left, narrowed by a filter, the selected one's summary and
// config on the right, and what the last action did below. It only shows
// and picks: the use case acts.
type TeaBrowser struct{}

// NewTeaBrowser constructs a TeaBrowser.
func NewTeaBrowser() TeaBrowser {
	return TeaBrowser{}
}

// Browse implements app.ProjectBrowser.
func (TeaBrowser) Browse(projects []app.ProjectOverview, selected domain.ProjectName, notice string) (app.Choice, error) {
	model := browser{projects: projects, notice: notice}
	for i, project := range projects {
		if project.Name() == selected {
			model.cursor = i
		}
	}
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return app.Choice{}, err
	}
	choice := final.(browser).choice
	if choice.Action == 0 {
		return app.Choice{}, app.ErrAborted
	}
	return choice, nil
}

// browserKeys are the keys making a choice.
var browserKeys = map[string]app.Action{
	"enter": app.ActionOpen, "f": app.ActionFreeze, "e": app.ActionEdit, "x": app.ActionKill, "d": app.ActionDelete,
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
// projects the filter shows.
type browser struct {
	projects      []app.ProjectOverview
	cursor        int
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
	return m
}

func (m browser) Init() tea.Cmd {
	return nil
}

func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
	switch key {
	case "up", "k":
		m = m.moved(-1)
	case "down", "j":
		m = m.moved(1)
	case "/":
		m.filtering = true
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

// typeFilter edits the filter as it is typed: enter keeps it for the
// keys to act on what it shows, esc clears it.
func (m browser) typeFilter(msg tea.KeyPressMsg) browser {
	switch msg.String() {
	case "up":
		return m.moved(-1)
	case "down":
		return m.moved(1)
	case "enter":
		m.filtering = false
		return m
	case "esc":
		m.filtering = false
		return m.filtered("")
	case "backspace":
		runes := []rune(m.filter)
		return m.filtered(string(runes[:max(len(runes)-1, 0)]))
	}
	if msg.Text == "" {
		return m
	}
	return m.filtered(m.filter + msg.Text)
}

// moved is m with the cursor by up to by projects down, or up when by is
// negative, staying in the projects shown.
func (m browser) moved(by int) browser {
	m.cursor = max(min(m.cursor+by, len(m.shown())-1), 0)
	return m
}

func (m browser) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

// render lays out the list beside the details, above the notice and the
// keys.
func (m browser) render() string {
	if m.width == 0 {
		return "" // until the terminal's size is known
	}
	notice := lipgloss.NewStyle().Width(m.width).Render(strings.Join(strings.Fields(m.notice), " "))
	keys := m.keys()
	height := max(m.height-lipgloss.Height(notice)-1, 1)

	list := lipgloss.NewStyle().Height(height).MaxHeight(height).
		Border(lipgloss.NormalBorder(), false, true, false, false).Render(m.list(height))
	details := lipgloss.NewStyle().PaddingLeft(1).Width(max(m.width-lipgloss.Width(list), 1)).
		Height(height).MaxHeight(height).Render(m.details())
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, list, details), notice, keys)
}

// keys are the filter being typed and what its keys do, or the filter
// kept and the keys acting on what it shows.
func (m browser) keys() string {
	switch {
	case m.filtering:
		return "/" + m.filter + keysStyle.Render("▏ enter keep · esc clear")
	case m.filter != "":
		return "/" + m.filter + keysStyle.Render(" · enter open · f freeze · e edit · x kill · d delete · esc clear · q quit")
	}
	return keysStyle.Render("enter open · f freeze · e edit · x kill · d delete · / filter · q quit")
}

// list is the shown projects' names and dimmed roots, the selected one
// highlighted and kept in view, and a mark for those whose session runs.
// It takes at most half the width, cutting roots short from the left.
func (m browser) list(height int) string {
	shown := m.shown()
	nameWidth, rootWidth := 0, 0 // of all projects, so filtering keeps the column still
	for _, project := range m.projects {
		nameWidth = max(nameWidth, len(project.Name().String()))
		rootWidth = max(rootWidth, ansi.StringWidth(root(project)))
	}
	// " ● name root " and the border fit in half the width.
	rootWidth = min(rootWidth, max(m.width/2-nameWidth-6, 0))
	first := max(m.cursor-height+1, 0) // scrolled down to the cursor
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
	return strings.Join(lines, "\n")
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

// details are the selected project's summary and config as written, or
// why it can't be read.
func (m browser) details() string {
	shown := m.shown()
	switch {
	case len(m.projects) == 0:
		return "No projects yet: press f to freeze the current directory."
	case len(shown) == 0:
		return fmt.Sprintf("No project's name contains %q.", m.filter)
	}
	project := shown[m.cursor]
	config := project.Config()
	if config.File == "" && project.Problem() != nil {
		return titleStyle.Render(project.Name().String()) + "\n\n" + problemStyle.Render(project.Problem().Error())
	}
	header := summary(project)
	if err := project.Problem(); err != nil {
		header = problemStyle.Render(err.Error())
	}
	return titleStyle.Render(config.File) + "\n\n" + header + "\n\n" + strings.TrimRight(string(config.Content), "\n")
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
