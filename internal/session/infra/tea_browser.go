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
// the left, the selected one's config on the right, and what the last
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

// browserKeys are the keys acting on the selected project.
var browserKeys = map[string]app.Action{
	"enter": app.ActionOpen, "e": app.ActionEdit, "x": app.ActionKill, "d": app.ActionDelete,
}

var (
	runningMark  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	titleStyle   = lipgloss.NewStyle().Bold(true)
	keysStyle    = lipgloss.NewStyle().Faint(true)
	problemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
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

// press moves the cursor, or ends with the choice the key makes. Only a
// running project's session can be killed.
func (m browser) press(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = max(min(m.cursor+1, len(m.projects)-1), 0)
	case "a":
		m.choice = app.Choice{Action: app.ActionAdd}
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	default:
		action, ok := browserKeys[key]
		if !ok || len(m.projects) == 0 {
			break
		}
		project := m.projects[m.cursor]
		if action == app.ActionKill && !project.Running() {
			break
		}
		m.choice = app.Choice{Action: action, Project: project.Name()}
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

// details are the selected project's config as written, or why it can't
// be read.
func (m browser) details() string {
	if len(m.projects) == 0 {
		return "No projects yet: press a to add the current directory."
	}
	project := m.projects[m.cursor]
	if err := project.Problem(); err != nil {
		return titleStyle.Render(project.Name().String()) + "\n\n" + problemStyle.Render(err.Error())
	}
	config := project.Config()
	return titleStyle.Render(config.File) + "\n\n" + strings.TrimRight(string(config.Content), "\n")
}
