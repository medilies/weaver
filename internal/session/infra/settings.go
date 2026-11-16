package infra

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"go.yaml.in/yaml/v3"
)

const (
	// settingsFileName is the settings file.
	settingsFileName = "settings.yml"
	// settingsSchemaFileName is the JSON Schema saved beside it, which it
	// points to with "$schema" so editors complete and check it.
	settingsSchemaFileName = "settings.schema.json"
)

//go:embed settings.schema.json
var settingsSchema []byte

// Settings are weaver-wide preferences, the same for every project: the
// theme, the keys, and how the browser behaves.
type Settings struct {
	palette      palette
	keys         DrawingKeys
	preview      bool
	previewEvery time.Duration
	miniatures   bool
	mouse        bool
	editor       string
	tmuxTimeout  time.Duration
	browserKeys  map[string]string // the browser's keys set, by setting name
	formKeys     map[string]string // the config form's keys set, by setting name, but the drawing's
	popup        bool
}

// DrawingKeys are the config form's keys on the drawing: tmux's unless
// set.
type DrawingKeys struct {
	splitRight, splitBelow, remove, shrink, grow string
}

// Editor is the editor the settings open configs in, or "" to go by
// $VISUAL and $EDITOR.
func (s Settings) Editor() string {
	return s.editor
}

// Popup reports whether the browser and the config form, started in a
// tmux pane, re-run themselves in a popup the size of the client.
func (s Settings) Popup() bool {
	return s.popup
}

// TmuxTimeout is how long weave waits on a tmux command before giving
// up.
func (s Settings) TmuxTimeout() time.Duration {
	return s.tmuxTimeout
}

// DefaultSettings are the settings without a settings file: the default
// theme, cyan, and tmux's keys.
func DefaultSettings() Settings {
	return Settings{
		palette:      defaultPalette(),
		keys:         DrawingKeys{splitRight: "%", splitBelow: "\"", remove: "x", shrink: "H", grow: "L"},
		preview:      true,
		previewEvery: time.Second,
		miniatures:   true,
		mouse:        true,
		tmuxTimeout:  5 * time.Second,
	}
}

// SettingsFile is the settings file in dir.
func SettingsFile(dir string) string {
	return filepath.Join(dir, settingsFileName)
}

// LoadSettings reads the settings file in dir, the defaults when there
// is none. An invalid file is an error naming the file and the field. It
// saves the settings' JSON Schema in dir, when dir exists, so it is
// there before the file is; failing to only warns, on warnings, since
// weave works without it.
func LoadSettings(dir string, warnings io.Writer) (Settings, error) {
	if err := writeSettingsSchema(dir); err != nil {
		// A warning that can't be written changes nothing worth failing for.
		_, _ = fmt.Fprintf(warnings, "warning: %v\n", err)
	}
	path := SettingsFile(dir)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	settings, err := decodeSettings(raw)
	if err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}
	return settings, nil
}

// writeSettingsSchema saves the settings' JSON Schema in dir, unless it
// is there already or dir doesn't exist: weave makes the folder only to
// register a project.
func writeSettingsSchema(dir string) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	path := filepath.Join(dir, settingsSchemaFileName)
	if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, settingsSchema) {
		return nil
	}
	return writeFileAtomic(path, settingsSchema)
}

// settingsFile is the settings file's format.
type settingsFile struct {
	SelectedColour yaml.Node `yaml:"selected-colour"`
	Theme          themeFile `yaml:"theme"`
	Keys           keysFile  `yaml:"keys"`
	Preview        *bool     `yaml:"preview"`
	PreviewEvery   *string   `yaml:"preview-every"`
	Miniatures     *bool     `yaml:"miniatures"`
	Mouse          *bool     `yaml:"mouse"`
	Editor         *string   `yaml:"editor"`
	TmuxTimeout    *string   `yaml:"tmux-timeout"`
	Popup          *bool     `yaml:"popup"`
	// Projects lists the registered projects, which FileProjectRegistry
	// reads.
	Projects yaml.Node `yaml:"projects"`
}

