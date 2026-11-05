package infra

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

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

// Confirm implements app.Confirmer.
func (HuhForms) Confirm(question string) (bool, error) {
	yes := false
	err := show(huh.NewGroup(huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&yes)))
	return yes, err
}

// Fill implements app.ProjectForm: a screen for the project, then, for
// each window, one for its name and one per pane. Windows captured from a
// running session come first, pre-filled.
func (f HuhForms) Fill(draft app.ProjectDraft) (domain.Project, error) {
	versions := versionsByKind(draft.Runtimes)
	answers := projectAnswers{name: draft.Name, root: draft.Root}
	for _, checkout := range draft.Checkouts.List() {
		answers.keep = append(answers.keep, checkout.Dir())
	}
	if err := show(answers.groups(draft.Checkouts, versions)...); err != nil {
		return domain.Project{}, err
	}
	project, err := answers.project(draft.Checkouts)
	if err != nil {
		return domain.Project{}, err
	}

	var windows []domain.Window
	for _, captured := range draft.Windows {
		window, kept, err := askWindow(project, windows, versions, &captured)
		if err != nil {
			return domain.Project{}, err
		}
		if kept {
			windows = append(windows, window)
		}
	}
	for {
		if len(windows) > 0 {
			more, err := f.Confirm("Add another window?")
			if err != nil {
				return domain.Project{}, err
			}
			if !more {
				return project.WithWindows(windows...)
			}
		}
		window, _, err := askWindow(project, windows, versions, nil)
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
	keep       []string // directories of the found repositories to keep
	more       string   // "dir = remote" lines
	php, node  string   // "" for none
	env        string   // "NAME=value" lines
}

func (a *projectAnswers) groups(found domain.Checkouts, versions map[domain.RuntimeKind][]string) []*huh.Group {
	groups := []*huh.Group{huh.NewGroup(
		huh.NewInput().Title("Project name").
			Description("Used as `weaver <name>` and as the tmux session name.").
			Value(&a.name).Validate(func(s string) error { _, err := domain.NewProjectName(s); return err }),
		huh.NewInput().Title("Root").
			Description("The project's directory.").
			Value(&a.root).Validate(func(s string) error { _, err := domain.NewProjectRoot(s); return err }),
	).Title("New project")}

	repositories := []huh.Field{huh.NewText().Title("Other repositories").
		Description("Cloned when missing. One per line: directory = remote, e.g. back = git@host:back.git").
		Value(&a.more).Validate(func(s string) error { _, err := parseCheckouts(s); return err })}
	if list := found.List(); len(list) > 0 {
		options := make([]huh.Option[string], 0, len(list))
		for _, checkout := range list {
			options = append(options, huh.NewOption(checkout.Dir()+"  "+checkout.Remote(), checkout.Dir()))
		}
		repositories = append([]huh.Field{huh.NewMultiSelect[string]().Title("Repositories found").
			Description("Cloned when missing, e.g. on a new machine.").
			// Without a height, huh takes the title and description out of
			// the options' rows and shows none of a short list.
			Options(options...).Height(len(options) + 2).Value(&a.keep)}, repositories...)
	}
	groups = append(groups, huh.NewGroup(repositories...).Title("Repositories"))

	every := versionFields("for every pane", "None", versions, &a.php, &a.node)
	every = append(every, envField("Environment for every pane", &a.env))
	return append(groups, huh.NewGroup(every...).Title("Every pane"))
}

func (a projectAnswers) project(found domain.Checkouts) (domain.Project, error) {
	name, err := domain.NewProjectName(a.name)
	if err != nil {
		return domain.Project{}, err
	}
	root, err := domain.NewProjectRoot(a.root)
	if err != nil {
		return domain.Project{}, err
	}
	checkouts, err := parseCheckouts(a.more)
	if err != nil {
		return domain.Project{}, err
	}
	for _, checkout := range found.List() {
		if slices.Contains(a.keep, checkout.Dir()) {
			checkouts = append(checkouts, checkout)
		}
	}
	kept, err := domain.NewCheckouts(checkouts...)
	if err != nil {
		return domain.Project{}, err
	}
	env, err := parseEnv(a.env)
	if err != nil {
		return domain.Project{}, err
	}
	runtimes, err := pins(a.php, a.node)
	if err != nil {
		return domain.Project{}, err
	}
	return domain.NewProject(name, root).WithCheckouts(kept).WithEnv(env).WithRuntimes(runtimes), nil
}

// askWindow asks for a window's name, then for its panes one by one. A
// captured window starts pre-filled, may be dropped (it reports whether it
// was kept), and keeps its panes' layout.
func askWindow(
	project domain.Project,
	earlier []domain.Window,
	versions map[domain.RuntimeKind][]string,
	captured *domain.Window,
) (domain.Window, bool, error) {
	raw, keep := "", true
	switch {
	case captured != nil:
		raw = captured.Name().String()
	case len(earlier) == 0:
		raw = project.Name().String()
	}
	fields := []huh.Field{huh.NewInput().Title("Window name").Value(&raw).Validate(func(s string) error {
		name, err := domain.NewWindowName(s)
		if err != nil {
			return err
		}
		for _, w := range earlier {
			if w.Name() == name {
				return domain.ErrDuplicateWindowName
			}
		}
		return nil
	})}
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
			if err := show(answers.groups(name, panes, versions)...); err != nil {
				return domain.Window{}, false, err
			}
			panes = append(panes, answers)
		}
	}
	for captured == nil {
		pane := &paneAnswers{of: len(panes) - 1, direction: "right", run: true}
		if err := show(pane.groups(name, panes, versions)...); err != nil {
			return domain.Window{}, false, err
		}
		panes = append(panes, pane)
		if !pane.another {
			break
		}
	}
	built := make([]domain.Pane, 0, len(panes))
	for i, pane := range panes {
		if pane.name == "" && len(panes) > 1 {
			pane.name = "pane " + strconv.Itoa(i+1) // borders need a label
		}
		p, err := pane.pane(i)
		if err != nil {
			return domain.Window{}, false, fmt.Errorf("pane %d: %w", i+1, err)
		}
		built = append(built, p)
	}
	window, err := domain.NewWindow(name, built...)
	return window, err == nil, err
}

