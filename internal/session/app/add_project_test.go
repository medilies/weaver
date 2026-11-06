package app_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// fakeForm is an app.ProjectForm that records the draft it was given and
// returns a set project, or an error.
type fakeForm struct {
	draft   app.ProjectDraft
	project domain.Project
	err     error
}

func (f *fakeForm) Fill(draft app.ProjectDraft) (domain.Project, error) {
	f.draft = draft
	return f.project, f.err
}

// answer is an app.Confirmer giving the same answer to every question.
type answer bool

func (a answer) Confirm(string) (bool, error) {
	return bool(a), nil
}

// foundRepos is a fake domain.RepoDetector.
type foundRepos struct{ repos domain.Repos }

func (f foundRepos) Detect(domain.ProjectRoot) (domain.Repos, error) {
	return f.repos, nil
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

type addSetup struct {
	configs inMemoryRepository
	form    *fakeForm
	found   domain.Repos
	current string // the session weaver runs in; none outside tmux
}

func newAddSetup(t *testing.T) addSetup {
	t.Helper()
	backRepo, err := domain.NewRepo("back", "git@host:back.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found, err := domain.NewRepos(backRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return addSetup{
		configs: inMemoryRepository{projects: map[string]domain.Project{}},
		form:    &fakeForm{project: shop(t)},
		found:   found,
	}
}

func (s addSetup) add(t *testing.T, confirm app.Confirmer, request app.AddRequest) (domain.Project, error) {
	t.Helper()
	sessions := capturedSessions{
		windows: map[string][]domain.Window{"store-dev": shop(t).Windows(), "my.session": shop(t).Windows()},
		current: s.current,
	}
	return app.NewAddProject(s.configs, foundRepos{s.found}, sessions, s.form, confirm).Handle(request)
}

func TestAddProject(t *testing.T) {
	t.Run("starts from the working directory when no root is given", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/shop" || s.form.draft.Name != "shop" {
			t.Fatalf("got root %q and name %q, want /srv/shop and shop", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("resolves a relative root against the working directory", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{Root: "../blog", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/blog" || s.form.draft.Name != "blog" {
			t.Fatalf("got root %q and name %q, want /srv/blog and blog", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("suggests the name given", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{Name: "store", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store" {
			t.Fatalf("got name %q, want store", s.form.draft.Name)
		}
	})

	t.Run("offers the repositories found under the root", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if list := s.form.draft.Repos.List(); len(list) != 1 || list[0].Dir() != "back" {
			t.Fatalf("got %v, want the back repository", list)
		}
	})

	t.Run("saves the project the user described", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, saved := s.configs.projects["shop"]; !saved {
			t.Fatal("got nothing saved, want shop")
		}
	})

	t.Run("keeps a registered project the user won't replace", func(t *testing.T) {
		s := newAddSetup(t)
		existing := rootedAt(t, "shop", "/srv/old-shop")
		s.configs.projects["shop"] = existing
		_, err := s.add(t, answer(false), app.AddRequest{WorkingDir: "/srv/shop"})
		if !errors.Is(err, app.ErrAborted) || s.configs.projects["shop"].Root() != existing.Root() {
			t.Fatalf("got %v, root %s; want ErrAborted and the old project kept", err, s.configs.projects["shop"].Root())
		}
	})

	t.Run("replaces a registered project when the user agrees", func(t *testing.T) {
		s := newAddSetup(t)
		s.configs.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.configs.projects["shop"].Root().String(); got != "/srv/shop" {
			t.Fatalf("got root %s, want the new project's /srv/shop", got)
		}
	})

	t.Run("starts from a running session's windows", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(s.form.draft.Windows) != 2 || s.form.draft.Windows[1].Name().String() != "logs" {
			t.Fatalf("got %d windows, want the session's shop and logs", len(s.form.draft.Windows))
		}
	})

	t.Run("suggests the session's name, keeping the working directory as root", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store-dev" || s.form.draft.Root != "/srv/shop" {
			t.Fatalf("got name %q and root %q, want store-dev and /srv/shop", s.form.draft.Name, s.form.draft.Root)
		}
	})

	t.Run("prefers the given name to the session's", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{Name: "store", WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store" {
			t.Fatalf("got name %q, want store", s.form.draft.Name)
		}
	})

	t.Run("suggests the root's name when the session's isn't a valid project name", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop", Session: "my.session"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "shop" {
			t.Fatalf("got name %q, want shop", s.form.draft.Name)
		}
	})

	t.Run("reports a session it can't read, before the form", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop", Session: "missing"}); err == nil {
			t.Fatal("got no error, want the missing session reported")
		}
		if s.form.draft.Root != "" {
			t.Fatal("the form ran, want it skipped")
		}
	})

	t.Run("freezes the session it runs in", func(t *testing.T) {
		s := newAddSetup(t)
		s.current = "store-dev"
		if _, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop", Session: app.CurrentSession}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store-dev" || len(s.form.draft.Windows) != 2 {
			t.Fatalf("got name %q and %d windows, want store-dev's name and 2 windows", s.form.draft.Name, len(s.form.draft.Windows))
		}
	})

	t.Run("can't freeze the current session outside tmux", func(t *testing.T) {
		s := newAddSetup(t)
		_, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop", Session: app.CurrentSession})
		if !errors.Is(err, domain.ErrNotInsideTmux) {
			t.Fatalf("got %v, want ErrNotInsideTmux", err)
		}
		if s.form.draft.Root != "" {
			t.Fatal("the form ran, want it skipped")
		}
	})

	t.Run("saves nothing when the user cancels the form", func(t *testing.T) {
		s := newAddSetup(t)
		s.form.err = app.ErrAborted
		_, err := s.add(t, answer(true), app.AddRequest{WorkingDir: "/srv/shop"})
		if !errors.Is(err, app.ErrAborted) || len(s.configs.projects) != 0 {
			t.Fatalf("got %v with %d projects saved, want ErrAborted and nothing saved", err, len(s.configs.projects))
		}
	})
}