// themeFile is the format of the settings' theme.
type themeFile struct {
	Selected yaml.Node `yaml:"selected"`
	Ok       yaml.Node `yaml:"ok"`
	Warning  yaml.Node `yaml:"warning"`
	Failure  yaml.Node `yaml:"failure"`
	Bright   yaml.Node `yaml:"bright"`
	Muted    yaml.Node `yaml:"muted"`
}

// keysFile is the format of the settings' keys.
type keysFile struct {
	SplitRight *string `yaml:"split-right"`
	SplitBelow *string `yaml:"split-below"`
	Remove     *string `yaml:"remove"`
	Shrink     *string `yaml:"shrink"`
	Grow       *string `yaml:"grow"`
	// The browser's.
	Start     *string `yaml:"start"`
	Bind      *string `yaml:"bind"`
	Edit      *string `yaml:"edit"`
	EditRaw   *string `yaml:"edit-raw"`
	Kill      *string `yaml:"kill"`
	Delete    *string `yaml:"delete"`
	Rerun     *string `yaml:"rerun"`
	Reload    *string `yaml:"reload"`
	Preview   *string `yaml:"preview"`
	Config    *string `yaml:"config"`
	Tasks     *string `yaml:"tasks"`
	Filter    *string `yaml:"filter"`
	Run       *string `yaml:"run"`
	Throwaway *string `yaml:"throwaway"`
	Interrupt *string `yaml:"interrupt"`
	Quit      *string `yaml:"quit"`
	// The config form's.
	Undo     *string `yaml:"undo"`
	Redo     *string `yaml:"redo"`
	Save     *string `yaml:"save"`
	MoveUp   *string `yaml:"move-up"`
	MoveDown *string `yaml:"move-down"`
}

// written are the keys the file sets, by setting name, in the order the
// file's format lists them.
func (k keysFile) written() []writtenKey {
	all := []writtenKey{
		{"split-right", k.SplitRight}, {"split-below", k.SplitBelow}, {"remove", k.Remove},
		{"shrink", k.Shrink}, {"grow", k.Grow},
		{"start", k.Start}, {"bind", k.Bind}, {"edit", k.Edit}, {"edit-raw", k.EditRaw},
		{"kill", k.Kill}, {"delete", k.Delete}, {"rerun", k.Rerun}, {"reload", k.Reload},
		{"preview", k.Preview}, {"config", k.Config}, {"tasks", k.Tasks}, {"filter", k.Filter},
		{"run", k.Run}, {"throwaway", k.Throwaway}, {"interrupt", k.Interrupt}, {"quit", k.Quit},
		{"undo", k.Undo}, {"redo", k.Redo}, {"save", k.Save}, {"move-up", k.MoveUp}, {"move-down", k.MoveDown},
	}
	return slices.DeleteFunc(all, func(w writtenKey) bool { return w.value == nil })
}

// writtenKey is a key setting the file sets.
type writtenKey struct {
	field string
	value *string
}

// settingsTypes names the file's shapes in YAML's errors as the file
// does.
var settingsTypes = strings.NewReplacer("infra.settingsFile", "settings", "infra.keysFile", "keys", "infra.themeFile", "theme")

var (
	hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// keyName is a key as Bubble Tea names it: one character, or a name
	// such as space or f5, after any modifiers.
	// shiftedCharacter is shift with a character, which terminals send as
	// the shifted character itself: shift+a is A.
	shiftedCharacter = regexp.MustCompile(`(^|\+)shift\+[^\s+]$`)
	keyName          = regexp.MustCompile(`^((ctrl|alt|shift|super|hyper|meta)\+)*([^\s+]|\+|[a-z][a-z0-9]+)$`)
)

