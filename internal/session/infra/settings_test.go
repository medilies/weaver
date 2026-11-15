package infra

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

// settingsIn writes content as the settings file of a new folder and
// loads it.
func settingsIn(t *testing.T, content string) (Settings, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, settingsFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return LoadSettings(dir, io.Discard)
}

func TestLoadSettings(t *testing.T) {
	t.Run("are the defaults without a settings file", func(t *testing.T) {
		got, err := LoadSettings(t.TempDir(), io.Discard)
		if err != nil || !reflect.DeepEqual(got, DefaultSettings()) {
			t.Fatalf("got %+v (%v), want the defaults", got, err)
		}
	})

	t.Run("read an ANSI colour number", func(t *testing.T) {
		got, err := settingsIn(t, "selected-colour: 214\n")
		if err != nil || got.colour != lipgloss.Color("214") {
			t.Fatalf("got %v (%v), want colour 214", got.colour, err)
		}
	})

	t.Run("read an ANSI colour number written in hex", func(t *testing.T) {
		got, err := settingsIn(t, "selected-colour: 0x10\n")
		if err != nil || got.colour != lipgloss.Color("16") {
			t.Fatalf("got %v (%v), want colour 16", got.colour, err)
		}
	})

	t.Run("read a hex colour", func(t *testing.T) {
		got, err := settingsIn(t, "selected-colour: \"#5fafd7\"\n")
		if err != nil || got.colour != lipgloss.Color("#5fafd7") {
			t.Fatalf("got %v (%v), want #5fafd7", got.colour, err)
		}
	})

	t.Run("keep tmux's keys for those left unset", func(t *testing.T) {
		got, err := settingsIn(t, "keys:\n  split-right: \"|\"\n  split-below: \"-\"\n")
		want := DrawingKeys{splitRight: "|", splitBelow: "-", remove: "x", shrink: "H", grow: "L"}
		if err != nil || got.keys != want {
			t.Fatalf("got %+v (%v), want %+v", got.keys, err, want)
		}
	})

	t.Run("turn the preview off", func(t *testing.T) {
		got, err := settingsIn(t, "preview: false\n")
		if err != nil || got.preview {
			t.Fatalf("got preview %t (%v), want it off", got.preview, err)
		}
	})

	t.Run("read how often the browser finds what it shows again", func(t *testing.T) {
		got, err := settingsIn(t, "preview-every: 500ms\n")
		if err != nil || got.previewEvery != 500*time.Millisecond {
			t.Fatalf("got %v (%v), want 500ms", got.previewEvery, err)
		}
	})

	t.Run("refuse a duration that isn't one, or is out of its range", func(t *testing.T) {
		for _, every := range []string{"1 second", "10", "50ms", "2m", "-1s"} {
			if _, err := settingsIn(t, "preview-every: "+every+"\n"); !errors.Is(err, ErrInvalidDuration) || !strings.Contains(err.Error(), "preview-every: ") {
				t.Errorf("%s: got %v, want ErrInvalidDuration naming preview-every", every, err)
			}
		}
	})

	t.Run("turn the miniatures off", func(t *testing.T) {
		got, err := settingsIn(t, "miniatures: false\n")
		if err != nil || got.miniatures {
			t.Fatalf("got miniatures %t (%v), want them off", got.miniatures, err)
		}
	})

	t.Run("turn the mouse off", func(t *testing.T) {
		got, err := settingsIn(t, "mouse: false\n")
		if err != nil || got.mouse {
			t.Fatalf("got mouse %t (%v), want it off", got.mouse, err)
		}
	})

	t.Run("read the editor", func(t *testing.T) {
		got, err := settingsIn(t, "editor: code --wait\n")
		if err != nil || got.Editor() != "code --wait" {
			t.Fatalf("got %q (%v), want code --wait", got.Editor(), err)
		}
	})

	t.Run("refuse an empty editor", func(t *testing.T) {
		if _, err := settingsIn(t, "editor: \" \"\n"); !errors.Is(err, ErrEmptyEditor) {
			t.Fatalf("got %v, want ErrEmptyEditor", err)
		}
	})

	t.Run("read how long to wait on tmux", func(t *testing.T) {
		got, err := settingsIn(t, "tmux-timeout: 10s\n")
		if err != nil || got.TmuxTimeout() != 10*time.Second {
			t.Fatalf("got %v (%v), want 10s", got.TmuxTimeout(), err)
		}
	})

	t.Run("refuse a tmux timeout out of its range", func(t *testing.T) {
		if _, err := settingsIn(t, "tmux-timeout: 100ms\n"); !errors.Is(err, ErrInvalidDuration) || !strings.Contains(err.Error(), "tmux-timeout: ") {
			t.Fatalf("got %v, want ErrInvalidDuration naming tmux-timeout", err)
		}
	})

	t.Run("refuse a colour that is neither a number to 255 nor #rrggbb", func(t *testing.T) {
		for _, colour := range []string{"256", "-1", "cyan", `"#fff"`, `"6"`, "[6]"} {
			if _, err := settingsIn(t, "selected-colour: "+colour+"\n"); !errors.Is(err, ErrInvalidColour) {
				t.Errorf("%s: got %v, want ErrInvalidColour", colour, err)
			}
		}
	})

	t.Run("name the file and the field of an invalid setting", func(t *testing.T) {
		_, err := settingsIn(t, "selected-colour: cyan\n")
		if err == nil || !strings.Contains(err.Error(), settingsFileName+": selected-colour: ") {
			t.Fatalf("got %v, want the file and field named", err)
		}
	})

	t.Run("refuse a field it doesn't know", func(t *testing.T) {
		if _, err := settingsIn(t, "colour: 6\n"); err == nil {
			t.Fatal("got no error, want the unknown field refused")
		}
	})

	t.Run("refuse an empty key", func(t *testing.T) {
		if _, err := settingsIn(t, "keys:\n  remove: \"\"\n"); !errors.Is(err, ErrEmptyKey) {
			t.Fatalf("got %v, want ErrEmptyKey", err)
		}
	})

	t.Run("refuse a key no terminal sends as written", func(t *testing.T) {
		for _, key := range []string{`" "`, "ctrl-x", "Space", "ctrl+", "shift+a"} {
			if _, err := settingsIn(t, "keys:\n  remove: "+key+"\n"); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("%s: got %v, want ErrInvalidKey", key, err)
			}
		}
	})

	t.Run("read a key by its name", func(t *testing.T) {
		got, err := settingsIn(t, "keys:\n  remove: ctrl+x\n  grow: space\n  shrink: \"+\"\n")
		if err != nil || got.keys.remove != "ctrl+x" || got.keys.grow != "space" || got.keys.shrink != "+" {
			t.Fatalf("got %+v (%v), want ctrl+x, space and +", got.keys, err)
		}
	})

	t.Run("refuse a second document", func(t *testing.T) {
		if _, err := settingsIn(t, "selected-colour: 6\n---\nselected-colour: 2\n"); !errors.Is(err, ErrTrailingSettings) {
			t.Fatalf("got %v, want ErrTrailingSettings", err)
		}
	})

	t.Run("name an unknown field without Go's types", func(t *testing.T) {
		_, err := settingsIn(t, "keys:\n  split: x\n")
		if err == nil || strings.Contains(err.Error(), "infra.") || strings.Contains(err.Error(), "SplitRight") {
			t.Fatalf("got %v, want the unknown field named in the file's words", err)
		}
	})

	t.Run("refuse a key the config form already uses, saying what it does", func(t *testing.T) {
		_, err := settingsIn(t, "keys:\n  remove: u\n")
		if !errors.Is(err, ErrKeyTaken) || !strings.Contains(err.Error(), `keys.remove: key taken: "u" already does undo`) {
			t.Fatalf("got %v, want u refused as undo", err)
		}
	})

	t.Run("refuse two drawing keys the same", func(t *testing.T) {
		_, err := settingsIn(t, "keys:\n  grow: H\n")
		if !errors.Is(err, ErrKeyTaken) || !strings.Contains(err.Error(), "already does keys.shrink") {
			t.Fatalf("got %v, want H refused as shrink's", err)
		}
	})
}

