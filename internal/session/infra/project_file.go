package infra

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/medilies/weaver/internal/session/domain"
)

var (
	// ErrTrailingData is returned when a config holds a second document.
	ErrTrailingData = errors.New("unexpected data after the config object")
	// ErrCommandAndPrefill is returned when a pane both runs a command and
	// leaves one at the prompt.
	ErrCommandAndPrefill = errors.New(`a pane can't have both "command" and "prefill"`)
	// ErrUnknownSplitTarget is returned when "split.of" names no pane of
	// the window.
	ErrUnknownSplitTarget = errors.New("split.of names no pane of this window")
	// ErrUnquotedHome is returned for a root or dir written as a bare ~,
	// which YAML reads as null rather than as the home directory.
	ErrUnquotedHome = errors.New(`a bare ~ is YAML's null: write "~" in quotes for the home directory`)
)

// projectFile is the on-disk shape of a project config, e.g.
// ~/.config/weaver/projects/<name>.yml; docs/config.md documents it.
type projectFile struct {
	Root     string            `yaml:"root"`
	Env      map[string]string `yaml:"env,omitempty"`
	Setup    []string          `yaml:"setup,omitempty"`
	Teardown []string          `yaml:"teardown,omitempty"`
	Before   []string          `yaml:"before,omitempty"`
	Clear    *bool             `yaml:"clear,omitempty"`
	Windows  []windowFile      `yaml:"windows,omitempty"`
}

type windowFile struct {
	Name   string            `yaml:"name"`
	Focus  bool              `yaml:"focus,omitempty"`
	Dir    string            `yaml:"dir,omitempty"`
	Env    map[string]string `yaml:"env,omitempty"`
	Before []string          `yaml:"before,omitempty"`
	Clear  *bool             `yaml:"clear,omitempty"`
	Grid   *gridFile         `yaml:"grid,omitempty"`
	Panes  []paneFile        `yaml:"panes,omitempty"`
}

type gridFile struct {
	Columns []trackText `yaml:"columns,omitempty,flow"`
	Rows    []trackText `yaml:"rows,omitempty,flow"`
	Areas   []areaRow   `yaml:"areas"`
}

// areaRow is a row of a grid's pane names, written on one line so the
// rows read as the grid they draw.
type areaRow []string

func (r areaRow) MarshalYAML() (any, error) {
	row := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, name := range r {
		var node yaml.Node
		if err := node.Encode(name); err != nil {
			return nil, err
		}
		row.Content = append(row.Content, &node)
	}
	return row, nil
}