var (
	// ErrInvalidColour is returned for a colour that is neither an ANSI
	// colour number nor "#rrggbb".
	ErrInvalidColour = errors.New("invalid colour")
	// ErrInvalidShade is returned for a shade that is neither a colour nor
	// a map of colour, bold and faint.
	ErrInvalidShade = errors.New("invalid shade")
	// ErrSelectedColourTwice is returned when selected-colour and
	// theme.selected are both set.
	ErrSelectedColourTwice = errors.New("selected-colour and theme.selected are the same setting: keep one")
	// ErrEmptyKey is returned for a key set to nothing.
	ErrEmptyKey = errors.New("a key can't be empty")
	// ErrInvalidKey is returned for a key no terminal sends as written,
	// such as " " for space or ctrl-x for ctrl+x.
	ErrInvalidKey = errors.New("invalid key")
	// ErrInvalidDuration is returned for a duration that isn't one, such
	// as "1 second", or is out of its setting's range.
	ErrInvalidDuration = errors.New("invalid duration")
	// ErrEmptyEditor is returned for an editor set to nothing.
	ErrEmptyEditor = errors.New("an editor can't be empty")
	// ErrTrailingSettings is returned when the settings file holds a
	// second document.
	ErrTrailingSettings = errors.New("unexpected data after the settings object")
	// ErrKeyTaken is returned for a drawing key the config form already
	// uses, or one two drawing keys share.
	ErrKeyTaken = errors.New("key taken")
)

// decodeSettings reads raw, any setting it leaves out keeping its
// default.
func decodeSettings(raw []byte) (Settings, error) {
	var file settingsFile
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return Settings{}, errors.New(settingsTypes.Replace(err.Error()))
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Settings{}, ErrTrailingSettings
	}
	settings := DefaultSettings()
	if file.SelectedColour.Kind != 0 && file.Theme.Selected.Kind != 0 {
		return Settings{}, ErrSelectedColourTwice
	}
	if err := file.Theme.into(&settings.palette, file.SelectedColour); err != nil {
		return Settings{}, err
	}
	if file.Preview != nil {
		settings.preview = *file.Preview
	}
	if file.Popup != nil {
		settings.popup = *file.Popup
	}
	if file.TmuxTimeout != nil {
		timeout, err := parseDuration("tmux-timeout", *file.TmuxTimeout, 500*time.Millisecond, time.Minute)
		if err != nil {
			return Settings{}, err
		}
		settings.tmuxTimeout = timeout
	}
	if file.Editor != nil {
		if strings.TrimSpace(*file.Editor) == "" {
			return Settings{}, fmt.Errorf("editor: %w", ErrEmptyEditor)
		}
		settings.editor = *file.Editor
	}
	if file.Mouse != nil {
		settings.mouse = *file.Mouse
	}
	if file.Miniatures != nil {
		settings.miniatures = *file.Miniatures
	}
	if file.PreviewEvery != nil {
		every, err := parseDuration("preview-every", *file.PreviewEvery, 100*time.Millisecond, time.Minute)
		if err != nil {
			return Settings{}, err
		}
		settings.previewEvery = every
	}
	set := map[string]string{}
	for _, key := range file.Keys.written() {
		if *key.value == "" {
			return Settings{}, fmt.Errorf("keys.%s: %w", key.field, ErrEmptyKey)
		}
		if !keyName.MatchString(*key.value) || shiftedCharacter.MatchString(*key.value) {
			return Settings{}, fmt.Errorf("keys.%s: %w: %q (want a character, or a name such as space or ctrl+x)", key.field, ErrInvalidKey, *key.value)
		}
		set[key.field] = *key.value
	}
	keys := &settings.keys
	for field, into := range map[string]*string{
		"split-right": &keys.splitRight, "split-below": &keys.splitBelow, "remove": &keys.remove,
		"shrink": &keys.shrink, "grow": &keys.grow,
	} {
		if key, ok := set[field]; ok {
			*into = key
		}
	}
	settings.formKeys = map[string]string{}
	for _, action := range formActions {
		if key, ok := set[action.field]; ok {
			settings.formKeys[action.field] = key
		}
	}
	if err := checkKeys(append(slices.Clone(drawingActions), formActions...), set, formFixedKeys); err != nil {
		return Settings{}, err
	}
	settings.browserKeys = map[string]string{}
	for _, action := range browserActions {
		if key, ok := set[action.field]; ok {
			settings.browserKeys[action.field] = key
		}
	}
	if err := checkKeys(browserActions, settings.browserKeys, browserFixedKeys); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// parseDuration reads value, field's, as a duration from lowest to
