package infra

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LocalWorkspace is the local filesystem.
type LocalWorkspace struct{}

// NewLocalWorkspace constructs a LocalWorkspace.
func NewLocalWorkspace() LocalWorkspace {
	return LocalWorkspace{}
}

// Exists implements domain.Workspace.
func (LocalWorkspace) Exists(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// Executable implements domain.Workspace, looking programs up on weave's
// own PATH, which usually is the one panes' shells start from. A program
// found in a relative PATH entry, such as ".", counts as found.
func (LocalWorkspace) Executable(program, dir string) bool {
	if strings.Contains(program, "/") && !filepath.IsAbs(program) {
		program = filepath.Join(dir, program)
	}
	_, err := exec.LookPath(program)
	return err == nil || errors.Is(err, exec.ErrDot)
}

// RealPath implements domain.Workspace.
func (LocalWorkspace) RealPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}
