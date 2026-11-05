package infra

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// TeaBrowser shows the registered projects full screen: their names on
// the left, the selected one's summary and config on the right, and what the last
// action did below. It only shows and picks: the use case acts.
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
	"enter": app.ActionOpen, "a": app.ActionAdd, "e": app.ActionEdit, "x": app.ActionKill, "d": app.ActionDelete,
}

var (
	runningMark  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	titleStyle   = lipgloss.NewStyle().Bold(true)
	keysStyle    = lipgloss.NewStyle().Faint(true)
	problemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	clonedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
)

// browser is TeaBrowser's Bubble Tea model.
type browser struct {
	projects      []app.ProjectOverview
	cursor        int
	notice        string
	width, height int
	choice        app.Choice
}

func (m browser) Init() tea.Cmd {
	return nil
}

func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m.press(msg.String())
	}
	return m, nil
}

// press moves the cursor, or ends with the choice the key makes.
func (m browser) press(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = max(min(m.cursor+1, len(m.projects)-1), 0)
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	default:
		action, ok := browserKeys[key]
		if !ok || (len(m.projects) == 0 && action != app.ActionAdd) {
			break
		}
		m.choice = app.Choice{Action: action}
		if len(m.projects) > 0 {
			m.choice.Project = m.projects[m.cursor].Name()
		}
		return m, tea.Quit
	}
	return m, nil
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
	keys := keysStyle.Render("enter open · a add · e edit · x kill · d delete · q quit")
	height := max(m.height-lipgloss.Height(notice)-1, 1)

	list := lipgloss.NewStyle().Height(height).MaxHeight(height).
		Border(lipgloss.NormalBorder(), false, true, false, false).Render(m.list(height))
	details := lipgloss.NewStyle().PaddingLeft(1).Width(max(m.width-lipgloss.Width(list), 1)).
		Height(height).MaxHeight(height).Render(m.details())
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, list, details), notice, keys)
}

// list is the projects' names, the selected one highlighted and kept in
// view, and a mark for those whose session runs.
func (m browser) list(height int) string {
	width := 0
	for _, project := range m.projects {
		width = max(width, len(project.Name().String()))
	}
	first := max(m.cursor-height+1, 0) // scrolled down to the cursor
	lines := make([]string, 0, len(m.projects)-first)
	for i, project := range m.projects[first:] {
		mark := " "
		if project.Running() {
			mark = runningMark
		}
		line := fmt.Sprintf(" %s %-*s ", mark, width, project.Name())
		if first+i == m.cursor {
			line = cursorStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// details are the selected project's summary and config as written, or
// why it can't be read.
func (m browser) details() string {
	if len(m.projects) == 0 {
		return "No projects yet: press a to add the current directory."
	}
	project := m.projects[m.cursor]
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

// summary is where the project stands: its root and the working tree it
// is in, its repositories, and what every pane starts with.
func summary(project app.ProjectOverview) string {
	root := contractHome(project.Project().Root().String())
	if !project.RootExists() {
		root += " " + problemStyle.Render("✗ missing")
	} else if tree, ok := project.WorkingTree(); ok {
		root += "  " + workingTree(tree)
	}
	lines := []string{root}

	var checkouts []string
	for _, status := range project.Checkouts() {
		dir := status.Checkout().Dir()
		if dir == "." {
			dir = "root"
		}
		if status.Cloned() {
			checkouts = append(checkouts, dir+" "+clonedStyle.Render("✓ cloned"))
		} else {
			checkouts = append(checkouts, dir+" "+problemStyle.Render("✗ missing"))
		}
	}
	if len(checkouts) > 0 {
		lines = append(lines, strings.Join(checkouts, " · "))
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

// workingTree reads like a shell prompt's: "main* ↑2 ↓1", the star for
// changes not committed.
func workingTree(tree domain.WorkingTree) string {
	text := tree.Branch()
	if tree.Detached() {
		text = "(detached)"
	}
	if tree.Dirty() {
		text += "*"
	}
	if tree.Ahead() > 0 {
		text += fmt.Sprintf(" ↑%d", tree.Ahead())
	}
	if tree.Behind() > 0 {
		text += fmt.Sprintf(" ↓%d", tree.Behind())
	}
	return text
}
