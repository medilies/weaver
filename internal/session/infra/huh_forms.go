package infra

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// HuhForms asks the user through full-screen terminal forms. Fields are
// checked with the domain's own constructors, so mistakes show as they
// are typed.
type HuhForms struct{}

// NewHuhForms constructs HuhForms.
func NewHuhForms() HuhForms {
	return HuhForms{}
}

// Confirm implements app.Confirmer, as a dialog in the middle of the
// screen. Esc answers No: for every question asked this way, No leaves
// things as they are. Declining to replace a registered project ends the
// freeze, as Ctrl+C would.
func (HuhForms) Confirm(question string) (bool, error) {
	yes := false
	keys := huh.NewDefaultKeyMap()
	keys.Confirm.Reject = key.NewBinding(key.WithKeys("n", "N", "esc"), key.WithHelp("n", "No"))
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&yes))).
		WithKeyMap(keys)
	form.SubmitCmd, form.CancelCmd = tea.Quit, tea.Interrupt
	help := form.Help().ShortHelpView(form.KeyBinds())
	width := max(lipgloss.Width(question)+confirmFrameWidth, lipgloss.Width(help))
	_, err := tea.NewProgram(dialog{form: form, width: width}, tea.WithOutput(os.Stderr)).Run() // where huh draws
	if errors.Is(err, tea.ErrInterrupted) || form.State == huh.StateAborted {
		return false, app.ErrAborted
	}
	return yes, err
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

// Fill implements app.ProjectForm: a screen for the project, then, for
// each window, one for its name and one per pane. Windows captured from a
// running session come first, pre-filled.
func (HuhForms) Fill(draft app.ProjectDraft) (domain.Project, error) {
	answers := projectAnswers{name: draft.Name, root: draft.Root}
	if err := show(answers.groups()...); err != nil {
		return domain.Project{}, err
	}
	project, err := answers.project()
	if err != nil {
		return domain.Project{}, err
	}

	var windows []domain.Window
	for _, captured := range draft.Windows {
		window, kept, err := askWindow(project, windows, &captured)
		if err != nil {
			return domain.Project{}, err
		}
		if kept {
			windows = append(windows, window)
		}
	}
	for {
		if len(windows) > 0 {
			more := false
			// Full screen, like the steps around it, and not Confirm: No
			// saves the project here.
			if err := show(huh.NewGroup(huh.NewConfirm().Title("Add another window?").
				Affirmative("Yes").Negative("No").Value(&more))); err != nil {
				return domain.Project{}, err
			}
			if !more {
				return project.WithWindows(windows...)
			}
		}
		window, _, err := askWindow(project, windows, nil)
		if err != nil {
			return domain.Project{}, err
		}
		windows = append(windows, window)
	}
}

// show runs groups as one full-screen form.
func show(groups ...*huh.Group) error {
	err := huh.NewForm(groups...).WithViewHook(fullScreen).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return app.ErrAborted
	}
	return err
}

func fullScreen(view tea.View) tea.View {
	view.AltScreen = true
	return view
}

// projectAnswers are the project screen's fields.
type projectAnswers struct {
	name, root string
	before     string // command lines
	env        string // "NAME=value" lines
}

func (a *projectAnswers) groups() []*huh.Group {
	return []*huh.Group{
		huh.NewGroup(
			huh.NewInput().Title("Project name").
				Description("Used as `weave <name>` and as the tmux session name.").
				Value(&a.name).Validate(func(s string) error { _, err := domain.NewProjectName(s); return err }),
			huh.NewInput().Title("Root").
				Description("The project's directory: absolute, or starting with ~/.").
				Value(&a.root).Validate(func(s string) error { _, err := parseRoot(s); return err }),
		).Title("New project"),
		huh.NewGroup(
			beforeField("Before commands for every pane", &a.before),
			envField("Environment for every pane", &a.env),
		).Title("Every pane"),
	}
}

func (a projectAnswers) project() (domain.Project, error) {
	name, err := domain.NewProjectName(a.name)
	if err != nil {
		return domain.Project{}, err
	}
	root, err := parseRoot(a.root)
	if err != nil {
		return domain.Project{}, err
	}
	env, err := parseEnv(a.env)
	if err != nil {
		return domain.Project{}, err
	}
	before, err := parseCommands(a.before)
	if err != nil {
		return domain.Project{}, err
	}
	return domain.NewProject(name, root).WithEnv(env).RunningFirst(before...), nil
}

