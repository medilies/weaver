package app_test

import (
	"errors"
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

// foundCheckouts is a fake domain.CheckoutDetector.
type foundCheckouts struct{ checkouts domain.Checkouts }

func (f foundCheckouts) Detect(domain.ProjectRoot) (domain.Checkouts, error) {
	return f.checkouts, nil
}

// installedRuntimes is a fake domain.RuntimeCatalog.
type installedRuntimes []domain.Runtime

func (i installedRuntimes) Available() ([]domain.Runtime, error) {
	return i, nil
}

type addSetup struct {
	repo  inMemoryRepository
	form  *fakeForm
	found domain.Checkouts
	php   domain.Runtime
}

func newAddSetup(t *testing.T) addSetup {
	t.Helper()
	backRepo, err := domain.NewCheckout("back", "git@host:back.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found, err := domain.NewCheckouts(backRepo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	php, err := domain.NewRuntime("php", "8.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return addSetup{
		repo:  inMemoryRepository{projects: map[string]domain.Project{}},
		form:  &fakeForm{project: shop(t)},
		found: found,
		php:   php,
	}
}

func (s addSetup) add(confirm app.Confirmer, request app.AddRequest) (domain.Project, error) {
	return app.NewAddProject(s.repo, foundCheckouts{s.found}, installedRuntimes{s.php}, s.form, confirm).Handle(request)
}

func TestAddProject(t *testing.T) {
	t.Run("starts from the working directory when no root is given", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/shop" || s.form.draft.Name != "shop" {
			t.Fatalf("got root %q and name %q, want /srv/shop and shop", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("resolves a relative root against the working directory", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(answer(true), app.AddRequest{Root: "../blog", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Root != "/srv/blog" || s.form.draft.Name != "blog" {
			t.Fatalf("got root %q and name %q, want /srv/blog and blog", s.form.draft.Root, s.form.draft.Name)
		}
	})

	t.Run("suggests the name given", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(answer(true), app.AddRequest{Name: "store", WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.form.draft.Name != "store" {
			t.Fatalf("got name %q, want store", s.form.draft.Name)
		}
	})

	t.Run("offers the repositories found under the root", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if list := s.form.draft.Checkouts.List(); len(list) != 1 || list[0].Dir() != "back" {
			t.Fatalf("got %v, want the back repository", list)
		}
	})

	t.Run("offers the installed versions", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.form.draft.Runtimes; len(got) != 1 || got[0] != s.php {
			t.Fatalf("got %v, want php 8.4", got)
		}
	})

	t.Run("saves the project the user described", func(t *testing.T) {
		s := newAddSetup(t)
		if _, err := s.add(answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, saved := s.repo.projects["shop"]; !saved {
			t.Fatal("got nothing saved, want shop")
		}
	})

	t.Run("keeps a registered project the user won't replace", func(t *testing.T) {
		s := newAddSetup(t)
		existing := rootedAt(t, "shop", "/srv/old-shop")
		s.repo.projects["shop"] = existing
		_, err := s.add(answer(false), app.AddRequest{WorkingDir: "/srv/shop"})
		if !errors.Is(err, app.ErrAborted) || s.repo.projects["shop"].Root() != existing.Root() {
			t.Fatalf("got %v, root %s; want ErrAborted and the old project kept", err, s.repo.projects["shop"].Root())
		}
	})

	t.Run("replaces a registered project when the user agrees", func(t *testing.T) {
		s := newAddSetup(t)
		s.repo.projects["shop"] = rootedAt(t, "shop", "/srv/old-shop")
		if _, err := s.add(answer(true), app.AddRequest{WorkingDir: "/srv/shop"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := s.repo.projects["shop"].Root().String(); got != "/srv/shop" {
			t.Fatalf("got root %s, want the new project's /srv/shop", got)
		}
	})

	t.Run("saves nothing when the user cancels the form", func(t *testing.T) {
		s := newAddSetup(t)
		s.form.err = app.ErrAborted
		_, err := s.add(answer(true), app.AddRequest{WorkingDir: "/srv/shop"})
		if !errors.Is(err, app.ErrAborted) || len(s.repo.projects) != 0 {
			t.Fatalf("got %v with %d projects saved, want ErrAborted and nothing saved", err, len(s.repo.projects))
		}
	})
}
