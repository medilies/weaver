package app_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// fakeForm is an app.ProjectForm that records the drafts it was given
// and returns a set project, or an error; then, when set, the next
// projects in turn.
type fakeForm struct {
	draft   app.ProjectDraft
	drafts  []app.ProjectDraft
	project domain.Project
	next    []domain.Project
	err     error
}

func (f *fakeForm) Fill(draft app.ProjectDraft) (domain.Project, error) {
	f.draft = draft
	f.drafts = append(f.drafts, draft)
	project := f.project
	if len(f.next) > 0 {
		f.project, f.next = f.next[0], f.next[1:]
	}
	return project, f.err
}

// leftSecondTime is an app.ProjectForm returning a set project, then
// left by the user.
type leftSecondTime struct {
	project domain.Project
	filled  bool
}

func (f *leftSecondTime) Fill(app.ProjectDraft) (domain.Project, error) {
	if f.filled {
		return domain.Project{}, app.ErrAborted
	}
	f.filled = true
	return f.project, nil
}

// answer is an app.Confirmer giving the same answer to every question.
type answer bool

func (a answer) Confirm(string) (bool, error) {
	return bool(a), nil
}

// answers is an app.Confirmer giving its answers in turn.
type answers []bool

func (a *answers) Confirm(string) (bool, error) {
	answer := (*a)[0]
	*a = (*a)[1:]
	return answer, nil
}

// capturedSessions is a fake domain.SessionCapturer: running sessions'
// windows, and the session weaver runs in, if any.
type capturedSessions struct {
	windows map[string][]domain.Window
	current string
}

func (c capturedSessions) Capture(session string) ([]domain.Window, error) {
	windows, ok := c.windows[session]
	if !ok {
		return nil, fmt.Errorf("can't find session: %s", session)
	}
	return windows, nil
}

func (c capturedSessions) Current() (string, error) {
	if c.current == "" {
		return "", domain.ErrNotInsideTmux
	}
	return c.current, nil
}

type freezeSetup struct {
	configs   inMemoryRegistry
	form      *fakeForm
	current   string // the session weaver runs in; none outside tmux
	workspace *fakeWorkspace
}

func newFreezeSetup(t *testing.T) freezeSetup {
	t.Helper()
	return freezeSetup{
		configs:   inMemoryRegistry{projects: map[string]domain.Project{}},
		form:      &fakeForm{project: shop(t)},
		workspace: &fakeWorkspace{},
	}
}

func (s freezeSetup) freeze(t *testing.T, confirm app.Confirmer, request app.FreezeRequest) (domain.Project, error) {
	t.Helper()
	sessions := capturedSessions{
		windows: map[string][]domain.Window{
			"store-dev": shop(t).Windows(), "my.session": shop(t).Windows(), "resolved": resolvedShop(t),
		},
		current: s.current,
	}
	return app.NewFreezeProject(s.configs, sessions, s.workspace, s.form, confirm).Handle(request)
}

