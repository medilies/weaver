package infra

import (
	"fmt"
	"maps"
)

// keyAction is an action a key setting can move to another key: the
// setting's name under keys, its own key, and the lists of the screen it
// applies in, the keys of one list never shared.
type keyAction struct {
	field, key string
	lists      []string
}

// Where in the browser an action applies: the project list, or the task
// list holding the cursor.
const (
	inProjects = "the project list"
	inTasks    = "the task list"
)

// browserActions are the browser's keys the settings can change.
var browserActions = []keyAction{
	{"start", "s", []string{inProjects}},
	{"bind", "b", []string{inProjects}},
	{"edit", "e", []string{inProjects}},
	{"edit-raw", "E", []string{inProjects}},
	{"kill", "x", []string{inProjects}},
	{"delete", "d", []string{inProjects}},
	{"rerun", "r", []string{inProjects}},
	{"reload", "R", []string{inProjects}},
	{"preview", "p", []string{inProjects}},
	{"config", "y", []string{inProjects}},
	{"tasks", "t", []string{inProjects}},
	{"filter", "/", []string{inProjects}},
	{"run", "r", []string{inTasks}},
	{"throwaway", "w", []string{inTasks}},
	{"interrupt", "i", []string{inProjects, inTasks}},
	{"quit", "q", []string{inProjects, inTasks}},
}

// inForm is the config form's one list: an action's key is never
// another's there, the drawing's keys included.
const inForm = "the config form"

// formActions are the config form's keys the settings can change, but
// for the drawing keys, which DrawingKeys holds.
var formActions = []keyAction{
	{"undo", "u", []string{inForm}},
	{"redo", "ctrl+r", []string{inForm}},
	{"save", "ctrl+s", []string{inForm}},
	{"move-up", "K", []string{inForm}},
	{"move-down", "J", []string{inForm}},
}

// drawingActions are the drawing keys, for checking them with the
// config form's other keys.
var drawingActions = []keyAction{
	{"split-right", "%", []string{inForm}},
	{"split-below", "\"", []string{inForm}},
	{"remove", "x", []string{inForm}},
	{"shrink", "H", []string{inForm}},
	{"grow", "L", []string{inForm}},
}

// formFixedKeys are the config form's keys no setting changes, and what
// they do.
var formFixedKeys = map[string]string{
	"up": "move", "down": "move", "k": "move", "j": "move",
	"left": "change", "right": "change", "h": "change", "l": "change",
	"enter": "edit", "o": "add", "d": "delete", "esc": "leave", "ctrl+c": "leave",
}

// browserFixedKeys are the browser's keys no setting changes, and what
// they do.
var browserFixedKeys = map[string]string{
	"up": "move", "down": "move", "k": "move", "j": "move",
	"left": "pick a window", "right": "pick a window",
	"tab": "pick a pane", "shift+tab": "pick a pane",
	"enter": "open", "esc": "go back", "ctrl+c": "quit",
}

// keymap is a screen's keys as the settings set them: by list, each
// action's own key by the key that does it now.
type keymap map[string]map[string]string

// newKeymap is the keymap of actions, set is their keys by setting name,
// those left out keeping their own.
func newKeymap(actions []keyAction, set map[string]string) keymap {
	m := keymap{}
	for _, action := range actions {
		key := action.key
		if bound, ok := set[action.field]; ok {
			key = bound
		}
		for _, list := range action.lists {
			if m[list] == nil {
				m[list] = map[string]string{}
			}
			m[list][key] = action.key
		}
	}
	return m
}

// pressed is what pressed does in list: the action's own key when the
// settings moved an action to it, "" when it is an action's own key the
// settings moved elsewhere, else pressed itself.
func (m keymap) pressed(list, pressed string) string {
	if own, ok := m[list][pressed]; ok {
		return own
	}
	for _, own := range m[list] {
		if own == pressed {
			return ""
		}
	}
	return pressed
}

// shown is the key doing in list what own does unless the settings say
// otherwise.
func (m keymap) shown(list, own string) string {
	for key, action := range m[list] {
		if action == own {
			return key
		}
	}
	return own
}

// checkKeys reports, for actions whose keys are set, a key two actions of
// one list share, or one fixed does, naming a setting that moved it:
// actions' own keys never clash.
func checkKeys(actions []keyAction, set, fixed map[string]string) error {
	byList := map[string]map[string]string{} // by list, what each key does
	for _, action := range actions {
		key := action.key
		if bound, ok := set[action.field]; ok {
			key = bound
		}
		for _, list := range action.lists {
			if byList[list] == nil {
				byList[list] = maps.Clone(fixed)
			}
			taken, this := byList[list], "keys."+action.field
			if does, ok := taken[key]; ok && does != this {
				if _, moved := set[action.field]; !moved {
					does, this = this, does // the other moved onto this one's key
				}
				return fmt.Errorf("%s: %w: %q already does %s in %s", this, ErrKeyTaken, key, does, list)
			}
			taken[key] = this
		}
	}
	return nil
}