// highest, such as "500ms" or "2s".
func parseDuration(field, value string, lowest, highest time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d < lowest || d > highest {
		return 0, fmt.Errorf("%s: %w: %q (want %s to %s, such as %q)", field, ErrInvalidDuration, value, lowest, highest, "2s")
	}
	return d, nil
}

// into sets on p the colours and shades the theme sets, selected-colour
// standing for theme.selected.
func (f themeFile) into(p *palette, selectedColour yaml.Node) error {
	colours := []struct {
		field string
		node  *yaml.Node
		into  *color.Color
	}{
		{"selected-colour", &selectedColour, &p.selected},
		{"theme.selected", &f.Selected, &p.selected},
		{"theme.ok", &f.Ok, &p.ok},
		{"theme.warning", &f.Warning, &p.warning},
		{"theme.failure", &f.Failure, &p.failure},
	}
	for _, c := range colours {
		if c.node.Kind == 0 {
			continue
		}
		colour, err := parseColour(c.field, c.node)
		if err != nil {
			return err
		}
		*c.into = colour
	}
	for _, s := range []struct {
		field string
		node  *yaml.Node
		into  *shade
	}{{"theme.bright", &f.Bright, &p.bright}, {"theme.muted", &f.Muted, &p.muted}} {
		if s.node.Kind == 0 {
			continue
		}
		parsed, err := parseShade(s.field, s.node)
		if err != nil {
			return err
		}
		*s.into = parsed
	}
	return nil
}

// parseShade reads a shade: a colour, or a map of colour, bold and faint,
// those it leaves out unset.
func parseShade(field string, node *yaml.Node) (shade, error) {
	if node.Kind == yaml.ScalarNode {
		colour, err := parseColour(field, node)
		return shade{colour: colour}, err
	}
	if node.Kind != yaml.MappingNode {
		return shade{}, fmt.Errorf("%s: %w (want a colour, or a map of colour, bold and faint)", field, ErrInvalidShade)
	}
	var parsed shade
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		var err error
		switch key {
		case "colour":
			parsed.colour, err = parseColour(field+".colour", value)
		case "bold":
			parsed.bold, err = parseFlag(field+".bold", value)
		case "faint":
			parsed.faint, err = parseFlag(field+".faint", value)
		default:
			err = fmt.Errorf("%s: %w: unknown field %q (want colour, bold or faint)", field, ErrInvalidShade, key)
		}
		if err != nil {
			return shade{}, err
		}
	}
	return parsed, nil
}

// parseFlag reads true or false.
func parseFlag(field string, node *yaml.Node) (bool, error) {
	var flag bool
	if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" || node.Decode(&flag) != nil {
		return false, fmt.Errorf("%s: %w: %q (want true or false)", field, ErrInvalidShade, node.Value)
	}
	return flag, nil
}

// parseColour reads field's ANSI colour number, 0-255, or "#rrggbb".
func parseColour(field string, node *yaml.Node) (color.Color, error) {
	value := node.Value
	if node.Kind == yaml.ScalarNode {
		// Base 0 reads YAML's 0x10 and 0o20 as the schema's integers.
		if n, err := strconv.ParseInt(value, 0, 0); node.Tag == "!!int" && err == nil && n >= 0 && n <= 255 {
			return lipgloss.Color(strconv.FormatInt(n, 10)), nil
		}
		if node.Tag == "!!str" && hexColour.MatchString(value) {
			return lipgloss.Color(value), nil
		}
	}
	return nil, fmt.Errorf("%s: %w: %q (want an ANSI colour number, 0-255, or \"#rrggbb\")", field, ErrInvalidColour, value)
}
