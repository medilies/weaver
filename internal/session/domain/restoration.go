package domain

// WindowPlacement is a window to add to a running session, and where:
// after a window, before one, or at the end when neither.
type WindowPlacement struct {
	window WindowPlan
	after  WindowName
	before WindowName
}

func (p WindowPlacement) Window() WindowPlan {
	return p.window
}

// After is the window to put it after, if any.
func (p WindowPlacement) After() (WindowName, bool) {
	return p.after, p.after.value != ""
}

// Before is the window to put it before, if any.
func (p WindowPlacement) Before() (WindowName, bool) {
	return p.before, p.before.value != ""
}

// Restoration plans the project windows a running session lacks, given
// the ones it still has, placed where the project lists them: after the
// window listed before them, else before the first window left. Only the
// missing windows are resolved, so a session missing nothing never fails
// on a runtime uninstalled since it started.
func (p Project) Restoration(present []WindowName, resolver RuntimeResolver) ([]WindowPlacement, error) {
	have := make(map[string]bool, len(present))
	for _, name := range present {
		have[name.value] = true
	}
	var firstLeft WindowName
	for _, window := range p.windows {
		if have[window.name.value] {
			firstLeft = window.name
			break
		}
	}
	var placements []WindowPlacement
	var previous WindowName
	for _, window := range p.windows {
		if have[window.name.value] {
			previous = window.name
			continue
		}
		planned, err := p.planWindow(window, resolver)
		if err != nil {
			return nil, err
		}
		placement := WindowPlacement{window: planned, after: previous}
		if previous.value == "" {
			placement.before = firstLeft
		}
		placements = append(placements, placement)
		previous = window.name
	}
	return placements, nil
}