// capturedAnswers pre-fills a pane screen from a captured pane, with its
// directory relative to the root when under it.
func capturedAnswers(root domain.ProjectRoot, pane domain.Pane) *paneAnswers {
	answers := &paneAnswers{name: pane.Name().String(), dir: pane.Dir().String(), direction: "right", run: true, fixed: true}
	if root.Contains(answers.dir) {
		answers.dir, _ = filepath.Rel(root.String(), answers.dir)
	}
	if split, ok := pane.Split(); ok {
		answers.of, answers.direction, answers.size = split.Of(), split.Direction().String(), split.Size().String()
	}
	return answers
}

// paneAnswers are a pane screen's fields.
type paneAnswers struct {
	name, dir       string
	of              int // index of the pane split
	direction, size string
	command         string
	run             bool // run the command, rather than only type it
	note            string
	php, node       string // "" for the project's
	env             string
	another         bool // add another pane to the window
	fixed           bool // captured: the window's panes are known
}

func (a *paneAnswers) groups(window domain.WindowName, earlier []*paneAnswers, versions map[domain.RuntimeKind][]string) []*huh.Group {
	index := len(earlier)
	layout := []huh.Field{
		huh.NewInput().Title("Pane name").
			Description("Shown in the pane's top border; a window with several panes needs one per pane.").
			Value(&a.name).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return nil
			}
			name, err := domain.NewPaneName(s)
			if err != nil {
				return err
			}
			for _, e := range earlier {
				if strings.TrimSpace(e.name) == name.String() {
					return domain.ErrDuplicatePaneName
				}
			}
			return nil
		}),
		huh.NewInput().Title("Directory").
			Description("Relative to the project root; empty for the root itself.").
			Value(&a.dir).Validate(func(s string) error { _, err := domain.NewPaneDir(s); return err }),
	}
	if index > 0 {
		targets := make([]huh.Option[int], 0, index)
		for i, e := range earlier {
			label := e.name
			if label == "" {
				label = "pane " + strconv.Itoa(i+1)
			}
			targets = append(targets, huh.NewOption(label, i))
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
	layout = append(layout, huh.NewInput().Title("Command").
		Description("Typed into the shell when the pane starts. Empty for a plain shell.").
		Value(&a.command).Validate(func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		_, err := domain.NewCommand(s)
		return err
	}))

	running := huh.NewGroup(huh.NewConfirm().Title("Run the command when the pane starts?").
		Affirmative("Run it").Negative("Only type it").Value(&a.run)).
		WithHideFunc(func() bool { return strings.TrimSpace(a.command) == "" })

	extras := []huh.Field{huh.NewText().Title("Note").
		Description("Lines printed in the pane before its shell starts.").Value(&a.note)}
	extras = append(extras, versionFields("for this pane", "Same as the project", versions, &a.php, &a.node)...)
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

func (a *paneAnswers) pane(index int) (domain.Pane, error) {
	pane := domain.NewPane()
	if strings.TrimSpace(a.name) != "" {
		name, err := domain.NewPaneName(a.name)
		if err != nil {
			return domain.Pane{}, err
		}
		pane = pane.Named(name)
	}
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
	runtimes, err := pins(a.php, a.node)
	if err != nil {
		return domain.Pane{}, err
	}
	return pane.WithEnv(env).WithRuntimes(runtimes), nil
}

// versionFields offers the installed PHP and Node versions, if any, with
// none as the first choice.
func versionFields(scope, none string, versions map[domain.RuntimeKind][]string, php, node *string) []huh.Field {
	var fields []huh.Field
	for _, choice := range []struct {
		kind  domain.RuntimeKind
		title string
		value *string
	}{{domain.PHP, "PHP " + scope, php}, {domain.Node, "Node.js " + scope, node}} {
		if len(versions[choice.kind]) == 0 {
			continue
		}
		options := []huh.Option[string]{huh.NewOption(none, "")}
		for _, version := range versions[choice.kind] {
			options = append(options, huh.NewOption(version, version))
		}
		fields = append(fields, huh.NewSelect[string]().Title(choice.title).Options(options...).Value(choice.value))
	}
	return fields
}

func envField(title string, value *string) huh.Field {
	return huh.NewText().Title(title).Description("One per line: NAME=value, set literally.").
		Value(value).Validate(func(s string) error { _, err := parseEnv(s); return err })
}

func versionsByKind(runtimes []domain.Runtime) map[domain.RuntimeKind][]string {
	versions := make(map[domain.RuntimeKind][]string)
	for _, runtime := range runtimes {
		versions[runtime.Kind()] = append(versions[runtime.Kind()], runtime.Version())
	}
	return versions
}

// pins turns chosen versions ("" for none) into Runtimes.
func pins(php, node string) (domain.Runtimes, error) {
	var runtimes []domain.Runtime
	for kind, version := range map[domain.RuntimeKind]string{domain.PHP: php, domain.Node: node} {
		if version == "" {
			continue
		}
		runtime, err := domain.NewRuntime(string(kind), version)
		if err != nil {
			return domain.Runtimes{}, err
		}
		runtimes = append(runtimes, runtime)
	}
	return domain.NewRuntimes(runtimes...)
}

// parseEnv reads "NAME=value" lines, skipping blank ones.
func parseEnv(text string) (domain.Env, error) {
	vars := make(map[string]string)
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return domain.Env{}, fmt.Errorf("%q: want NAME=value", line)
		}
		vars[strings.TrimSpace(name)] = value
	}
	return domain.NewEnv(vars)
}

// parseCheckouts reads "dir = remote" lines, skipping blank ones.
func parseCheckouts(text string) ([]domain.Checkout, error) {
	var checkouts []domain.Checkout
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dir, remote, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%q: want directory = remote", line)
		}
		checkout, err := domain.NewCheckout(strings.TrimSpace(dir), strings.TrimSpace(remote))
		if err != nil {
			return nil, err
		}
		checkouts = append(checkouts, checkout)
	}
	return checkouts, nil
}
