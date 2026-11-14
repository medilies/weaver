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

// previewEvery is how often the preview of the selected window, and the
// selected project's running tasks, are found again.
const previewEvery = time.Second

// previewTick asks for the selected window's preview and the running
// tasks again.
type previewTick struct{}

// previewed is the preview of a project's window, or why there is none,
// in answer to the ask numbered asked.
type previewed struct {
	asked   int
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
// selected window of the selected project, when the preview is shown, the
// project's owned session has the window, and the details have the room to
// draw it.
func (m browser) previewKey() (app.ProjectOverview, domain.WindowName, bool) {
	shown := m.shown()
	if m.drawings || m.config || len(shown) == 0 || m.live == nil {
		return app.ProjectOverview{}, domain.WindowName{}, false
	}
	project := shown[m.cursor]
	windows := project.Project().Plan().Windows()
	if !project.Previewable() || len(windows) == 0 || !m.roomToDraw() {
		return app.ProjectOverview{}, domain.WindowName{}, false
	}
	window := windows[m.drawn(windows)].Name()
	if slices.Contains(project.MissingWindows(), window) {
		return app.ProjectOverview{}, domain.WindowName{}, false // its drawing shows what enter restores
	}
	return project, window, true
}

// fetch is m asking for the preview it wants, if any, away from the
// screen's updates; an answer to an earlier ask is then dropped.
func (m browser) fetch() (browser, tea.Cmd) {
	project, window, ok := m.previewKey()
	if !ok {
		return m, nil
	}
	m.previewAsked++
	asked, live := m.previewAsked, m.live
	return m, func() tea.Msg {
		preview, err := live.Preview(project, window)
		return previewed{asked: asked, project: project.Name(), window: window, preview: preview, err: err}
	}
}

// shownPreview is the preview drawn in place of the selected window's
// drawing, if it is the one wanted.
func (m browser) shownPreview() (domain.WindowPreview, bool) {
	project, window, ok := m.previewKey()
	if !ok || !m.hasPreview || m.preview.project != project.Name() || m.preview.window != window {
		return domain.WindowPreview{}, false
	}
	return m.preview.preview, true
}

// roomToDraw reports whether the details have the room to draw the
// selected project's window, rather than list its panes, as frame lays
// them out with a notice of one line: a longer one, as a failure's, mustn't
// stop the preview it tells about. Miniatures don't change it: they are
// drawn only with room to spare.
func (m browser) roomToDraw() bool {
	shown := m.shown()
	if len(shown) == 0 {
		return false
	}
	height := m.height - 1 - 1 - 2 // the notice, the keys and the cards' borders
	width := m.width - m.cardsBeside() - cardSides
	below := height - windowListRow(detailsTop(shown[m.cursor], width)) - 1
	return width >= minDrawingWidth && below-1 >= minDrawingHeight
}

// withPreview is m after msg brought a preview, or why there is none. An
// answer to an earlier ask, or no longer wanted, is dropped. A failure
// falls back to the drawing, saying why once, and the next tick tries
// again. The preview's selected pane follows the drawing's, unless it is
// one the config doesn't have, which stays selected.
func (m browser) withPreview(msg previewed) browser {
	m.previewAnswered = max(m.previewAnswered, msg.asked)
	project, window, ok := m.previewKey()
	if msg.asked != m.previewAsked || !ok || project.Name() != msg.project || window != msg.window {
		return m
	}
	panes := msg.preview.Panes()
	if msg.err == nil && len(panes) == 0 {
		msg.err = fmt.Errorf("%w: %s", domain.ErrEmptyPreview, msg.window)
	}
	if msg.err != nil {
		m.hasPreview = false
		failure := "preview: " + msg.err.Error()
		switch {
		case errors.Is(msg.err, domain.ErrSessionNotRunning):
			failure = fmt.Sprintf("preview: %s's session ended", msg.project)
		case errors.Is(msg.err, domain.ErrWindowNotRunning):
			failure = fmt.Sprintf("preview: %s's window %s closed", msg.project, msg.window)
		}
		if failure != m.previewFailure {
			m.notice, m.previewFailure = failure, failure
		}
		return m
	}
	before, wasShown := m.shownPreview()
	if m.previewFailure != "" && m.notice == m.previewFailure {
		m.notice = "" // the failure is over
	}
	m.preview, m.hasPreview, m.previewFailure = msg, true, ""
	if wasShown && m.previewPane < len(before.Panes()) {
		if _, inConfig := before.Panes()[m.previewPane].InConfig(); !inConfig {
			m.previewPane = min(m.previewPane, len(panes)-1)
			return m
		}
	}
	return m.previewFollowing()
}

// previewFollowing is m with the preview's pane selected that is the
// drawing's selected pane or, when the preview lacks it, as a zoomed
// window does, the preview's first, which the drawing then selects too.
func (m browser) previewFollowing() browser {
	preview, ok := m.shownPreview()
	if !ok {
		return m
	}
	for i, pane := range preview.Panes() {
		if index, inConfig := pane.InConfig(); inConfig && index == m.pane {
			m.previewPane = i
			return m
		}
	}
	return m.previewPaneSelected(0)
}

// previewPaneSelected is m with the preview's pane at index selected, and
// the drawing's too when the config has it.
func (m browser) previewPaneSelected(index int) browser {
	preview, ok := m.shownPreview()
	if !ok || index < 0 || index >= len(preview.Panes()) {
		return m
	}
	m.previewPane = index
	if pane, inConfig := preview.Panes()[index].InConfig(); inConfig {
		m.pane = pane
	}
	return m
}

// switchedView is m after p: the drawing instead of the preview wanted,
// even before it arrives, or the preview instead of the drawing, or of a
// preview failing, saying why when the selected project has none. While
// the config is shown, p does nothing.
func (m browser) switchedView() browser {
	if m.config {
		return m
	}
	if _, _, wanted := m.previewKey(); wanted && m.previewFailure == "" {
		m.drawings = true
		return m
	}
	m.drawings, m.previewFailure = false, ""
	shown := m.shown()
	if len(shown) == 0 {
		return m
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
	return m
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
	if preview, ok := m.shownPreview(); ok && m.previewPane < len(preview.Panes()) {
		return preview.Panes()[m.previewPane].InConfig()
	}
	return m.selectedPane(count), true
}
