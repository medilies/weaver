package infra

import (
	"strings"

	"charm.land/lipgloss/v2"
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

// taskListPadding is the cells around the task list's lines, in its
// column between the project list and the details: a space on the left,
// a space and a border on the right.
const taskListPadding = 3

// taskListWidth is the width the task list's lines take, wide enough for
// every project's, so moving between projects keeps the column still, but
// at most a quarter of width; 0 when no project has tasks, which leaves
// the column out.
func taskListWidth(projects []app.ProjectOverview, width int) int {
	widest := 0
	for _, project := range projects {
		for _, task := range tasksOf(project.Project()) {
			widest = max(widest, ansi.StringWidth(task.group), 2+ansi.StringWidth(task.label))
		}
	}
	if widest == 0 {
		return 0
	}
	return min(widest, max(width/4-taskListPadding, 1))
}

// taskColumn is the task list in its column, height lines tall, its
// lines width cells wide, with the task each line shows, or -1; none when
// width is 0.
func (m browser) taskColumn(width, height int) (string, []int) {
	if width == 0 {
		return "", nil
	}
	lines, tasks := m.taskList(width, height)
	return lipgloss.NewStyle().Width(width+taskListPadding).Height(height).MaxHeight(height).
		Border(lipgloss.NormalBorder(), false, true, false, false).Padding(0, 1).
		Render(strings.Join(lines, "\n")), tasks
}

// taskList is the selected project's tasks under their window and pane,
// the selected one highlighted while the cursor is in the list, kept in
// view in height lines cut to width; with the task each line shows, or -1.
func (m browser) taskList(width, height int) ([]string, []int) {
	lines := []string{titleStyle.Render("tasks")}
	tasks := []int{-1}
	listed := m.tasksShown()
	if len(listed) == 0 {
		return append(lines, keysStyle.Render("none")), append(tasks, -1)
	}
	selected, group := 0, ""
	for i, task := range listed {
		if task.group != group {
			group = task.group
			lines = append(lines, keysStyle.Render(ansi.Truncate(group, width, "…")))
			tasks = append(tasks, -1)
		}
		label := ansi.Truncate("  "+task.label, width, "…")
		if m.tasking && i == m.selectedTask(len(listed)) {
			selected = len(lines)
			label = cursorStyle.Render(label + strings.Repeat(" ", width-ansi.StringWidth(label)))
		}
		lines = append(lines, label)
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
	return m
}

// pressInTasks is m after key, pressed with the cursor in the task list:
// ↑↓ select a task, enter runs it, esc goes back to the projects.
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
