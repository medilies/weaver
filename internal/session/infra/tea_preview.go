package infra

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// previewEvery is how often the preview of the selected window is
// captured again.
const previewEvery = time.Second

// previewTick asks for the selected window's preview again.
type previewTick struct{}

// previewed is the preview of a project's window, or why there is none.
type previewed struct {
	project domain.ProjectName
	window  domain.WindowName
	preview domain.WindowPreview
	err     error
}

// tick is the next previewTick, previewEvery from now.
func tick() tea.Cmd {
	return tea.Tick(previewEvery, func(time.Time) tea.Msg { return previewTick{} })
}

// previewKey is the project and window the preview is wanted for: the
// selected window of the selected project, when the preview is shown and
// the project's owned session has the window.
func (m browser) previewKey() (app.ProjectOverview, domain.WindowName, bool) {
	shown := m.shown()
	if m.drawings || m.config || len(shown) == 0 || m.live == nil {
		return app.ProjectOverview{}, domain.WindowName{}, false
	}
	project := shown[m.cursor]
	windows := project.Project().Plan().Windows()
	if !project.Previewable() || len(windows) == 0 {
		return app.ProjectOverview{}, domain.WindowName{}, false
	}
	window := windows[m.drawn(windows)].Name()
	if slices.Contains(project.MissingWindows(), window) {
		return app.ProjectOverview{}, domain.WindowName{}, false // its drawing shows what enter restores
	}
	return project, window, true
}

// fetch captures the preview the browser wants, if any, away from the
// screen's updates.
func (m browser) fetch() tea.Cmd {
	project, window, ok := m.previewKey()
	if !ok {
		return nil
	}
	live := m.live
	return func() tea.Msg {
		preview, err := live.Preview(project, window)
		return previewed{project: project.Name(), window: window, preview: preview, err: err}
	}
}

// shownPreview is the preview drawn in place of the selected window's
// drawing, if it is the one wanted.
func (m browser) shownPreview() (domain.WindowPreview, bool) {
	project, window, ok := m.previewKey()
	if !ok || !m.hasPreview || m.preview.project != project.Name() || m.preview.window != window || !m.roomToDraw() {
		return domain.WindowPreview{}, false
	}
	return m.preview.preview, true
}

// roomToDraw reports whether the details have the room to draw the
// selected project's window, rather than list its panes. Miniatures don't
// change it: they are drawn only with room to spare.
func (m browser) roomToDraw() bool {
	_, _, _, height, width := m.frame()
	below := height - windowListRow(detailsTop(m.shown()[m.cursor], width)) - 1
	return width >= minDrawingWidth && below-1 >= minDrawingHeight
}

// withPreview is m after msg brought a preview, or why there is none: a
// preview no longer wanted is dropped; one for a new window selects the
// pane the drawing selected; a failure falls back to the drawing, saying
// why, and the next tick tries again.
func (m browser) withPreview(msg previewed) browser {
	project, window, ok := m.previewKey()
	if !ok || project.Name() != msg.project || window != msg.window {
		return m
	}
	_, wasShown := m.shownPreview()
	if msg.err != nil {
		m.hasPreview = false
		switch {
		case errors.Is(msg.err, domain.ErrSessionNotRunning):
			if wasShown {
				m.notice = fmt.Sprintf("preview: %s's session ended", msg.project)
			}
		case errors.Is(msg.err, domain.ErrWindowNotRunning):
			if wasShown {
				m.notice = fmt.Sprintf("preview: %s's window %s closed", msg.project, msg.window)
			}
		default:
			m.notice = "preview: " + msg.err.Error()
		}
		return m
	}
	panes := msg.preview.Panes()
	if !wasShown {
		m.previewPane = 0
		for i, pane := range panes {
			if index, ok := pane.InConfig(); ok && index == m.pane {
				m.previewPane = i
			}
		}
	}
	m.preview, m.hasPreview = msg, true
	m.previewPane = max(min(m.previewPane, len(panes)-1), 0)
	return m
}

// switchedView is m after p: the drawing instead of the preview, keeping
// the pane selected when the config has it, or the preview instead of the
// drawing, saying why when the selected project has none.
func (m browser) switchedView() (browser, tea.Cmd) {
	if preview, ok := m.shownPreview(); ok {
		if index, inConfig := preview.Panes()[m.previewPane].InConfig(); inConfig {
			m.pane = index
		}
		m.drawings = true
		return m, nil
	}
	m.drawings = false
	shown := m.shown()
	if len(shown) == 0 {
		return m, nil
	}
	project := shown[m.cursor]
	session, running := project.Session()
	switch {
	case !running:
		m.notice = fmt.Sprintf("preview: %s isn't running", project.Name())
	case !session.Owned():
		m.notice = fmt.Sprintf("preview: %s's session wasn't started by weaver", project.Name())
	case !m.roomToDraw():
		m.notice = "preview: the terminal is too small"
	}
	return m, m.fetch()
}

// previewNames are the names of preview's panes, as the session names
// them, or as the config does those the session doesn't, or their
// numbers.
func previewNames(preview domain.WindowPreview, window domain.WindowPlan) []string {
	panes := preview.Panes()
	names := make([]string, len(panes))
	for i, pane := range panes {
		names[i] = pane.Name()
		if index, ok := pane.InConfig(); names[i] == "" && ok && index < len(window.Panes()) {
			names[i] = window.Panes()[index].Name().String()
		}
		if names[i] == "" {
			names[i] = strconv.Itoa(i + 1)
		}
	}
	return names
}

// previewAlerts are the alerts of preview's panes: those of the config's
// panes they are, from alerts, and none for the others.
func previewAlerts(preview domain.WindowPreview, alerts []alert) []alert {
	panes := preview.Panes()
	shown := make([]alert, len(panes))
	for i, pane := range panes {
		if index, ok := pane.InConfig(); ok {
			shown[i] = alertOf(alerts, index)
		}
	}
	return shown
}

// selectedInConfig is the index of the selected pane among the config's
// panes of the drawn window of count panes, or false for a pane of the
// preview the config doesn't have.
func (m browser) selectedInConfig(count int) (int, bool) {
	if preview, ok := m.shownPreview(); ok {
		return preview.Panes()[m.previewPane].InConfig()
	}
	return m.selectedPane(count), true
}
