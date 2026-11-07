package infra

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// GitWorkspace is the local filesystem, with git to clone repositories.
type GitWorkspace struct {
	progress io.Writer
}

// NewGitWorkspace constructs a GitWorkspace that writes git's progress
// and prompts to progress.
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
func (w GitWorkspace) Clone(repo domain.Repo, dir string) error {
	cmd := exec.Command("git", "clone", "--", repo.Remote(), dir)
	cmd.Stdout, cmd.Stderr = w.progress, w.progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", repo.Remote(), err)
	}
	return nil
}

// Executable implements domain.Workspace, looking programs up on weave's
// own PATH, which usually is the one panes' shells start from. A program
// found in a relative PATH entry, such as ".", counts as found.
func (w GitWorkspace) Executable(program, dir string) bool {
	if strings.Contains(program, "/") && !filepath.IsAbs(program) {
		program = filepath.Join(dir, program)
	}
	_, err := exec.LookPath(program)
	return err == nil || errors.Is(err, exec.ErrDot)
}
