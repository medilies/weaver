package infra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/medilies/weaver/internal/session/domain"
)

// Editor opens configs in the user's editor: $VISUAL, else $EDITOR, else
// vi. Like git, it runs the editor through sh, so "code --wait" works.
type Editor struct{}

// NewEditor constructs an Editor.
func NewEditor() Editor {
	return Editor{}
}

// Edit implements app.ConfigEditor. The editor works on a copy, in a
// temporary directory that also holds the JSON Schema the config points
// to.
func (Editor) Edit(name domain.ProjectName, document []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "weaver-edit-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, name.String()+".json")
	if err := os.WriteFile(filepath.Join(dir, schemaFileName), schema, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, document, 0o600); err != nil {
		return nil, err
	}

	editor := os.Getenv("VISUAL")
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
