package infra

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/medilies/weaver/internal/session/domain"
)

// GitWorkspace is the local filesystem, with git to clone repositories.
type GitWorkspace struct {
	progress io.Writer
}

// NewGitWorkspace shows git's progress and prompts on progress.
func NewGitWorkspace(progress io.Writer) GitWorkspace {
	return GitWorkspace{progress: progress}
}

// Exists implements domain.Workspace.
func (w GitWorkspace) Exists(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// Clone implements domain.Workspace.
func (w GitWorkspace) Clone(checkout domain.Checkout, dir string) error {
	cmd := exec.Command("git", "clone", "--", checkout.Remote(), dir)
	cmd.Stdout, cmd.Stderr = w.progress, w.progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", checkout.Remote(), err)
	}
	return nil
}
