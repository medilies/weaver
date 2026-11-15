package infra

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// notesConfig is a grid window with a column in cells, beside a second
// window.
const notesConfig = `windows:
  - name: code
    grid:
      columns: [30, 1fr]
      areas:
        - [files, edit]
    panes:
      - name: files
        tasks: [{command: ls}]
      - name: edit
        tasks: [{command: echo saved}]
  - name: shell
`

// shopConfig has a focused window, and a focused pane in it.
const shopConfig = `env: { APP_ENV: local, LOG: info }
windows:
  - name: shell
  - name: api
    focus: true
    env: { APP_ENV: staging }
    panes:
      - name: logs
        command: sleep 3
      - name: back
        focus: true
        env: { APP_ENV: dev }
        split: { direction: right, size: 60% }
`

// formDriver drives the config form as a user does, by keys, and reads
// it as they do, on its screen.
type formDriver struct {
	t    *testing.T
	form configForm
	quit bool
}

// editing is the config form editing the project name, whose config is
// windows rooted at an existing directory, at 160x45, the screen the
// terminal tests show it on.
func editing(t *testing.T, name, windows string) *formDriver {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	projectName, err := domain.NewProjectName(name)
	if err != nil {
		t.Fatal(err)
	}
	config := []byte("root: " + root + "\n" + windows)
	project, err := decodeProject(projectName, config)
	if err != nil {
		t.Fatalf("decoding %q: %v", config, err)
	}
	form, err := NewTeaConfigForm(NewLocalWorkspace(), nil, DefaultSettings()).form(app.ProjectDraft{
		Name: name, NameStays: true, Root: root, Env: project.Env(), Before: project.Before(),
		Clearing: project.Clearing(), Setup: project.Setup(), Teardown: project.Teardown(),
		Windows: project.Windows(), Config: config,
	})
	if err != nil {
		t.Fatal(err)
	}
	d := &formDriver{t: t, form: form}
	d.send(tea.WindowSizeMsg{Width: 160, Height: 45})
	return d
}

func (d *formDriver) send(msg tea.Msg) {
	d.t.Helper()
	if d.quit {
		d.t.Fatalf("%v sent after the form ended", msg)
	}
	next, cmd := d.form.Update(msg)
	d.form = next.(configForm)
	d.quit = cmd != nil && reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

// keys presses keys, named as tea.KeyPressMsg.String names them.
func (d *formDriver) keys(names ...string) {
	d.t.Helper()
	special := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "up": tea.KeyUp, "down": tea.KeyDown,
		"left": tea.KeyLeft, "right": tea.KeyRight,
	}
	for _, name := range names {
		var key tea.KeyPressMsg
		if code, ok := special[name]; ok {
			key = tea.KeyPressMsg{Code: code}
		} else if letter, ok := strings.CutPrefix(name, "ctrl+"); ok {
			key = tea.KeyPressMsg{Code: rune(letter[0]), Mod: tea.ModCtrl}
		} else {
			key = tea.KeyPressMsg{Code: rune(name[0]), Text: name}
		}
		if key.String() != name {
			d.t.Fatalf("key %q reads as %q", name, key.String())
		}
		d.send(key)
	}
}

