package infra

import (
	"errors"
	"os"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/medilies/weaver/internal/session/app"
)

// HuhConfirmer asks the user yes-or-no questions through huh's forms.
type HuhConfirmer struct{}

// NewHuhConfirmer constructs HuhConfirmer.
func NewHuhConfirmer() HuhConfirmer {
	return HuhConfirmer{}
}

// Confirm implements app.Confirmer, as a dialog in the middle of the
// screen. Esc answers No: for every question asked this way, No leaves
// things as they are. Declining to replace a registered project ends the
// freeze, as Ctrl+C would.
func (HuhConfirmer) Confirm(question string) (bool, error) {
	yes := false
	keys := huh.NewDefaultKeyMap()
	keys.Confirm.Reject = key.NewBinding(key.WithKeys("n", "N", "esc"), key.WithHelp("n", "No"))
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&yes))).
		WithKeyMap(keys).WithTheme(huh.ThemeFunc(dialogTheme))
	form.SubmitCmd, form.CancelCmd = tea.Quit, tea.Interrupt
	help := form.Help().ShortHelpView(form.KeyBinds())
	width := max(lipgloss.Width(question)+confirmFrameWidth, lipgloss.Width(help))
	_, err := tea.NewProgram(dialog{form: form, width: width}, tea.WithOutput(os.Stderr)).Run() // where huh draws
	if errors.Is(err, tea.ErrInterrupted) || form.State == huh.StateAborted {
		return false, app.ErrAborted
	}
	return yes, err
}

// dialogTheme is huh's base theme in weaver's styles: a bold question,
// the chosen answer a cyan block, as a list's selected item, and the keys
// faint.
func dialogTheme(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)
	t.Focused.Title = titleStyle
	t.Focused.FocusedButton = t.Focused.FocusedButton.
		Foreground(selectedStyle.GetForeground()).Background(selectedStyle.GetBackground())
	t.Focused.BlurredButton = t.Focused.BlurredButton.UnsetForeground().UnsetBackground()
	t.Help.ShortKey, t.Help.ShortDesc, t.Help.ShortSeparator = keysStyle, keysStyle, keysStyle
	return t
}

const confirmFrameWidth = 2 // the field's left border and padding

// dialog shows a form full screen, centred, as wide as its content or the
// screen, whichever is narrower.
type dialog struct {
	form       *huh.Form
	width      int
	cols, rows int
}

func (d dialog) Init() tea.Cmd {
	return d.form.Init()
}

func (d dialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		d.cols, d.rows = size.Width, size.Height
		d.form = d.form.WithWidth(min(d.width, size.Width))
	}
	_, cmd := d.form.Update(msg)
	return d, cmd
}

func (d dialog) View() tea.View {
	view := tea.NewView("") // until the terminal's size is known
	if d.cols > 0 {
		form := d.form.View()
		form = lipgloss.NewStyle().Width(lipgloss.Width(form)).Render(form) // centred as a block, not line by line
		view.SetContent(lipgloss.Place(d.cols, d.rows, lipgloss.Center, lipgloss.Center, form))
	}
	view.AltScreen = true
	return view
}
