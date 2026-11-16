package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// finding is the use case finding projects among r.
func finding(r registered) app.FindProject {
	return app.NewFindProject(r, infra.NewLocalWorkspace())
}

// `weave .` opens the project holding the current directory: "." is no
// project's name.
func TestDotOpensTheProjectHoldingTheCurrentDirectory(t *testing.T) {
	root := newRootCommand(application{findProject: finding(registered{})})
	root.SetArgs([]string{"."})
	root.SetErr(io.Discard)
	if err := root.Execute(); !errors.Is(err, app.ErrNoProjectAt) {
		t.Fatalf("got %v, want ErrNoProjectAt from looking up the current directory", err)
	}
}

// `weave --detached` alone has no project to open: it doesn't fall back
// to the browser.
func TestStartNeedsAProject(t *testing.T) {
	root := newRootCommand(application{})
	root.SetArgs([]string{"--detached"})
	root.SetErr(io.Discard)
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "needs a project") {
		t.Fatalf("got %v, want --detached refused without a project", err)
	}
}

// foreignSession is a fake domain.SessionLauncher finding every project's
// session running, started otherwise, and failing to do anything else.
type foreignSession struct{}

func (foreignSession) Find(domain.ProjectRef) (domain.RunningSession, bool, error) {
	return domain.NewRunningSession(false), true, nil
}

func (foreignSession) Attach(domain.ProjectRef) error {
	return errors.New("attached")
}

func (foreignSession) Start(domain.SessionPlan) error {
	return errors.New("started")
}

func (foreignSession) Restore(domain.ProjectRef, []domain.WindowPlacement) error {
	return errors.New("restored")
}

// started runs weave with args against the projects names, every one
// running, and returns what it printed.
func started(t *testing.T, names registered, args ...string) string {
	t.Helper()
	root := newRootCommand(application{
		findProject: finding(names),
		openProject: app.NewOpenProject(names, foreignSession{}, nil, nil),
	})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out.String()
}

// `weave -d shop` prints what it did, and doesn't attach.
func TestStartPrintsWhatItDid(t *testing.T) {
	if got := started(t, registered{"shop"}, "-d", "shop"); got != "shop is already running.\n" {
		t.Fatalf("got %q, want shop already running", got)
	}
}

// `weave open kill` reaches the project kill, which `weave kill` doesn't.
func TestOpenReachesAProjectNamedAfterACommand(t *testing.T) {
	if got := started(t, registered{"kill"}, "open", "-d", "kill"); got != "kill is already running.\n" {
		t.Fatalf("got %q, want kill already running", got)
	}
}

// `weave -d /srv/shop/api` starts the project whose root holds the
// directory.
func TestStartFindsTheProjectHoldingADirectory(t *testing.T) {
	if got := started(t, registered{"shop"}, "-d", "/srv/shop/api"); got != "shop is already running.\n" {
		t.Fatalf("got %q, want shop already running", got)
	}
}
