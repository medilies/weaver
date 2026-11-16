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
	adopted *[]string // the sessions renamed, each as "session › session name"
}

func (c capturedSessions) Adopt(session string, project domain.ProjectRef) error {
	usual, _ := project.SessionNames()
	if c.adopted != nil {
		*c.adopted = append(*c.adopted, session+" › "+usual)
	}
	return nil
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

// configsAt is an app.ProjectDocuments over configs, whose projects'
// configs are stored as "stored", recording the config asked for.
type configsAt struct {
	inMemoryRegistry
	asked *string
}

func (c configsAt) ConfigAt(root domain.ProjectRoot, config string) (domain.Project, app.ConfigDocument, error) {
	*c.asked = config
	project, err := c.Find(root)
	if err != nil {
		return domain.Project{}, app.ConfigDocument{}, fmt.Errorf("%w: in %s", domain.ErrMissingConfig, root)
	}
	return project, app.ConfigDocument{File: root.String() + "/weaver.yml", Content: []byte("stored")}, nil
}

func (c configsAt) Document(domain.ProjectRoot) (app.ConfigDocument, error) {
	return app.ConfigDocument{}, nil
}

func (c configsAt) ReplaceDocument(domain.ProjectRoot, []byte) error { return nil }

func (c configsAt) Commented(domain.ProjectRoot) (bool, error) { return false, nil }

type bindSetup struct {
	configs   inMemoryRegistry
	form      *fakeForm
	current   string // the session weaver runs in; none outside tmux
	workspace *fakeWorkspace
	adopted   *[]string
	asked     *string // the config the documents were asked for
}

func newBindSetup(t *testing.T) bindSetup {
	t.Helper()
	return bindSetup{
		configs:   inMemoryRegistry{projects: map[string]domain.Project{}},
		form:      &fakeForm{project: shop(t)},
		workspace: &fakeWorkspace{},
		adopted:   &[]string{},
		asked:     new(string),
	}
}

func (s bindSetup) bind(t *testing.T, request app.BindRequest) (domain.Project, error) {
	t.Helper()
	sessions := capturedSessions{
		windows: map[string][]domain.Window{"store-dev": shop(t).Windows(), "resolved": resolvedShop(t)},
		current: s.current,
		adopted: s.adopted,
	}
	return app.NewBindProject(s.configs, configsAt{s.configs, s.asked}, sessions, s.workspace, s.form).Handle(request)
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
	t.Run("starts from the working directory, named after it", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/shop" || s.form.draft.Name != "shop" || s.form.draft.Config != nil {
			t.Fatalf("got root %q, name %q and config %q; want /srv/shop, shop and none", s.form.draft.Root, s.form.draft.Name, s.form.draft.Config)
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

	t.Run("saves the project the user described", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, saved := s.configs.projects["shop"]; !saved {
			t.Fatal("got nothing saved, want shop")
		}
	})

	t.Run("starts from the config already in the directory, to show what saving changes", func(t *testing.T) {
		s := newBindSetup(t)
		up, down := command(t, "docker compose up -d"), command(t, "docker compose down")
		s.configs.projects["shop"] = shop(t).SettingUp(up).TearingDown(down)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Config: "tmux/dev.yml"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := s.form.draft
		if string(got.Config) != "stored" || *s.asked != "tmux/dev.yml" || len(got.Windows) != 2 {
			t.Fatalf("got config %q, asked for %q, %d windows; want the stored tmux/dev.yml's", got.Config, *s.asked, len(got.Windows))
		}
		if len(got.Setup) != 1 || got.Setup[0] != up || len(got.Teardown) != 1 || got.Teardown[0] != down {
			t.Fatalf("got setup %v, teardown %v; want the config's", got.Setup, got.Teardown)
		}
	})

	t.Run("tells the form whether saving registers the root", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil || !s.form.draft.Unregistered {
			t.Fatalf("got %v, unregistered=%v; want a new root unregistered", err, s.form.draft.Unregistered)
		}
		s.configs.projects["shop"] = shop(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil || s.form.draft.Unregistered {
			t.Fatalf("got %v, unregistered=%v; want a registered root", err, s.form.draft.Unregistered)
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

	t.Run("starts from a running session's windows, keeping the config's commands", func(t *testing.T) {
		s := newBindSetup(t)
		up := command(t, "docker compose up -d")
		s.configs.projects["shop"] = project(t, "shop").SettingUp(up)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.form.draft; len(got.Windows) != 2 || got.Windows[1].Name().String() != "logs" || len(got.Setup) != 1 {
			t.Fatalf("got %d windows and setup %v, want the session's shop and logs, and the config's setup", len(got.Windows), got.Setup)
		}
	})

	t.Run("renames the session it started from after the project", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: "store-dev"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := *s.adopted; len(got) != 1 || got[0] != "store-dev › w/shop" {
			t.Fatalf("got %q renamed, want store-dev to w/shop", got)
		}
	})

	t.Run("renames no session when it started from none", func(t *testing.T) {
		s := newBindSetup(t)
		if _, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop"}); err != nil || len(*s.adopted) != 0 {
			t.Fatalf("got %v and %q renamed, want nothing renamed", err, *s.adopted)
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
		if len(s.form.draft.Windows) != 2 || len(*s.adopted) != 1 {
			t.Fatalf("got %d windows and %q renamed, want store-dev's 2 windows, and it renamed", len(s.form.draft.Windows), *s.adopted)
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
		_, err := s.bind(t, app.BindRequest{WorkingDir: "/srv/shop", Session: "store-dev"})
		if !errors.Is(err, app.ErrAborted) || len(s.configs.projects) != 0 || len(*s.adopted) != 0 {
			t.Fatalf("got %v with %d projects saved and %q renamed, want ErrAborted and nothing done", err, len(s.configs.projects), *s.adopted)
		}
	})
}
