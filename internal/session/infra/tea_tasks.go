package infra

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// listedTask is one of a project's tasks in the task list: where it is,
// the window and pane it is shown under, and how it shows.
type listedTask struct {
	at     app.TaskAt
	window int // the window's index, to draw it
	group  string
	label  string
}

// tasksOf are project's tasks in its panes' order, each under its pane's
// window and name, or its window alone for a window's only, unnamed pane:
// "api › back", "logs".
func tasksOf(project domain.Project) []listedTask {
	var listed []listedTask
	for w, window := range project.Windows() {
		for p, pane := range window.Panes() {
			group := window.Name().String()
			if !pane.Name().IsZero() {
				group += " › " + pane.Name().String()
			}
			for t, task := range pane.Tasks() {
				listed = append(listed, listedTask{
					at:     app.TaskAt{Window: window.Name(), Pane: p, Task: t},
					window: w, group: group, label: task.Label(),
				})
			}
		}
	}
	return listed
}

// taskListTitle heads the task list.
const taskListTitle = "tasks"

// taskListWidth is the width the task list's lines take, wide enough for
// its title and every project's lines, so moving between projects keeps the column still, but
// at most a quarter of width; 0 when no project has tasks, which leaves
// the column out.
func taskListWidth(projects []app.ProjectOverview, width int) int {
	widest := 0
	for _, project := range projects {
		for _, task := range tasksOf(project.Project()) {
			widest = max(widest, gutterWidth+ansi.StringWidth(task.group), gutterWidth+2+ansi.StringWidth(task.label))
		}
	}
	if widest == 0 {
		return 0
	}
	widest = max(widest, ansi.StringWidth(taskListTitle))
	return min(widest, max(width/4-cardSides, 1))
}

// taskColumn is the task list in its card, its lines height tall and
// width cells wide, with the task each line shows, or -1; none when width
// is 0.
func (m browser) taskColumn(width, height int) (string, []int) {
	if width == 0 {
		return "", nil
	}
	lines, tasks := m.taskList(width, height)
	return column(taskListTitle, strings.Join(lines, "\n"), width+cardSides, height, m.theme.edge(m.tasking)), tasks
}

// taskList is the selected project's tasks under their window and pane,
// ● before those still running, the selected one bright after the
// cursor's mark while the cursor is in the list, kept in view in height
// lines cut to width; with the task each line shows, or -1.
func (m browser) taskList(width, height int) ([]string, []int) {
	// Cut, as every line is, so that each stays one row, as clicks count them.
	var lines []string
	var tasks []int
	listed := m.tasksShown()
	if len(listed) == 0 {
		return []string{m.theme.muted.Render("none")}, []int{-1}
	}
	selected, group := 0, ""
	for i, task := range listed {
		if task.group != group {
			group = task.group
			lines = append(lines, strings.Repeat(" ", gutterWidth)+m.theme.muted.Render(ansi.Truncate(group, width-gutterWidth, "…")))
			tasks = append(tasks, -1)
		}
		mark := " "
		if m.taskRuns(task.at) {
			mark = m.theme.runningMark()
		}
		label := ansi.Truncate(task.label, max(width-gutterWidth-2, 0), "…")
		at := m.tasking && i == m.selectedTask(len(listed))
		if at {
			selected = len(lines)
			label = m.theme.bright.Render(label)
		}
		lines = append(lines, m.theme.gutter(at, true)+mark+" "+label)
		tasks = append(tasks, i)
	}
	first := max(selected-height+1, 0)
	return lines[first:], tasks[first:]
}

// tasksShown are the selected project's tasks.
func (m browser) tasksShown() []listedTask {
	shown := m.shown()
	if len(shown) == 0 {
		return nil
	}
	return tasksOf(shown[m.cursor].Project())
}

// selectedTask is the selected task's index among count.
func (m browser) selectedTask(count int) int {
	return max(min(m.task, count-1), 0)
}

// taskSelected is m with the task at index of the selected project's
// selected, its window drawn and its pane selected.
func (m browser) taskSelected(index int) browser {
	listed := m.tasksShown()
	if len(listed) == 0 {
		return m
	}
	m.task = max(min(index, len(listed)-1), 0)
	task := listed[m.task]
	m.window, m.pane = task.window, task.at.Pane
	return m.previewFollowing()
}

// pressInTasks is m after key, pressed with the cursor in the task list:
// ↑↓ select a task, enter runs it and goes there, r runs it, staying, w
// runs it in a throwaway window, staying, i interrupts it when it runs,
// esc goes back to the projects.
func (m browser) pressInTasks(key string) (browser, bool) {
	switch key {
	case "up", "k":
		return m.taskSelected(m.selectedTask(len(m.tasksShown())) - 1), false
	case "down", "j":
		return m.taskSelected(m.selectedTask(len(m.tasksShown())) + 1), false
	case "esc":
		m.tasking = false
		return m, false
	case "enter":
		return m.runningTask(m.selectedTask(len(m.tasksShown())))
	case "r":
		next, chosen := m.runningTask(m.selectedTask(len(m.tasksShown())))
		if chosen {
			next.choice.Action = app.ActionRunTaskStaying
		}
		return next, chosen
	case "i":
		listed := m.tasksShown()
		if len(listed) == 0 || !m.taskRuns(listed[m.selectedTask(len(listed))].at) {
			return m, false
		}
		next, chosen := m.runningTask(m.selectedTask(len(listed)))
		if chosen {
			next.choice.Action = app.ActionInterruptTask
		}
		return next, chosen
	case "w":
		next, chosen := m.runningTask(m.selectedTask(len(m.tasksShown())))
		if chosen {
			next.choice.Action = app.ActionRunTaskApart
		}
		return next, chosen
	}
	return m, false
}

// runningTask is m choosing to run the selected project's task at index,
// and true to end with that choice.
func (m browser) runningTask(index int) (browser, bool) {
	listed := m.tasksShown()
	if index < 0 || index >= len(listed) {
		return m, false
	}
	m = m.taskSelected(index)
	m.choice = app.Choice{Action: app.ActionRunTask, Project: m.shown()[m.cursor].Name(), Task: listed[index].at}
	return m, true
}

// taskRuns reports whether the selected project's task at still runs, as
// last found while the browser shows it.
func (m browser) taskRuns(at app.TaskAt) bool {
	return m.shown()[m.cursor].TaskRuns(at)
}

// refreshed is the projects as they were found, in answer to the ask
// numbered asked.
type refreshed struct {
	asked    int
	projects []app.ProjectOverview
}

// refresh is m asking for the projects as they are now, the selected one
// built again in full, away from the screen's updates; an answer to an
// earlier ask is then dropped.
func (m browser) refresh() (browser, tea.Cmd) {
	if len(m.shown()) == 0 || m.live == nil {
		return m, nil
	}
	m.refreshAsked++
	asked, projects, selected, live := m.refreshAsked, m.projects, m.selectedName(), m.live
	return m, func() tea.Msg {
		return refreshed{asked: asked, projects: live.Live(projects, selected)}
	}
}

// withRefreshed is m with projects in place of the overviews of the same
// names, so that what they show is what was found now.
func (m browser) withRefreshed(projects []app.ProjectOverview) browser {
	byName := make(map[domain.ProjectName]app.ProjectOverview, len(projects))
	for _, project := range projects {
		byName[project.Name()] = project
	}
	m.projects = slices.Clone(m.projects)
	for i, p := range m.projects {
		if project, ok := byName[p.Name()]; ok {
			m.projects[i] = project
		}
	}
	return m
}
