package main

import (
	"sync"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// The settings mostly change how the confirmation dialog, the config
// form and the browser look and behave, so they are read the first time
// one of those is shown: invalid settings stop what shows them, not the
// help, the version, completion or a command that never shows them,
// which keeps tmux's default limit.

// builtOnce returns the adapter built from the settings, building it
// the first time it is asked for.
func builtOnce[T any](settings func() (infra.Settings, error), build func(infra.Settings) T) func() (T, error) {
	return sync.OnceValues(func() (T, error) {
		s, err := settings()
		if err != nil {
			var none T
			return none, err
		}
		return build(s), nil
	})
}

type lazyConfirmer func() (infra.TeaConfirmer, error)

func (l lazyConfirmer) Confirm(question string) (bool, error) {
	confirm, err := l()
	if err != nil {
		return false, err
	}
	return confirm.Confirm(question)
}

type lazyConfigForm func() (infra.TeaConfigForm, error)

func (l lazyConfigForm) Fill(draft app.ProjectDraft) (domain.Project, error) {
	form, err := l()
	if err != nil {
		return domain.Project{}, err
	}
	return form.Fill(draft)
}

type lazyBrowser func() (infra.TeaBrowser, error)

func (l lazyBrowser) Browse(projects []app.ProjectOverview, selected domain.ProjectName, notice app.Notice, previews app.LiveSessions) (app.Choice, error) {
	browser, err := l()
	if err != nil {
		return app.Choice{}, err
	}
	return browser.Browse(projects, selected, notice, previews)
}

func (l lazyBrowser) Confirm(question string) (bool, error) {
	browser, err := l()
	if err != nil {
		return false, err
	}
	return browser.Confirm(question)
}

// Working tells nothing when the browser couldn't be built.
func (l lazyBrowser) Working(text string) {
	if browser, err := l(); err == nil {
		browser.Working(text)
	}
}

// Leave has nothing to take off screen when the browser couldn't be built.
func (l lazyBrowser) Leave() error {
	browser, err := l()
	if err != nil {
		return nil //nolint:nilerr // never shown, so never left
	}
	return browser.Leave()
}

var (
	_ app.Confirmer      = lazyConfirmer(nil)
	_ app.ProjectForm    = lazyConfigForm(nil)
	_ app.ProjectBrowser = lazyBrowser(nil)
)
