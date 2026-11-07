package infra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// workingTreeTimeout bounds reading a working tree, so a huge repository
// doesn't hold up the browser: it then shows none.
const workingTreeTimeout = time.Second

// WorkingTree implements domain.Workspace, from git's machine-readable
// status. It doesn't take the index lock, so it never makes a git command
// the user runs meanwhile fail.
func (w GitWorkspace) WorkingTree(dir string) (domain.WorkingTree, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), workingTreeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "-C", dir,
		"status", "--porcelain=v2", "--branch")
	// git's children, such as a submodule's status or an fsmonitor hook,
	// inherit its pipes: once git is killed, don't wait for them too.
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) { // git itself finished
		return domain.WorkingTree{}, false
	}
	tree := domain.DetachedHead()
	dirty := false
	var ahead, behind int
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		header, ok := strings.CutPrefix(line, "# ")
		if !ok {
			dirty = dirty || line != ""
			continue
		}
		key, value, _ := strings.Cut(header, " ")
		switch key {
		case "branch.head":
			if value != "(detached)" {
				tree = domain.OnBranch(value)
			}
		case "branch.ab": // "+<ahead> -<behind>"
			a, b, _ := strings.Cut(value, " ")
			ahead, _ = strconv.Atoi(strings.TrimPrefix(a, "+"))
			behind, _ = strconv.Atoi(strings.TrimPrefix(b, "-"))
		}
	}
	if dirty {
		tree = tree.WithChanges()
	}
	return tree.Diverged(ahead, behind), true
}

// Detect implements domain.RepoDetector. A directory counts only when
// it is a repository's top level (a project's subdirectory inside its
// repository isn't another repository), isn't a submodule of the root,
// which brings it, and has an origin remote.
func (w GitWorkspace) Detect(root domain.ProjectRoot) (domain.Repos, error) {
	entries, err := os.ReadDir(root.String())
	if errors.Is(err, os.ErrNotExist) {
		return domain.Repos{}, nil
	}
	if err != nil {
		return domain.Repos{}, err
	}
	dirs := []string{"."}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs = append(dirs, entry.Name())
		}
	}
	resolvedRoot, err := filepath.EvalSymlinks(root.String())
	if err != nil {
		return domain.Repos{}, err
	}
	var repos []domain.Repo
	for _, dir := range dirs {
		remote, ok := originOf(filepath.Join(root.String(), dir), resolvedRoot)
		if !ok {
			continue
		}
		repo, err := domain.NewRepo(dir, remote)
		if err != nil {
			return domain.Repos{}, err
		}
		repos = append(repos, repo)
	}
	return domain.NewRepos(repos...)
}

// originOf is the origin remote of the repository whose top level is dir,
// unless it is a submodule of root, which brings it. root is resolved.
func originOf(dir, root string) (string, bool) {
	// A second line is the superproject of a submodule.
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel", "--show-superproject-working-tree").Output()
	if err != nil {
		return "", false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || lines[0] != resolved || len(lines) > 1 && lines[1] == root {
		return "", false
	}
	remote, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", false
	}
	return withoutCredentials(strings.TrimSpace(string(remote))), true
}

// withoutCredentials drops the user info of an http(s) remote, which
// configs would otherwise keep: a token is often the user name. An ssh
// user, such as git@, isn't a secret and stays.
func withoutCredentials(remote string) string {
	u, err := url.Parse(remote)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User == nil {
		return remote
	}
	u.User = nil
	return u.String()
}
