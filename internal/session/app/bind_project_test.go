package app_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// fakeForm is an app.ProjectForm that records the draft it was given
// and returns a set project, or an error.
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

type bindSetup struct {
	configs   inMemoryRegistry
	form      *fakeForm
	current   string // the session weaver runs in; none outside tmux
	workspace *fakeWorkspace
}

func newBindSetup(t *testing.T) bindSetup {
	t.Helper()
	return bindSetup{
		configs:   inMemoryRegistry{projects: map[string]domain.Project{}},
		form:      &fakeForm{project: shop(t)},
		workspace: &fakeWorkspace{},
	}
}

func (s bindSetup) bind(t *testing.T, request app.BindRequest) (domain.Project, error) {
	t.Helper()
	sessions := capturedSessions{
		windows: map[string][]domain.Window{
			"store-dev": shop(t).Windows(), "my.session": shop(t).Windows(), "resolved": resolvedShop(t),
		},
		current: s.current,
	}
	return app.NewBindProject(s.configs, sessions, s.workspace, s.form).Handle(request)
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

func TestBindProject(t *testing.T) {
	t.Run("starts from the working directory when no root is given", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/shop" || s.form.draft.Name != "shop" {
			t.Fatalf("got root %q and name %q, want /srv/shop and shop", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("resolves a relative root against the working directory", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{Root: "../blog", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/blog" || s.form.draft.Name != "blog" {
			t.Fatalf("got root %q and name %q, want /srv/blog and blog", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("suggests the name given", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{Name: "store", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store" {
			t.Fatalf("got name %q, want store", s.form.draft.Name)
		}
	})

	t.Run("has the name typed in the form", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.NameStays {
			t.Fatal("got the name kept, want it typed")
		}
	})

	t.Run("saves the project the user described", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, saved := s.configs.projects["shop"]; !saved {
			t.Fatal("got nothing saved, want shop")
		}
	})

	t.Run("replaces a registered project with the one the form returns", func(t *testing.T) {
		s := newBindSetup(t)
		s.configs.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.configs.projects["shop"].Root().String(); got != "/srv/shop" {
			t.Fatalf("got root %s, want the new project's /srv/shop", got)
		}
	})

	t.Run("keeps the setup and teardown commands of the project registered under the name", func(t *testing.T) {
		s := newBindSetup(t)
		up, down := command(t, "docker compose up -d"), command(t, "docker compose down")
		s.configs.projects["shop"] = shop(t).SettingUp(up).TearingDown(down)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.form.draft; len(got.Setup) != 1 || got.Setup[0] != up || len(got.Teardown) != 1 || got.Teardown[0] != down {
			t.Fatalf("got setup %v, teardown %v; want the registered shop's", got.Setup, got.Teardown)
		}
	})

	t.Run("starts a new project without setup or teardown commands", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.form.draft; len(got.Setup)+len(got.Teardown) != 0 {
			t.Fatalf("got setup %v, teardown %v; want none", got.Setup, got.Teardown)
		}
	})

	t.Run("starts from a running session's windows", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(s.form.draft.Windows) != 2 || s.form.draft.Windows[1].Name().String() != "logs" {
			t.Fatalf("got %d windows, want the session's shop and logs", len(s.form.draft.Windows))
		}
	})

	t.Run("keeps captured directories under a root written through a symlink", func(t *testing.T) {
		s := newBindSetup(t)
		s.workspace.links = map[string]string{"/home/me/shop": "/srv/shop"}
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/home/me/shop", Session: "resolved"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, _ := s.form.draft.Windows[0].Panes()[0].Dir(); got.String() != "/home/me/shop/api" {
			t.Fatalf("got dir %q, want /home/me/shop/api", got)
		}
	})

	t.Run("suggests the session's name, keeping the working directory as root", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store-dev" || s.form.draft.Root != "/srv/shop" {
			t.Fatalf("got name %q and root %q, want store-dev and /srv/shop", s.form.draft.Name, s.form.draft.Root)
		}
	})

	t.Run("prefers the given name to the session's", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{Name: "store", WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store" {
			t.Fatalf("got name %q, want store", s.form.draft.Name)
		}
	})

	t.Run("suggests the root's name when the session's isn't a valid project name", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: "my.session"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "shop" {
			t.Fatalf("got name %q, want shop", s.form.draft.Name)
		}
	})

	t.Run("reports a session it can't read, before the form", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: "missing"}); err == nil {
			t.Fatal("got no error, want the missing session reported")
		}
		if s.form.draft.Root != "" {
			t.Fatal("the form ran, want it skipped")
		}
	})

	t.Run("binds the session it runs in", func(t *testing.T) {
		s := newBindSetup(t)
		s.current = "store-dev"
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: app.CurrentSession}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store-dev" || len(s.form.draft.Windows) != 2 {
			t.Fatalf("got name %q and %d windows, want store-dev's name and 2 windows", s.form.draft.Name, len(s.form.draft.Windows))
		}
	})

	t.Run("can't bind the current session outside tmux", func(t *testing.T) {
		s := newBindSetup(t)
		_, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: app.CurrentSession})
		if !errors.Is(err, domain.ErrNotInsideTmux) {
			t.Fatalf("got %v, want ErrNotInsideTmux", err)
		}
		if s.form.draft.Root != "" {
			t.Fatal("the form ran, want it skipped")
		}
	})

	t.Run("saves nothing when the user cancels the form", func(t *testing.T) {
		s := newBindSetup(t)
		s.form.err = app.ErrAborted
		_, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"})
		if !errors.Is(err, app.ErrAborted) || len(s.configs.projects) != 0 {
			t.Fatalf("got %v with %d projects saved, want ErrAborted and nothing saved", err, len(s.configs.projects))
		}
	})
}