func TestLoadSettingsSavesTheSchema(t *testing.T) {
	t.Run("in a folder that exists", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := LoadSettings(dir, io.Discard); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, settingsSchemaFileName)); err != nil || !slices.Equal(got, settingsSchema) {
			t.Fatalf("got %d bytes (%v), want the schema", len(got), err)
		}
	})

	t.Run("without making the folder", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "weaver")
		if _, err := LoadSettings(dir, io.Discard); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("got %v, want no folder made", err)
		}
	})
}

func TestSettingsSchemaDeclaresEveryField(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(settingsSchema, &schema); err != nil {
		t.Fatalf("parsing the schema: %v", err)
	}
	fields := func(of reflect.Type) []string {
		var names []string
		for field := range of.Fields() {
			names = append(names, strings.Split(field.Tag.Get("yaml"), ",")[0])
		}
		slices.Sort(names)
		return names
	}
	keys := func(m map[string]any) []string {
		var names []string
		for name := range m {
			names = append(names, name)
		}
		slices.Sort(names)
		return names
	}
	top := map[string]any{}
	for name := range schema.Properties {
		top[name] = nil
	}
	file := reflect.TypeFor[settingsFile]()
	if got, want := keys(top), fields(file); !slices.Equal(got, want) {
		t.Errorf("schema has %q, the file %q", got, want)
	}
	keysField, _ := file.FieldByName("Keys")
	if got, want := keys(schema.Properties["keys"].Properties), fields(keysField.Type); !slices.Equal(got, want) {
		t.Errorf("schema's keys have %q, the file %q", got, want)
	}
}
