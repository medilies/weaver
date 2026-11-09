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
	"strconv"
	"strings"

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
// selected colour and the config form's keys on the drawing.
type Settings struct {
	colour color.Color
	keys   DrawingKeys
}

// DrawingKeys are the config form's keys on the drawing: tmux's unless
// set.
type DrawingKeys struct {
	splitRight, splitBelow, remove, shrink, grow string
}

// DefaultSettings are the settings without a settings file: cyan, and
// tmux's keys.
func DefaultSettings() Settings {
	return Settings{colour: lipgloss.Color("6"), keys: DrawingKeys{splitRight: "%", splitBelow: "\"", remove: "x", shrink: "H", grow: "L"}}
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
}

// keysFile is the format of the settings' keys.
type keysFile struct {
	SplitRight *string `yaml:"split-right"`
	SplitBelow *string `yaml:"split-below"`
	Remove     *string `yaml:"remove"`
	Shrink     *string `yaml:"shrink"`
	Grow       *string `yaml:"grow"`
}

// settingsTypes names the file's shapes in YAML's errors as the file
// does.
var settingsTypes = strings.NewReplacer("infra.settingsFile", "settings", "infra.keysFile", "keys")

var (
	hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// keyName is a key as Bubble Tea names it: one character, or a name
	// such as space or f5, after any modifiers.
	keyName = regexp.MustCompile(`^((ctrl|alt|shift|super|hyper|meta)\+)*([^\s+]|\+|[a-z][a-z0-9]+)$`)
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
	keys := &settings.keys
	for _, key := range []struct {
		field string
		value *string
		into  *string
	}{
		{"split-right", file.Keys.SplitRight, &keys.splitRight},
		{"split-below", file.Keys.SplitBelow, &keys.splitBelow},
		{"remove", file.Keys.Remove, &keys.remove},
		{"shrink", file.Keys.Shrink, &keys.shrink},
		{"grow", file.Keys.Grow, &keys.grow},
	} {
		if key.value == nil {
			continue
		}
		if *key.value == "" {
			return Settings{}, fmt.Errorf("keys.%s: %w", key.field, ErrEmptyKey)
		}
		if !keyName.MatchString(*key.value) {
			return Settings{}, fmt.Errorf("keys.%s: %w: %q (want a character, or a name such as space or ctrl+x)", key.field, ErrInvalidKey, *key.value)
		}
		*key.into = *key.value
	}
	if err := keys.free(); err != nil {
		return Settings{}, err
	}
	return settings, nil
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

// look is how the selected colour shows: text in it, or a block of it.
type look struct {
	picked, selected lipgloss.Style
}

func (s Settings) look() look {
	return look{
		picked:   lipgloss.NewStyle().Foreground(s.colour),
		selected: lipgloss.NewStyle().Background(s.colour).Foreground(lipgloss.Color("0")),
	}
}
