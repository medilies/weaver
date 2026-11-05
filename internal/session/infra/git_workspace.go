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
func (w GitWorkspace) Clone(checkout domain.Checkout, dir string) error {
	cmd := exec.Command("git", "clone", "--", checkout.Remote(), dir)
	cmd.Stdout, cmd.Stderr = w.progress, w.progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", checkout.Remote(), err)
	}
	return nil
}

// Detect implements domain.CheckoutDetector. A directory counts only when
// it is a repository's top level (a project's subdirectory inside its
// repository isn't another repository) and has an origin remote.
func (w GitWorkspace) Detect(root domain.ProjectRoot) (domain.Checkouts, error) {
	entries, err := os.ReadDir(root.String())
	if errors.Is(err, os.ErrNotExist) {
		return domain.Checkouts{}, nil
	}
	if err != nil {
		return domain.Checkouts{}, err
	}
	dirs := []string{"."}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs = append(dirs, entry.Name())
		}
	}
	var checkouts []domain.Checkout
	for _, dir := range dirs {
		remote, ok := originOf(filepath.Join(root.String(), dir))
		if !ok {
			continue
		}
		checkout, err := domain.NewCheckout(dir, remote)
		if err != nil {
			return domain.Checkouts{}, err
		}
		checkouts = append(checkouts, checkout)
	}
	return domain.NewCheckouts(checkouts...)
}

// originOf is the origin remote of the repository whose top level is dir.
func originOf(dir string) (string, bool) {
	top, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || strings.TrimSpace(string(top)) != resolved {
		return "", false
	}
	remote, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(remote)), true
}
