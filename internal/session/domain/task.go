package domain

import "fmt"

// Task is a command a pane offers to run on request, in its project's
// running session: typed into the pane when its shell is idle, otherwise
// run in a throwaway window. It shows by its name, or else its command.
type Task struct {
	name    string
	command Command
}

// NewTask validates and constructs a Task running command, named rawName
// or unnamed when it is blank: trimmed, one line.
func NewTask(rawName string, command Command) (Task, error) {
	name, err := oneLine(rawName)
	if err != nil {
		return Task{}, err
	}
	if command.text == "" {
		return Task{}, ErrEmptyCommand // the zero Command, not one from NewCommand
	}
	return Task{name: name, command: command}, nil
}

// Name is the task's name, or "" when it is unnamed.
func (t Task) Name() string {
	return t.name
}

// Command is what the task types into its pane's shell and runs.
func (t Task) Command() Command {
	return t.command
}

// Label is how the task shows: its name, or else its command.
func (t Task) Label() string {
	if t.name != "" {
		return t.name
	}
	return t.command.text
}

// ThrowawayWindow is the name of the window the task runs in when its
// pane is busy: its own, or else its command's program, or else its
// command.
func (t Task) ThrowawayWindow() WindowName {
	name := t.name
	if name == "" {
		name = t.command.text
		if program, ok := t.command.Program(); ok {
			name = program
		}
	}
	return WindowName{value: name}
}

// checkTasks enforces that no two tasks share a name, nor two unnamed
// ones a command: each must be told apart from the others.
func checkTasks(tasks []Task) error {
	seen := make(map[Task]bool, len(tasks))
	for _, task := range tasks {
		key := Task{name: task.name}
		if task.name == "" {
			key.command = task.command
		}
		if seen[key] {
			return fmt.Errorf("task %q: %w", task.Label(), ErrDuplicateTask)
		}
		seen[key] = true
	}
	return nil
}