// resolvedShop is a session's windows as tmux reports them for a shop
// rooted at /home/me/shop, a symlink to /srv/shop.
func resolvedShop(t *testing.T) []domain.Window {
	t.Helper()
	dir, err := domain.NewPaneDir("/srv/shop/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	name, _ := domain.NewWindowName("main")
	window, err := domain.NewWindow(name, domain.NewPane().In(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return []domain.Window{window}
}

func TestFreezeProject(t *testing.T) {
	t.Run("starts from the working directory when no root is given", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/shop" || s.form.draft.Name != "shop" {
			t.Fatalf("got root %q and name %q, want /srv/shop and shop", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("resolves a relative root against the working directory", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{Root: "../blog", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/blog" || s.form.draft.Name != "blog" {
			t.Fatalf("got root %q and name %q, want /srv/blog and blog", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("suggests the name given", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{Name: "store", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store" {
			t.Fatalf("got name %q, want store", s.form.draft.Name)
		}
	})

	t.Run("has the name typed in the form", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.NameStays {
			t.Fatal("got the name kept, want it typed")
		}
	})

	t.Run("saves the project the user described", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, saved := s.configs.projects["shop"]; !saved {
			t.Fatal("got nothing saved, want shop")
		}
	})

	t.Run("keeps a registered project the user won't replace", func(t *testing.T) {
		s := newFreezeSetup(t)
		existing := rootedAt(t, "shop", "/srv/old-shop")
		s.configs.projects["shop"] = existing
		s.form.next = []domain.Project{rootedAt(t, "store", "/srv/shop")}
		_, err := s.freeze(t, answer(false), app.FreezeRequest{WorkingDir: "/srv/shop"})
		if err != nil || s.configs.projects["shop"].Root() != existing.Root() {
			t.Fatalf("got %v, root %s; want the old project kept", err, s.configs.projects["shop"].Root())
		}
	})

	t.Run("reopens the form typing the name when the user won't replace", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.configs.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		s.form.next = []domain.Project{rootedAt(t, "store", "/srv/shop")}
		if _, err := s.freeze(t, &answers{false}, app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		again := s.form.drafts[1]
		if again.Name != "shop" || again.Retype == nil || again.NameStays {
			t.Fatalf("got name %q, retype %v, stays %t; want shop typed again", again.Name, again.Retype, again.NameStays)
		}
	})

	t.Run("reopens the form on the project as filled in", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.configs.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		s.form.next = []domain.Project{rootedAt(t, "store", "/srv/shop")}
		if _, err := s.freeze(t, &answers{false}, app.FreezeRequest{WorkingDir: "/srv/blog"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		again := s.form.drafts[1]
		if again.Root != "/srv/shop" || len(again.Windows) != len(shop(t).Windows()) {
			t.Fatalf("got root %q and %d windows, want the filled-in shop's", again.Root, len(again.Windows))
		}
	})

	t.Run("saves under the name typed after declining to replace", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.configs.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		s.form.next = []domain.Project{rootedAt(t, "store", "/srv/shop")}
		if _, err := s.freeze(t, &answers{false}, app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, saved := s.configs.projects["store"]; !saved {
			t.Fatal("got store unsaved, want it saved")
		}
	})

	t.Run("stops when the user leaves the reopened form", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.configs.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		form := &leftSecondTime{project: shop(t)}
		_, err := app.NewFreezeProject(s.configs, capturedSessions{}, s.workspace, form, answer(false)).
			Handle(app.FreezeRequest{WorkingDir: "/srv/shop"})
		if !errors.Is(err, app.ErrAborted) {
			t.Fatalf("got %v, want ErrAborted", err)
		}
	})

	t.Run("replaces a registered project when the user agrees", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.configs.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.configs.projects["shop"].Root().String(); got != "/srv/shop" {
			t.Fatalf("got root %s, want the new project's /srv/shop", got)
		}
	})

	t.Run("keeps the setup and teardown commands of the project registered under the name", func(t *testing.T) {
		s := newFreezeSetup(t)
		up, down := command(t, "docker compose up -d"), command(t, "docker compose down")
		s.configs.projects["shop"] = shop(t).SettingUp(up).TearingDown(down)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.form.draft; len(got.Setup) != 1 || got.Setup[0] != up || len(got.Teardown) != 1 || got.Teardown[0] != down {
			t.Fatalf("got setup %v, teardown %v; want the registered shop's", got.Setup, got.Teardown)
		}
	})

	t.Run("starts a new project without setup or teardown commands", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.form.draft; len(got.Setup)+len(got.Teardown) != 0 {
			t.Fatalf("got setup %v, teardown %v; want none", got.Setup, got.Teardown)
		}
	})

	t.Run("starts from a running session's windows", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(s.form.draft.Windows) != 2 || s.form.draft.Windows[1].Name().String() != "logs" {
			t.Fatalf("got %d windows, want the session's shop and logs", len(s.form.draft.Windows))
		}
	})

	t.Run("keeps captured directories under a root written through a symlink", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.workspace.links = map[string]string{"/home/me/shop": "/srv/shop"}
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/home/me/shop", Session: "resolved"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := s.form.draft.Windows[0].Panes()[0].Dir(); got.String() != "/home/me/shop/api" {
			t.Fatalf("got dir %q, want /home/me/shop/api", got)
		}
	})

	t.Run("suggests the session's name, keeping the working directory as root", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store-dev" || s.form.draft.Root != "/srv/shop" {
			t.Fatalf("got name %q and root %q, want store-dev and /srv/shop", s.form.draft.Name, s.form.draft.Root)
		}
	})

	t.Run("prefers the given name to the session's", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{Name: "store", WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store" {
			t.Fatalf("got name %q, want store", s.form.draft.Name)
		}
	})

	t.Run("suggests the root's name when the session's isn't a valid project name", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop", Session: "my.session"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "shop" {
			t.Fatalf("got name %q, want shop", s.form.draft.Name)
		}
	})

	t.Run("reports a session it can't read, before the form", func(t *testing.T) {
		s := newFreezeSetup(t)
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop", Session: "missing"}); err == nil {
			t.Fatal("got no error, want the missing session reported")
		}
		if s.form.draft.Root != "" {
			t.Fatal("the form ran, want it skipped")
		}
	})

	t.Run("freezes the session it runs in", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.current = "store-dev"
		if _, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop", Session: app.CurrentSession}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store-dev" || len(s.form.draft.Windows) != 2 {
			t.Fatalf("got name %q and %d windows, want store-dev's name and 2 windows", s.form.draft.Name, len(s.form.draft.Windows))
		}
	})

	t.Run("can't freeze the current session outside tmux", func(t *testing.T) {
		s := newFreezeSetup(t)
		_, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop", Session: app.CurrentSession})
		if !errors.Is(err, domain.ErrNotInsideTmux) {
			t.Fatalf("got %v, want ErrNotInsideTmux", err)
		}
		if s.form.draft.Root != "" {
			t.Fatal("the form ran, want it skipped")
		}
	})

	t.Run("saves nothing when the user cancels the form", func(t *testing.T) {
		s := newFreezeSetup(t)
		s.form.err = app.ErrAborted
		_, err := s.freeze(t, answer(true), app.FreezeRequest{WorkingDir: "/srv/shop"})
		if !errors.Is(err, app.ErrAborted) || len(s.configs.projects) != 0 {
			t.Fatalf("got %v with %d projects saved, want ErrAborted and nothing saved", err, len(s.configs.projects))
		}
	})
}
