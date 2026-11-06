package domain

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Repo is a git repository a project expects at dir, relative to its
// root ("." is the root itself), cloned from remote when missing.
type Repo struct {
	dir    string
	remote string
}

// NewRepo validates and constructs a Repo.
func NewRepo(dir, remote string) (Repo, error) {
	d, err := oneLine(dir)
	if err != nil {
		return Repo{}, err
	}
	if filepath.IsAbs(d) {
		return Repo{}, fmt.Errorf("%w: %q is absolute", ErrInvalidRepoDir, dir)
	}
	d = filepath.Clean(d) // "" becomes "."
	if d == ".." || strings.HasPrefix(d, "../") {
		return Repo{}, fmt.Errorf("%w: %q leaves the project root", ErrInvalidRepoDir, dir)
	}
	r, err := oneLine(remote)
	if err != nil {
		return Repo{}, err
	}
	if r == "" {
		return Repo{}, fmt.Errorf("%q: %w", d, ErrEmptyRemote)
	}
	return Repo{dir: d, remote: r}, nil
}

// Dir is relative to the project root; "." is the root itself.
func (r Repo) Dir() string {
	return r.dir
}

func (r Repo) Remote() string {
	return r.remote
}

// Path is where the repository lives under root.
func (r Repo) Path(root ProjectRoot) string {
	return filepath.Join(root.path, r.dir)
}

// Contains reports whether dir is the repository's directory under root,
// or a directory inside it.
func (r Repo) Contains(root ProjectRoot, dir string) bool {
	return ProjectRoot{path: r.Path(root)}.Contains(dir)
}

// Repos are a project's repositories, at most one per directory.
type Repos struct {
	list []Repo
}

// NewRepos validates and constructs Repos.
func NewRepos(repos ...Repo) (Repos, error) {
	sorted := slices.Clone(repos)
	slices.SortFunc(sorted, func(a, b Repo) int {
		return cloneOrder(a.dir, b.dir)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].dir == sorted[i-1].dir {
			return Repos{}, fmt.Errorf("%w: %q", ErrDuplicateRepo, sorted[i].dir)
		}
	}
	return Repos{list: sorted}, nil
}

// cloneOrder sorts the root first and a directory before the ones inside
// it, so a repository is cloned before repositories nested in it.
func cloneOrder(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == ".":
		return -1
	case b == ".":
		return 1
	}
	return strings.Compare(a, b)
}

// List is the repositories in clone order.
func (r Repos) List() []Repo {
	return slices.Clone(r.list)
}