// types types text, a key per character.
func (d *formDriver) types(text string) {
	d.t.Helper()
	for _, r := range text {
		d.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// screen is the form as the terminal shows it, without its styles.
func (d *formDriver) screen() string {
	return ansi.Strip(d.form.View().Content)
}

// fatal fails the test with the screen.
func (d *formDriver) fatal(format string, args ...any) {
	d.t.Helper()
	d.t.Fatalf(format+"\n%s", append(args, d.screen())...)
}

// sees fails the test unless the screen shows text.
func (d *formDriver) sees(text string) {
	d.t.Helper()
	if !strings.Contains(d.screen(), text) {
		d.fatal("the screen doesn't show %q", text)
	}
}

// doesntSee fails the test if the screen shows text.
func (d *formDriver) doesntSee(text string) {
	d.t.Helper()
	if strings.Contains(d.screen(), text) {
		d.fatal("the screen shows %q", text)
	}
}

// cursorTo moves the cursor down to the first row matching pattern.
func (d *formDriver) cursorTo(pattern string) {
	d.t.Helper()
	row := regexp.MustCompile(`› +` + pattern)
	for range 60 {
		if row.MatchString(d.screen()) {
			return
		}
		d.keys("down")
	}
	d.fatal("the cursor never reached %q", pattern)
}

// saved is the config the form ended saving.
func (d *formDriver) saved() string {
	d.t.Helper()
	if !d.quit || !d.form.saved {
		d.fatal("the form didn't save")
	}
	config, err := encodeProject(d.form.project)
	if err != nil {
		d.t.Fatal(err)
	}
	return string(config)
}

// splitAt is the column the drawing's second pane starts at, its prompt
// on the same line as the first's; 0 when no line has two prompts.
func (d *formDriver) splitAt() int {
	for _, line := range strings.Split(d.screen(), "\n") {
		if strings.Count(line, "$ █") >= 2 {
			return len([]rune(line[:strings.LastIndex(line, "$ █")]))
		}
	}
	return 0
}

func TestFocusingWindowUnfocusesTheOther(t *testing.T) {
	d := editing(t, "shop", shopConfig)
	d.cursorTo("focus")

	d.keys("enter", "ctrl+s")

	d.sees("Save these changes to the config?")
	removed, kept := regexp.MustCompile(`(?m)^ - +focus: true`), regexp.MustCompile(`(?m)^   +focus: true`)
	if screen := d.screen(); len(removed.FindAllString(screen, -1)) != 1 || len(kept.FindAllString(screen, -1)) != 1 {
		d.fatal("the diff doesn't remove api's focus and keep back's")
	}
	d.keys("y")
	config := d.saved()
	if n := strings.Count(config, "focus: true"); n != 1 {
		t.Fatalf("the config has %d focus: true, not 1:\n%s", n, config)
	}
	if focused := regexp.MustCompile(`- name: (\w+)\s+focus: true`).FindStringSubmatch(config); focused == nil || focused[1] != "back" {
		t.Fatalf("the focused pane isn't back:\n%s", config)
	}
}

func TestTypedAreasAddPaneAsTheyAreTyped(t *testing.T) {
	d := editing(t, "notes", notesConfig)
	d.cursorTo("areas")

	d.keys("enter", "ctrl+u")
	d.types("files, edit / files, logs")

	d.sees("─ logs ─")
	d.keys("enter", "ctrl+s")
	d.sees("- [files, logs]")
	d.sees("- name: logs")
	d.keys("y")
	config := d.saved()
	if !strings.Contains(config, "columns: [30, 1fr]") {
		t.Fatalf("the columns changed:\n%s", config)
	}
	names := regexp.MustCompile(`name: (files|edit|logs)\b`).FindAllStringSubmatch(config, -1)
	if len(names) != 3 || names[0][1] != "files" || names[1][1] != "edit" || names[2][1] != "logs" {
		t.Fatalf("the panes aren't files, edit, logs:\n%s", config)
	}
}

func TestFieldTypedIsKeptOnlyWhenValid(t *testing.T) {
	d := editing(t, "notes", notesConfig)
	d.cursorTo("columns")

	d.keys("enter", "ctrl+u")
	d.types("30 1fr 1fr")
	d.keys("enter")

	d.sees("✗")
	d.keys("esc")
	d.sees("columns   30 1fr ")
	d.doesntSee("● unsaved")
}

func TestGridTurnedIntoSplitsKeepsItsSizes(t *testing.T) {
	d := editing(t, "notes", notesConfig)
	d.cursorTo("layout")
	before := d.splitAt()
	if before == 0 {
		d.fatal("no two prompts side by side in the drawing")
	}

	d.keys("right")

	d.doesntSee(" areas ")
	if after := d.splitAt(); after != before {
		d.fatal("edit starts at column %d, not %d", after, before)
	}
	d.keys("ctrl+s")
	d.sees("grid:")
	d.keys("y")
	// edit is 129 of the 160 columns; tmux's default 80 would make it 49
	// of 80, 61%.
	if config := d.saved(); !strings.Contains(config, "size: 81%") {
		t.Fatalf("edit's split isn't 81%% of the window:\n%s", config)
	}
}

func TestUndoGoesBackAndRedoForward(t *testing.T) {
	d := editing(t, "notes", notesConfig)
	d.cursorTo("layout")
	d.keys("right")

	d.keys("u")

	d.sees(" areas ")
	d.doesntSee("● unsaved")
	d.keys("ctrl+r")
	d.doesntSee(" areas ")
	d.sees("● unsaved")
}

func TestUndoWithNothingToUndoSaysSo(t *testing.T) {
	d := editing(t, "notes", notesConfig)

	d.keys("u")

	d.sees("✗ Nothing to undo.")
}

func TestLeavingChangesAsksFirst(t *testing.T) {
	d := editing(t, "notes", notesConfig)
	d.cursorTo("layout")
	d.keys("right")

	d.keys("esc")

	d.sees("Discard your changes?")
	d.keys("n")
	d.sees("● unsaved")
	d.keys("esc", "y")
	if !d.quit || d.form.saved {
		d.fatal("the form didn't leave without saving")
	}
}

func TestLeavingUnchangedEndsAtOnce(t *testing.T) {
	d := editing(t, "notes", notesConfig)

	d.keys("esc")

	if !d.quit || d.form.saved {
		d.fatal("the form didn't leave without saving")
	}
}

func TestFormScrollsBackToItsTop(t *testing.T) {
	d := editing(t, "notes", notesConfig)
	for range 60 {
		if !strings.Contains(d.screen(), "╭─ project") {
			break
		}
		d.keys("down")
	}
	d.doesntSee("╭─ project")
	root := regexp.MustCompile(`› +root`)
	for range 60 {
		if root.MatchString(d.screen()) {
			break
		}
		d.keys("up")
	}

	d.sees("╭─ project")
	d.sees("   name      notes")
}
