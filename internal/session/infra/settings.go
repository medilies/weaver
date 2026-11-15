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
	// settingsFileName is the settings file, beside the projects' folder.
	settingsFileName = "settings.yml"
	// settingsSchemaFileName is the JSON Schema saved beside it, which it
	// points to with "$schema" so editors complete and check it.
	settingsSchemaFileName = "settings.schema.json"
)

//go:embed settings.schema.json
var settingsSchema []byte

// Settings are weaver-wide preferences, the same for every project: the
// selected colour, the keys, and how the browser behaves.
type Settings struct {
	colour       color.Color
	keys         DrawingKeys
	preview      bool
	previewEvery time.Duration
	miniatures   bool
	mouse        bool
	editor       string
	tmuxTimeout  time.Duration
	browserKeys  map[string]string // the browser's keys set, by setting name
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

// TmuxTimeout is how long weave waits on a tmux command before giving
// up.
func (s Settings) TmuxTimeout() time.Duration {
	return s.tmuxTimeout
}

// DefaultSettings are the settings without a settings file: cyan, and
// tmux's keys.
func DefaultSettings() Settings {
	return Settings{
		colour:       lipgloss.Color("6"),
		keys:         DrawingKeys{splitRight: "%", splitBelow: "\"", remove: "x", shrink: "H", grow: "L"},
		preview:      true,
		previewEvery: time.Second,
		miniatures:   true,
		mouse:        true,
		tmuxTimeout:  5 * time.Second,
	}
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
	path := filepath.Join(dir, settingsFileName)
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
// save a config.
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
	Keys           keysFile  `yaml:"keys"`
	Preview        *bool     `yaml:"preview"`
	PreviewEvery   *string   `yaml:"preview-every"`
	Miniatures     *bool     `yaml:"miniatures"`
	Mouse          *bool     `yaml:"mouse"`
	Editor         *string   `yaml:"editor"`
	TmuxTimeout    *string   `yaml:"tmux-timeout"`
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
var settingsTypes = strings.NewReplacer("infra.settingsFile", "settings", "infra.keysFile", "keys")

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
	// ErrInvalidColour is returned for a selected colour that is neither
	// an ANSI colour number nor "#rrggbb".
	ErrInvalidColour = errors.New("invalid colour")
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
	if node := &file.SelectedColour; node.Kind != 0 {
		colour, err := parseColour(node)
		if err != nil {
			return Settings{}, err
		}
		settings.colour = colour
	}
	if file.Preview != nil {
		settings.preview = *file.Preview
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
	if err := keys.free(); err != nil {
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

// parseColour reads an ANSI colour number, 0-255, or "#rrggbb".
func parseColour(node *yaml.Node) (color.Color, error) {
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
	return nil, fmt.Errorf("selected-colour: %w: %q (want an ANSI colour number, 0-255, or \"#rrggbb\")", ErrInvalidColour, value)
}

// free reports a drawing key the config form already uses, or two
// drawing keys the same, naming both.
func (k DrawingKeys) free() error {
	taken := map[string]string{}
	for key, does := range formKeys {
		taken[key] = does
	}
	for _, key := range []struct{ field, value string }{
		{"split-right", k.splitRight}, {"split-below", k.splitBelow}, {"remove", k.remove},
		{"shrink", k.shrink}, {"grow", k.grow},
	} {
		if does, ok := taken[key.value]; ok {
			return fmt.Errorf("keys.%s: %w: %q already does %s", key.field, ErrKeyTaken, key.value, does)
		}
		taken[key.value] = "keys." + key.field
	}
	return nil
}
