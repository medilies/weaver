package domain

// Workspace is the filesystem projects live in. Infrastructure provides
// the implementation.
type Workspace interface {
	// Exists reports whether dir is an existing directory.
	Exists(dir string) (bool, error)
	// Clone clones checkout's remote into dir, creating missing parents.
	Clone(checkout Checkout, dir string) error
	// WorkingTree is where the git repository dir is in stands, or false
	// when dir isn't in one.
	WorkingTree(dir string) (WorkingTree, bool)
	// Executable reports whether program can be run from dir: found on
	// PATH or, when it holds a slash, at that path from dir.
	Executable(program, dir string) bool
}

// CheckoutDetector finds the git repositories already in a project root.
// Infrastructure provides the implementation.
type CheckoutDetector interface {
	// Detect finds the repositories at root and in its immediate
	// directories, each with the remote it was cloned from.
	Detect(root ProjectRoot) (Checkouts, error)
}

// WorkingTree is where a git working tree stands: the branch checked out,
// whether it has changes not committed, and how far it is from the
// branch it tracks.
type WorkingTree struct {
	branch string
	dirty  bool
	ahead  int
	behind int
}

// OnBranch is a clean working tree on branch.
func OnBranch(branch string) WorkingTree {
	return WorkingTree{branch: branch}
}

// DetachedHead is a clean working tree on no branch.
func DetachedHead() WorkingTree {
	return WorkingTree{}
}

// WithChanges is t with changes not committed.
func (t WorkingTree) WithChanges() WorkingTree {
	t.dirty = true
	return t
}

// Diverged is t with commits its tracked branch lacks (ahead) and
// commits it lacks from it (behind); negative counts read as none.
func (t WorkingTree) Diverged(ahead, behind int) WorkingTree {
	t.ahead, t.behind = max(ahead, 0), max(behind, 0)
	return t
}

// Branch is the branch checked out, "" on a detached head.
func (t WorkingTree) Branch() string {
	return t.branch
}

// Detached reports whether no branch is checked out.
func (t WorkingTree) Detached() bool {
	return t.branch == ""
}

// Dirty reports whether the working tree has changes not committed,
// untracked files included.
func (t WorkingTree) Dirty() bool {
	return t.dirty
}

// Ahead is the number of commits the tracked branch lacks.
func (t WorkingTree) Ahead() int {
	return t.ahead
}

// Behind is the number of the tracked branch's commits missing here.
func (t WorkingTree) Behind() int {
	return t.behind
}
