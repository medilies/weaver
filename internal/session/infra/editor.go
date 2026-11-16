package infra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/medilies/weaver/internal/session/app"
)

// Editor opens configs in the user's editor: the settings', else $VISUAL,
// else $EDITOR, else vi. Like git, it runs the editor through sh, so
// "code --wait" works.
type Editor struct {
	preferred func() string // the settings' editor, or ""
}

// NewEditor constructs an Editor going by $VISUAL and $EDITOR.
func NewEditor() Editor {
	return Editor{}
}

// Preferring is e opening configs in the editor preferred says, when it
// says one, read as each config is opened.
func (e Editor) Preferring(preferred func() string) Editor {
	e.preferred = preferred
	return e
}

// Edit implements app.ConfigEditor. The editor works on a copy named like
// the stored config, so it opens in the config's format, in a temporary
// directory.
func (e Editor) Edit(document app.ConfigDocument) ([]byte, error) {
	dir, err := os.MkdirTemp("", "weaver-edit-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, filepath.Base(document.File))
	if err := os.WriteFile(path, document.Content, 0o600); err != nil {
		return nil, err
	}

	var editor string
	if e.preferred != nil {
		editor = e.preferred()
	}
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command("/bin/sh", "-c", editor+` "$1"`, "weaver-edit", path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running %s: %w", editor, err)
	}
	return os.ReadFile(path)
}