// askWindow asks for a window's name, then for its panes one by one. A
// captured window starts pre-filled, may be dropped (it reports whether it
// was kept), and keeps its panes' layout.
func askWindow(
	project domain.Project,
	earlier []domain.Window,
	captured *domain.Window,
) (domain.Window, bool, error) {
	raw, before, keep := "", "", true
	switch {
	case captured != nil:
		raw = captured.Name().String()
	case len(earlier) == 0:
		raw = project.Windows()[0].Name().String() // the window it opens until given others
	}
	fields := []huh.Field{huh.NewInput().Title("Window name").Value(&raw).Validate(func(s string) error {
		name, err := domain.NewWindowName(s)
		if err != nil {
			return err
		}
		window, err := domain.NewWindow(name)
		if err != nil {
			return err
		}
		_, err = project.WithWindows(append(slices.Clone(earlier), window)...)
		return err
	})}
	fields = append(fields, beforeField("Before commands for every pane of this window", &before))
	if captured != nil {
		fields = append(fields, huh.NewConfirm().Title("Keep this window?").
			Description(fmt.Sprintf("It has %d pane(s) in the running session.", len(captured.Panes()))).
			Affirmative("Keep it").Negative("Drop it").Value(&keep))
	}
	if err := show(huh.NewGroup(fields...).Title(fmt.Sprintf("Window %d", len(earlier)+1))); err != nil {
		return domain.Window{}, false, err
	}
	if !keep {
		return domain.Window{}, false, nil
	}
	name, err := domain.NewWindowName(raw)
	if err != nil {
		return domain.Window{}, false, err
	}

	var panes []*paneAnswers
	if captured != nil {
		for _, pane := range captured.Panes() {
			answers := capturedAnswers(project.Root(), pane)
			if err := show(answers.groups(name, panes)...); err != nil {
				return domain.Window{}, false, err
			}
			panes = append(panes, answers)
		}
	}
	for captured == nil {
		pane := &paneAnswers{of: len(panes) - 1, direction: "right", run: true}
		if err := show(pane.groups(name, panes)...); err != nil {
			return domain.Window{}, false, err
		}
		panes = append(panes, pane)
		if !pane.another {
			break
		}
	}
	names, err := domain.NamePanes(paneNames(panes)...)
	if err != nil {
		return domain.Window{}, false, err
	}
	built := make([]domain.Pane, 0, len(panes))
	for i, pane := range panes {
		p, err := pane.pane(i, names[i])
		if err != nil {
			return domain.Window{}, false, fmt.Errorf("pane %d: %w", i+1, err)
		}
		built = append(built, p)
	}
	cmds, err := parseCommands(before)
	if err != nil {
		return domain.Window{}, false, err
	}
	window, err := domain.NewWindow(name, built...)
	if err != nil {
		return domain.Window{}, false, err
	}
	return window.RunningFirst(cmds...), true, nil
}

// capturedAnswers pre-fills a pane screen from a captured pane, with its
// directory relative to the root when under it and the command it runs.
func capturedAnswers(root domain.ProjectRoot, pane domain.Pane) *paneAnswers {
	answers := &paneAnswers{name: pane.Name().String(), direction: "right", run: true, fixed: true}
	if dir, ok := pane.Dir(); ok {
		answers.dir = root.Relative(dir).String()
	}
	if split, ok := pane.Split(); ok {
		answers.of, answers.direction, answers.size = split.Of(), split.Direction().String(), split.Size().String()
	}
	if cmd, ok := pane.Startup().Command(); ok {
		answers.command, answers.run = cmd.String(), pane.Startup().Submits()
	}
	return answers
}

// paneAnswers are a pane screen's fields.
type paneAnswers struct {
	name, dir       string
	of              int // index of the pane split
	direction, size string
	before          string // command lines
	command         string
	run             bool // run the command, rather than prefill it
	note            string
	env             string
	another         bool // add another pane to the window
	fixed           bool // captured: the window's panes are known
}