type paneFile struct {
	Name    string            `yaml:"name,omitempty"`
	Focus   bool              `yaml:"focus,omitempty"`
	Dir     string            `yaml:"dir,omitempty"`
	Split   *splitFile        `yaml:"split,omitempty"`
	Before  []string          `yaml:"before,omitempty"`
	Clear   *bool             `yaml:"clear,omitempty"`
	Command string            `yaml:"command,omitempty"`
	Prefill string            `yaml:"prefill,omitempty"`
	Note    []string          `yaml:"note,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Tasks   []taskFile        `yaml:"tasks,omitempty"`
}

// taskFile is one of a pane's tasks, written on one line:
// { name: migrate, command: php artisan migrate }.
type taskFile struct {
	Name    string `yaml:"name,omitempty"`
	Command string `yaml:"command"`
}

func (t taskFile) MarshalYAML() (any, error) {
	type fields taskFile // without this method, so encoding doesn't recurse
	var node yaml.Node
	if err := node.Encode(fields(t)); err != nil {
		return nil, err
	}
	node.Style = yaml.FlowStyle
	return &node, nil
}

type splitFile struct {
	Of        string `yaml:"of,omitempty"`
	Direction string `yaml:"direction"`
	Size      string `yaml:"size,omitempty"`
}

// schemaHeader points YAML editors (e.g. VS Code's YAML extension) at the
// schema saved next to the configs.
const schemaHeader = "# yaml-language-server: $schema=./" + schemaFileName + "\n"

// fileTypes names the file shapes in YAML's errors as the config does,
// e.g. "field comand not found in type pane".
var fileTypes = strings.NewReplacer(
	"infra.projectFile", "project", "infra.windowFile", "window",
	"infra.paneFile", "pane", "infra.splitFile", "split", "infra.gridFile", "grid", "infra.taskFile", "task",
)

// decodeProject strictly parses a config: unknown fields (usually typos)
// and a second document are errors, not silently ignored.
func decodeProject(name domain.ProjectName, raw []byte) (domain.Project, error) {
	var file projectFile
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	// An empty document leaves the root empty, which toProject reports.
	if err := decoder.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return domain.Project{}, errors.New(fileTypes.Replace(err.Error()))
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return domain.Project{}, ErrTrailingData
	}
	if err := file.bareHome(raw); err != nil {
		return domain.Project{}, err
	}
	if err := nullGrid(raw); err != nil {
		return domain.Project{}, err
	}
	return file.toProject(name)
}

// nullGrid reports a window's grid written empty or as null in raw, which
// would otherwise read as no grid at all.
func nullGrid(raw []byte) error {
	var grids struct {
		Windows []struct {
			Name string    `yaml:"name"`
			Grid yaml.Node `yaml:"grid"`
		} `yaml:"windows"`
	}
	if err := yaml.Unmarshal(raw, &grids); err != nil {
		return nil // decodeProject already read raw strictly
	}
	for _, window := range grids.Windows {
		if window.Grid.Kind == yaml.ScalarNode && window.Grid.Tag == "!!null" {
			return fmt.Errorf("window %q: grid: %w", window.Name, domain.ErrEmptyGrid)
		}
	}
	return nil
}

// homeFile holds the nodes of the paths in a config, which a bare ~ would
// quietly leave empty: the root, not the home directory.
type homeFile struct {
	Root    yaml.Node `yaml:"root"`
	Windows []struct {
		Dir   yaml.Node `yaml:"dir"`
		Panes []struct {
			Dir yaml.Node `yaml:"dir"`
		} `yaml:"panes"`
	} `yaml:"windows"`
}

// bareHome reports a root, window dir or pane dir written as a bare ~ in raw, the
// config f was decoded from.
func (f projectFile) bareHome(raw []byte) error {
	var paths homeFile
	if err := yaml.Unmarshal(raw, &paths); err != nil {
		return nil // decodeProject already read raw strictly
	}
	if isBareHome(paths.Root) {
		return fmt.Errorf("root: %w", ErrUnquotedHome)
	}
	for i, window := range paths.Windows {
		if isBareHome(window.Dir) {
			return fmt.Errorf("window %q: dir: %w", f.Windows[i].Name, ErrUnquotedHome)
		}
		for j, pane := range window.Panes {
			if isBareHome(pane.Dir) {
				w := f.Windows[i]
				return fmt.Errorf("window %q: pane %s: dir: %w", w.Name, w.Panes[j].ref(j), ErrUnquotedHome)
			}
		}
	}
	return nil
}

func isBareHome(node yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null" && node.Value == "~"
}

func (f projectFile) toProject(name domain.ProjectName) (domain.Project, error) {
	root, err := parseRoot(f.Root)
	if err != nil {
		return domain.Project{}, err
	}
	env, err := domain.NewEnv(f.Env)
	if err != nil {
		return domain.Project{}, fmt.Errorf("env: %w", err)
	}
	before, err := toCommands(f.Before)
	if err != nil {
		return domain.Project{}, fmt.Errorf("before: %w", err)
	}
	setup, err := toCommands(f.Setup)
	if err != nil {
		return domain.Project{}, fmt.Errorf("setup: %w", err)
	}
	teardown, err := toCommands(f.Teardown)
	if err != nil {
		return domain.Project{}, fmt.Errorf("teardown: %w", err)
	}
	windows := make([]domain.Window, 0, len(f.Windows))
	for _, w := range f.Windows {
		window, err := w.toWindow()
		if err != nil {
			return domain.Project{}, fmt.Errorf("window %q: %w", w.Name, err)
		}
		windows = append(windows, window)
	}
	return domain.NewProject(name, root).
		WithEnv(env).
		RunningFirst(before...).
		WithClearing(toClearing(f.Clear)).
		SettingUp(setup...).
		TearingDown(teardown...).
		WithWindows(windows...)
}

// toClearing reads a clear setting: unset when it isn't written.
func toClearing(clear *bool) domain.Clearing {
	switch {
	case clear == nil:
		return domain.ClearingUnset
	case *clear:
		return domain.ClearingOn
	}
	return domain.ClearingOff
}

// clearOf is how a clear setting is written: left out when unset.
func clearOf(c domain.Clearing) *bool {
	if !c.IsSet() {
		return nil
	}
	on := c == domain.ClearingOn
	return &on
}

func toCommands(lines []string) ([]domain.Command, error) {
	cmds := make([]domain.Command, 0, len(lines))
	for _, line := range lines {
		cmd, err := domain.NewCommand(line)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}

func (w windowFile) toWindow() (domain.Window, error) {
	name, err := domain.NewWindowName(w.Name)
	if err != nil {
		return domain.Window{}, err
	}
	panes := make([]domain.Pane, 0, len(w.Panes))
	for i, p := range w.Panes {
		pane, err := p.toPane(i, w.Panes)
		if err != nil {
			return domain.Window{}, fmt.Errorf("pane %s: %w", p.ref(i), err)
		}
		panes = append(panes, pane)
	}
	before, err := toCommands(w.Before)
	if err != nil {
		return domain.Window{}, fmt.Errorf("before: %w", err)
	}
	dir, err := toDir(w.Dir)
	if err != nil {
		return domain.Window{}, err
	}
	env, err := domain.NewEnv(w.Env)
	if err != nil {
		return domain.Window{}, fmt.Errorf("env: %w", err)
	}
	var window domain.Window
	if w.Grid != nil {
		grid, err := w.Grid.toGrid()
		if err != nil {
			return domain.Window{}, fmt.Errorf("grid: %w", err)
		}
		window, err = domain.NewGridWindow(name, grid, panes...)
		if err != nil {
			return domain.Window{}, err
		}
	} else if window, err = domain.NewWindow(name, panes...); err != nil {
		return domain.Window{}, err
	}
	window = window.In(dir).WithEnv(env).RunningFirst(before...).WithClearing(toClearing(w.Clear))
	if w.Focus {
		window = window.WithFocus()
	}
	return window, nil
}

// toDir reads a window or pane dir, "~" expanded.
func toDir(raw string) (domain.PaneDir, error) {
	path, err := ExpandHome(raw)
	if err != nil {
		return domain.PaneDir{}, err
	}
	dir, err := domain.NewPaneDir(path)
	if err != nil {
		return domain.PaneDir{}, fmt.Errorf("dir: %w", err)
	}
	return dir, nil
}

func (g gridFile) toGrid() (domain.Grid, error) {
	areas := make([][]domain.PaneName, len(g.Areas))
	for i, row := range g.Areas {
		for _, raw := range row {
			name, err := domain.NewPaneName(raw)
			if err != nil {
				return domain.Grid{}, fmt.Errorf("areas: row %d: %w", i+1, err)
			}
			areas[i] = append(areas[i], name)
		}
	}
	columns, err := toTracks(g.Columns)
	if err != nil {
		return domain.Grid{}, fmt.Errorf("columns: %w", err)
	}
	rows, err := toTracks(g.Rows)
	if err != nil {
		return domain.Grid{}, fmt.Errorf("rows: %w", err)
	}
	return domain.NewGrid(areas, columns, rows)
}

// trackText is a grid column's or row's size as the config writes it: a
// number of cells is written bare, as YAML's integer, the rest as text.
type trackText string

// MarshalYAML writes a size in cells as a number, so it reads back as
// written: columns: [30, 1fr].
func (t trackText) MarshalYAML() (any, error) {
	if n, err := strconv.Atoi(string(t)); err == nil {
		return n, nil
	}
	return string(t), nil
}

func toTracks(raw []trackText) ([]domain.Track, error) {
	tracks := make([]domain.Track, 0, len(raw))
	for _, r := range raw {
		track, err := domain.ParseTrack(string(r))
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}
	return tracks, nil
}

// toPane builds the pane at index of its window; siblings are all the
// window's panes, so "split.of" can be resolved by name.
func (p paneFile) toPane(index int, siblings []paneFile) (domain.Pane, error) {
	pane := domain.NewPane()
	if p.Name != "" {
		name, err := domain.NewPaneName(p.Name)
		if err != nil {
			return domain.Pane{}, err
		}
		pane = pane.Named(name)
	}

	if strings.TrimSpace(p.Dir) != "" { // blank is the window's dir, as no dir is
		dir, err := toDir(p.Dir)
		if err != nil {
			return domain.Pane{}, err
		}
		pane = pane.In(dir)
	}

	if p.Split != nil {
		split, err := p.Split.toSplit(index, siblings)
		if err != nil {
			return domain.Pane{}, fmt.Errorf("split: %w", err)
		}
		pane = pane.Splitting(split)
	}

	before, err := toCommands(p.Before)
	if err != nil {
		return domain.Pane{}, fmt.Errorf("before: %w", err)
	}
	pane = pane.RunningFirst(before...).WithClearing(toClearing(p.Clear))

	if pane, err = p.withStartup(pane); err != nil {
		return domain.Pane{}, err
	}

	if len(p.Note) > 0 {
		pane = pane.WithNote(p.Note...)
	}

	env, err := domain.NewEnv(p.Env)
	if err != nil {
		return domain.Pane{}, fmt.Errorf("env: %w", err)
	}

	tasks := make([]domain.Task, 0, len(p.Tasks))
	for i, t := range p.Tasks {
		task, err := t.toTask()
		if err != nil {
			return domain.Pane{}, fmt.Errorf("tasks: #%d: %w", i+1, err)
		}
		tasks = append(tasks, task)
	}
	pane = pane.WithEnv(env).Offering(tasks...)
	if p.Focus {
		pane = pane.WithFocus()
	}
	return pane, nil
}

func (t taskFile) toTask() (domain.Task, error) {
	cmd, err := domain.NewCommand(t.Command)
	if err != nil {
		return domain.Task{}, fmt.Errorf("command: %w", err)
	}
	return domain.NewTask(t.Name, cmd)
}

func (p paneFile) withStartup(pane domain.Pane) (domain.Pane, error) {
	switch {
	case p.Command != "" && p.Prefill != "":
		return domain.Pane{}, ErrCommandAndPrefill
	case p.Command != "":
		cmd, err := domain.NewCommand(p.Command)
		if err != nil {
			return domain.Pane{}, fmt.Errorf("command: %w", err)
		}
		return pane.Running(cmd), nil
	case p.Prefill != "":
		cmd, err := domain.NewCommand(p.Prefill)
		if err != nil {
			return domain.Pane{}, fmt.Errorf("prefill: %w", err)
		}
		return pane.Prefilling(cmd), nil
	}
	return pane, nil
}

// ref names a pane in errors: by name, else by 1-based position.
func (p paneFile) ref(index int) string {
	if name := strings.TrimSpace(p.Name); name != "" {
		return strconv.Quote(name)
	}
	return "#" + strconv.Itoa(index+1)
}

// toSplit resolves "of" to a pane index, defaulting to the previous pane.
func (s splitFile) toSplit(index int, siblings []paneFile) (domain.Split, error) {
	of := index - 1
	if target := strings.TrimSpace(s.Of); target != "" {
		of = -1
		for i, sibling := range siblings {
			if strings.TrimSpace(sibling.Name) == target {
				of = i
				break
			}
		}
		if of < 0 {
			return domain.Split{}, fmt.Errorf("%w: %q", ErrUnknownSplitTarget, s.Of)
		}
	}
	direction, err := domain.ParseDirection(s.Direction)
	if err != nil {
		return domain.Split{}, err
	}
	size, err := domain.ParseSize(s.Size)
	if err != nil {
		return domain.Split{}, err
	}
	return domain.NewSplit(of, direction, size)
}

// hasComments reports whether a config holds a comment, other than the
// schema header first.
func hasComments(raw []byte) (bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(bytes.TrimPrefix(raw, []byte(schemaHeader)), &doc); err != nil {
		return false, err
	}
	return commented(&doc), nil
}

// commented reports whether node or a node under it carries a comment.
func commented(node *yaml.Node) bool {
	if node.HeadComment != "" || node.LineComment != "" || node.FootComment != "" {
		return true
	}
	return slices.ContainsFunc(node.Content, commented)
}

// encodeProject writes a config in its canonical form: defaults left out,
// focus only off the first window or pane, a grid as written, with its
// window's panes in creation order and no splits, "split.of" only when it
// isn't the previous pane, a pane's dir only when it isn't its window's,
// directories under the root relative to it, the home directory as "~",
// and pointing to the schema saved next to it with a header line.
func encodeProject(project domain.Project) ([]byte, error) {
	root := project.Root()
	file := projectFile{
		Root:     contractHome(root.String()),
		Env:      envMap(project.Env()),
		Setup:    lines(project.Setup()),
		Teardown: lines(project.Teardown()),
		Before:   lines(project.Before()),
		Clear:    clearOf(project.Clearing()),
	}
	if !onlyDefaultWindow(project) {
		for i, window := range project.Windows() {
			file.Windows = append(file.Windows, windowFileOf(root, window))
			file.Windows[i].Focus = i > 0 && i == project.FocusedWindow()
		}
	}

	var doc bytes.Buffer
	doc.WriteString(schemaHeader)
	encoder := yaml.NewEncoder(&doc)
	encoder.SetIndent(2)
	if err := encoder.Encode(file); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return doc.Bytes(), nil
}

func windowFileOf(root domain.ProjectRoot, window domain.Window) windowFile {
	file := windowFile{
		Name:   window.Name().String(),
		Dir:    dirOf(root, window.Dir()),
		Env:    envMap(window.Env()),
		Before: lines(window.Before()),
		Clear:  clearOf(window.Clearing()),
	}
	panes := window.Panes()
	grid, gridded := window.Grid()
	if gridded {
		file.Grid = gridFileOf(grid)
	} else if len(panes) == 1 && isPlain(root, window, panes[0]) {
		return file
	}
	for _, i := range window.Listing() {
		pane := paneFileOf(root, window, i)
		if gridded {
			pane.Split = nil // the grid's
		}
		file.Panes = append(file.Panes, pane)
	}
	return file
}

func gridFileOf(grid domain.Grid) *gridFile {
	file := &gridFile{Columns: trackTexts(grid.Columns()), Rows: trackTexts(grid.Rows())}
	for _, row := range grid.Areas() {
		names := make(areaRow, len(row))
		for i, name := range row {
			names[i] = name.String()
		}
		file.Areas = append(file.Areas, names)
	}
	return file
}

// trackTexts is the tracks' text, or nil for none.
func trackTexts(tracks []domain.Track) []trackText {
	if len(tracks) == 0 {
		return nil
	}
	out := make([]trackText, len(tracks))
	for i, t := range tracks {
		out[i] = trackText(t.String())
	}
	return out
}

// paneFileOf is the file shape of the window's pane at index.
func paneFileOf(root domain.ProjectRoot, window domain.Window, index int) paneFile {
	panes := window.Panes()
	pane := panes[index]
	file := paneFile{
		Name:   pane.Name().String(),
		Dir:    paneDirOf(root, window, pane),
		Before: lines(pane.Before()),
		Clear:  clearOf(pane.Clearing()),
		Note:   pane.Note(),
		Env:    envMap(pane.Env()),
		Focus:  index > 0 && index == window.FocusedPane(),
	}
	for _, task := range pane.Tasks() {
		file.Tasks = append(file.Tasks, taskFile{Name: task.Name(), Command: task.Command().String()})
	}
	if split, ok := pane.Split(); ok {
		file.Split = &splitFile{Direction: split.Direction().String(), Size: split.Size().String()}
		if split.Of() != index-1 {
			file.Split.Of = panes[split.Of()].Name().String()
		}
	}
	if cmd, ok := pane.Startup().Command(); ok {
		if pane.Startup().Submits() {
			file.Command = cmd.String()
		} else {
			file.Prefill = cmd.String()
		}
	}
	return file
}

// dirOf writes a directory under the root relative to it, and the root
// itself as "".
func dirOf(root domain.ProjectRoot, dir domain.PaneDir) string {
	path := root.Relative(dir).String()
	if filepath.IsAbs(path) {
		return contractHome(path)
	}
	return keepRelative(path)
}

// paneDirOf writes the pane's own dir, unless it starts where its window
// would start it anyway. The root is then written ".", as "" would mean
// the window's dir.
func paneDirOf(root domain.ProjectRoot, window domain.Window, pane domain.Pane) string {
	dir, set := pane.Dir()
	if !set || root.Resolve(dir) == root.Resolve(window.Dir()) {
		return ""
	}
	if written := dirOf(root, dir); written != "" {
		return written
	}
	return "."
}

// keepRelative writes a relative path starting with a directory named ~
// as "./~", which ExpandHome would otherwise read as the home directory.
func keepRelative(rel string) string {
	if rel == "~" || strings.HasPrefix(rel, "~/") {
		return "./" + rel
	}
	return rel
}

// isPlain reports whether pane is a bare shell in its window's dir, which
// a window without panes means.
func isPlain(root domain.ProjectRoot, window domain.Window, pane domain.Pane) bool {
	_, splits := pane.Split()
	_, types := pane.Startup().Command()
	return pane.Name().IsZero() && paneDirOf(root, window, pane) == "" && !splits && !types &&
		len(pane.Note()) == 0 && len(pane.Env().Vars()) == 0 && len(pane.Before()) == 0 && !pane.Clearing().IsSet() &&
		len(pane.Tasks()) == 0
}

// onlyDefaultWindow reports whether the project's windows are what it
// opens without any: one window named after it, holding a bare shell in
// the root.
func onlyDefaultWindow(project domain.Project) bool {
	windows := project.Windows()
	if len(windows) != 1 {
		return false
	}
	window := windows[0]
	if window.Name().String() != project.Name().String() || len(window.Before()) > 0 || len(window.Env().Vars()) > 0 || window.Clearing().IsSet() ||
		project.Root().Resolve(window.Dir()) != project.Root().String() {
		return false
	}
	panes := window.Panes()
	return len(panes) == 1 && isPlain(project.Root(), window, panes[0])
}

func envMap(env domain.Env) map[string]string {
	vars := env.Vars()
	if len(vars) == 0 {
		return nil
	}
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		m[v.Name()] = v.Value()
	}
	return m
}

// lines is the commands' text, or nil for none.
func lines(cmds []domain.Command) []string {
	if len(cmds) == 0 {
		return nil
	}
	out := make([]string, len(cmds))
	for i, cmd := range cmds {
		out[i] = cmd.String()
	}
	return out
}
