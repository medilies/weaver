package infra

import (
	"errors"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
)

// TeaConfirmer asks the user yes-or-no questions in a dialog.
type TeaConfirmer struct{}

// NewTeaConfirmer constructs TeaConfirmer.
func NewTeaConfirmer() TeaConfirmer {
	return TeaConfirmer{}
}

// Confirm implements app.Confirmer, as a dialog in the middle of the
// screen, No chosen until the user picks Yes. Esc answers No: for every
// question asked this way, No leaves things as they are. Ctrl+C aborts.
// Declining to replace a registered project ends the freeze, as Ctrl+C
// would.
func (TeaConfirmer) Confirm(question string) (bool, error) {
	final, err := tea.NewProgram(confirmation{question: question}, tea.WithOutput(os.Stderr)).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return false, app.ErrAborted
	}
	if err != nil {
		return false, err
	}
	return final.(confirmation).yes, nil
}

// confirmation is the dialog: the question, the answer chosen, and the
// terminal's size.
type confirmation struct {
	question   string
	yes        bool
	cols, rows int
}

// confirmingKeys are told below the buttons.
var confirmingKeys = []keyTold{{"←/→", "toggle"}, {"enter", "submit"}, {"y", "Yes"}, {"n", "No"}}

const (
	answerYes, answerNo = "  Yes  ", "  No  "
	answersGap          = "  "
	confirmIndent       = 2 // the question's border and padding, which the buttons line up with
)

func (c confirmation) Init() tea.Cmd {
	return nil
}

func (c confirmation) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.cols, c.rows = msg.Width, msg.Height
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return c.clicked(msg.X, msg.Y)
		}
	case tea.KeyPressMsg:
		return c.press(msg.String())
	}
	return c, nil
}

// press toggles the answer, or ends the dialog with one.
func (c confirmation) press(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "right", "h", "l", "tab", "shift+tab", "←/→":
		c.yes = !c.yes
	case "enter":
		return c, tea.Quit
	case "y", "Y":
		c.yes = true
		return c, tea.Quit
	case "n", "N", "esc":
		c.yes = false
		return c, tea.Quit
	case "ctrl+c":
		return c, tea.Interrupt
	}
	return c, nil
}

// clicked answers with the button clicked at (x, y), or does what the
// key told there does.
func (c confirmation) clicked(x, y int) (tea.Model, tea.Cmd) {
	lines, spots, left, top := c.laidOut()
	row := y - top
	if row < 0 || row >= len(lines) {
		return c, nil
	}
	spot, ok := hotspotAt(spots[row], x-left)
	if !ok {
		return c, nil
	}
	return c.press(spot.key)
}

// laidOut is the dialog's lines, the hotspots on each, and where the
// dialog starts on the screen, centred as a block.
func (c confirmation) laidOut() (lines []string, spots [][]hotspot, left, top int) {
	width := max(c.cols, 1)
	question := lipgloss.NewStyle().Width(max(width-confirmIndent, 1)).Render(c.question)
	for line := range strings.SplitSeq(question, "\n") {
		lines = append(lines, keysStyle.Render("┃ ")+titleStyle.Render(strings.TrimRight(line, " ")))
	}
	lines = append(lines, "")
	yes, no := answerYes, answerNo
	if c.yes {
		yes = selectedStyle.Render(yes)
	} else {
		no = selectedStyle.Render(no)
	}
	from := confirmIndent
	lines = append(lines, strings.Repeat(" ", from)+yes+answersGap+no)
	yesTo := from + ansi.StringWidth(answerYes)
	noFrom := yesTo + len(answersGap)
	answers := []hotspot{{from: from, to: yesTo, key: "y"}, {from: noFrom, to: noFrom + ansi.StringWidth(answerNo), key: "n"}}
	lines = append(lines, "")
	keys, keySpots := toldKeys(confirmingKeys)
	keys, keySpots = cut(keys, keySpots, width)
	lines = append(lines, keys)

	spots = make([][]hotspot, len(lines))
	spots[len(lines)-3], spots[len(lines)-1] = answers, keySpots
	blockWidth := 0
	for _, line := range lines {
		blockWidth = max(blockWidth, ansi.StringWidth(line))
	}
	return lines, spots, max((c.cols-blockWidth)/2, 0), max((c.rows-len(lines))/2, 0)
}

func (c confirmation) View() tea.View {
	view := tea.NewView("") // until the terminal's size is known
	if c.cols > 0 {
		lines, _, left, top := c.laidOut()
		pad := strings.Repeat(" ", left)
		for i := range lines {
			lines[i] = pad + lines[i]
		}
		view.SetContent(strings.Repeat("\n", top) + strings.Join(lines, "\n"))
	}
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