func (a *paneAnswers) groups(window domain.WindowName, earlier []*paneAnswers) []*huh.Group {
	index := len(earlier)
	layout := []huh.Field{
		huh.NewInput().Title("Pane name").
			Description("Shown in the pane's top border; a window with several panes needs one per pane.").
			Value(&a.name).Validate(func(s string) error {
			name, err := optionalPaneName(s)
			if err != nil {
				return err
			}
			_, err = domain.NamePanes(append(paneNames(earlier), name)...)
			return err
		}),
		huh.NewInput().Title("Directory").
			Description("Relative to the project root; empty for the root itself.").
			Value(&a.dir).Validate(func(s string) error { _, err := domain.NewPaneDir(s); return err }),
	}
	if index > 0 {
		targets := make([]huh.Option[int], 0, index)
		for i, e := range earlier {
			targets = append(targets, huh.NewOption(e.nameAt(i), i))
		}
		layout = append(layout,
			huh.NewSelect[int]().Title("Split").Description("The pane this one is carved out of.").
				Options(targets...).Value(&a.of),
			huh.NewSelect[string]().Title("Direction").
				Options(huh.NewOption("Right of it", "right"), huh.NewOption("Below it", "below")).Value(&a.direction),
			huh.NewInput().Title("Size").Description("A share of the split pane: 30% or 20 cells. Empty for half.").
				Value(&a.size).Validate(func(s string) error { _, err := domain.ParseSize(s); return err }),
		)
	}
	layout = append(layout, beforeField("Before commands", &a.before), huh.NewInput().Title("Command").
		Description("Typed into the shell when the pane starts. Empty for a plain shell.").
		Value(&a.command).Validate(func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		_, err := domain.NewCommand(s)
		return err
	}))

	running := huh.NewGroup(huh.NewConfirm().Title("Run the command when the pane starts?").
		Affirmative("Run it").Negative("Prefill it").Value(&a.run)).
		WithHideFunc(func() bool { return strings.TrimSpace(a.command) == "" })

	extras := []huh.Field{huh.NewText().Title("Note").
		Description("Lines printed in the pane before its shell starts.").Value(&a.note)}
	extras = append(extras, envField("Environment for this pane", &a.env))
	if !a.fixed {
		extras = append(extras, huh.NewConfirm().Title(fmt.Sprintf("Add another pane to %s?", window)).
			Affirmative("Yes").Negative("No").Value(&a.another))
	}

	title := fmt.Sprintf("%s, pane %d", window, index+1)
	return []*huh.Group{
		huh.NewGroup(layout...).Title(title),
		running.Title(title),
		huh.NewGroup(extras...).Title(title),
	}
}

// nameAt is the name of the pane at index among the panes a new one may
// split, as they are named once there are several.
func (a *paneAnswers) nameAt(index int) string {
	if name := strings.TrimSpace(a.name); name != "" {
		return name
	}
	return domain.DefaultPaneName(index).String()
}

// optionalPaneName reads a pane name, blank for none.
func optionalPaneName(raw string) (domain.PaneName, error) {
	if strings.TrimSpace(raw) == "" {
		return domain.PaneName{}, nil
	}
	return domain.NewPaneName(raw)
}

// paneNames are the names given to the panes' screens, the zero
// PaneName for each left blank. Each was checked as it was typed.
func paneNames(panes []*paneAnswers) []domain.PaneName {
	names := make([]domain.PaneName, len(panes))
	for i, pane := range panes {
		names[i], _ = optionalPaneName(pane.name)
	}
	return names
}

// pane is the pane at index, named as its window names it.
func (a *paneAnswers) pane(index int, name domain.PaneName) (domain.Pane, error) {
	pane := domain.NewPane().Named(name)
	dir, err := domain.NewPaneDir(a.dir)
	if err != nil {
		return domain.Pane{}, err
	}
	pane = pane.In(dir)
	if index > 0 {
		direction, err := domain.ParseDirection(a.direction)
		if err != nil {
			return domain.Pane{}, err
		}
		size, err := domain.ParseSize(a.size)
		if err != nil {
			return domain.Pane{}, err
		}
		split, err := domain.NewSplit(a.of, direction, size)
		if err != nil {
			return domain.Pane{}, err
		}
		pane = pane.Splitting(split)
	}
	before, err := parseCommands(a.before)
	if err != nil {
		return domain.Pane{}, err
	}
	pane = pane.RunningFirst(before...)
	if strings.TrimSpace(a.command) != "" {
		cmd, err := domain.NewCommand(a.command)
		if err != nil {
			return domain.Pane{}, err
		}
		if a.run {
			pane = pane.Running(cmd)
		} else {
			pane = pane.Prefilling(cmd)
		}
	}
	if note := strings.TrimRight(a.note, "\n"); note != "" {
		pane = pane.WithNote(strings.Split(note, "\n")...)
	}
	env, err := parseEnv(a.env)
	if err != nil {
		return domain.Pane{}, err
	}
	return pane.WithEnv(env), nil
}

// beforeField asks for commands to run, in order, before a pane's own.
func beforeField(title string, value *string) huh.Field {
	return huh.NewText().Title(title).
		Description("One command per line, e.g. nvm use 20, typed and run before the pane's command.").
		Value(value).Validate(func(s string) error { _, err := parseCommands(s); return err })
}

func envField(title string, value *string) huh.Field {
	return huh.NewText().Title(title).Description("One per line: NAME=value, set literally.").
		Value(value).Validate(func(s string) error { _, err := parseEnv(s); return err })
}
