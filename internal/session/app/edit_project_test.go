package app_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// storedDocuments is an in-memory app.ProjectDocuments, as YAML files,
// where "invalid" is the only invalid document and a document holding
// "#" is commented.
type storedDocuments map[string]string

func (d storedDocuments) Commented(root domain.ProjectRoot) (bool, error) {
	return strings.Contains(d[filepath.Base(root.String())], "#"), nil
}

func (d storedDocuments) Document(root domain.ProjectRoot) (app.ConfigDocument, error) {
	content, ok := d[filepath.Base(root.String())]
	if !ok {
		return app.ConfigDocument{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, root)
	}
	return app.ConfigDocument{File: root.String() + "/weaver.yml", Content: []byte(content)}, nil
}

func (d storedDocuments) ReplaceDocument(root domain.ProjectRoot, content []byte) error {
	if string(content) == "invalid" {
		return domain.ErrInvalidConfig
	}
	d[filepath.Base(root.String())] = string(content)
	return nil
}

// ConfigAt finds no config: binding starts from a blank project.
func (d storedDocuments) ConfigAt(root domain.ProjectRoot, _ string) (domain.Project, app.ConfigDocument, error) {
	return domain.Project{}, app.ConfigDocument{}, fmt.Errorf("%w: in %s", domain.ErrMissingConfig, root)
}

// scriptedEditor is an app.ConfigEditor saving the next scripted text on
// each edit, and recording the file and content each edit opened.
type scriptedEditor struct {
	saves  []string
	files  []string
	opened []string
	err    error
}

func (e *scriptedEditor) Edit(document app.ConfigDocument) ([]byte, error) {
	e.files = append(e.files, document.File)
	e.opened = append(e.opened, string(document.Content))
	if e.err != nil {
		return nil, e.err
	}
	save := e.saves[0]
	e.saves = e.saves[1:]
	return []byte(save), nil
}

// countingConfirmer is an app.Confirmer recording how often it asked.
type countingConfirmer struct {
	answer bool
	asked  int
}

func (c *countingConfirmer) Confirm(string) (bool, error) {
	c.asked++
	return c.answer, nil
}

func TestEditProject(t *testing.T) {
	// edit runs the form on shop, stored with documents.
	edit := func(t *testing.T, documents storedDocuments, form *fakeForm, confirm app.Confirmer) (inMemoryRegistry, error) {
		t.Helper()
		registry := inMemoryRegistry{projects: map[string]domain.Project{"shop": shop(t)}}
		return registry, app.NewEditProject(registry, documents, form, &scriptedEditor{}, confirm).Handle(rootOf(t, "shop"))
	}
	changed := shop(t).WithClearing(domain.ClearingOn)

	t.Run("saves the project the form returns", func(t *testing.T) {
		registry, err := edit(t, storedDocuments{"shop": "root: /srv/shop"}, &fakeForm{project: changed}, answer(true))
		if err != nil || registry.projects["shop"].Clearing() != domain.ClearingOn {
			t.Fatalf("got %v, clearing %v; want the change saved", err, registry.projects["shop"].Clearing())
		}
	})

	t.Run("starts the form from the registered project", func(t *testing.T) {
		env, err := domain.NewEnv(map[string]string{"APP_ENV": "local"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		nvm, err := domain.NewCommand("nvm use 20")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		registry := inMemoryRegistry{projects: map[string]domain.Project{"shop": shop(t).WithEnv(env).RunningFirst(nvm).WithClearing(domain.ClearingOn)}}
		form := &fakeForm{project: changed}
		documents := storedDocuments{"shop": "root: /srv/shop"}
		if err := app.NewEditProject(registry, documents, form, &scriptedEditor{}, answer(true)).Handle(rootOf(t, "shop")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		draft := form.draft
		if draft.Name != "shop" || draft.Root != "/srv/shop" || len(draft.Windows) != 2 ||
			len(draft.Before) != 1 || len(draft.Env.Vars()) != 1 || draft.Clearing != domain.ClearingOn {
			t.Fatalf("got draft %+v, want shop's project", draft)
		}
	})

	t.Run("gives the form the setup and teardown commands to keep", func(t *testing.T) {
		up, down := command(t, "docker compose up -d"), command(t, "docker compose down")
		registry := inMemoryRegistry{projects: map[string]domain.Project{"shop": shop(t).SettingUp(up).TearingDown(down)}}
		form := &fakeForm{project: changed}
		documents := storedDocuments{"shop": "root: /srv/shop"}
		if err := app.NewEditProject(registry, documents, form, &scriptedEditor{}, answer(true)).Handle(rootOf(t, "shop")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := form.draft; len(got.Setup) != 1 || got.Setup[0] != up || len(got.Teardown) != 1 || got.Teardown[0] != down {
			t.Fatalf("got setup %v, teardown %v; want shop's", got.Setup, got.Teardown)
		}
	})

	t.Run("gives the form the config as stored, to show what saving changes", func(t *testing.T) {
		form := &fakeForm{project: changed}
		if _, err := edit(t, storedDocuments{"shop": "root: /srv/shop"}, form, answer(true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(form.draft.Config); got != "root: /srv/shop" {
			t.Fatalf("got config %q, want the stored one", got)
		}
	})

	t.Run("saves nothing when the user leaves the form", func(t *testing.T) {
		registry, err := edit(t, storedDocuments{"shop": "root: /srv/shop"}, &fakeForm{err: app.ErrAborted}, answer(true))
		if !errors.Is(err, app.ErrAborted) || registry.projects["shop"].Clearing() != domain.ClearingUnset {
			t.Fatalf("got %v, clearing %v; want ErrAborted and shop unchanged", err, registry.projects["shop"].Clearing())
		}
	})

	t.Run("asks before dropping a config's comments, and stops on no", func(t *testing.T) {
		confirm := &countingConfirmer{answer: false}
		form := &fakeForm{project: changed}
		_, err := edit(t, storedDocuments{"shop": "# mine\nroot: /srv/shop"}, form, confirm)
		if !errors.Is(err, app.ErrAborted) || confirm.asked != 1 || form.draft.Name != "" {
			t.Fatalf("got %v after %d questions, form opened on %q; want ErrAborted after one, no form", err, confirm.asked, form.draft.Name)
		}
	})

	t.Run("saves a commented config once the user goes on", func(t *testing.T) {
		registry, err := edit(t, storedDocuments{"shop": "# mine\nroot: /srv/shop"}, &fakeForm{project: changed}, answer(true))
		if err != nil || registry.projects["shop"].Clearing() != domain.ClearingOn {
			t.Fatalf("got %v, clearing %v; want the change saved", err, registry.projects["shop"].Clearing())
		}
	})

	t.Run("doesn't ask about a config without comments", func(t *testing.T) {
		confirm := &countingConfirmer{answer: true}
		if _, err := edit(t, storedDocuments{"shop": "root: /srv/shop"}, &fakeForm{project: changed}, confirm); err != nil || confirm.asked != 0 {
			t.Fatalf("got %v after %d questions, want none", err, confirm.asked)
		}
	})

	t.Run("saves a project the form renamed, at its root", func(t *testing.T) {
		renamed := domain.NewProject(projectName(t, "store"), shop(t).Root())
		registry, err := edit(t, storedDocuments{"shop": "root: /srv/shop"}, &fakeForm{project: renamed}, answer(true))
		if _, ok := registry.projects["store"]; err != nil || !ok || len(registry.projects) != 1 {
			t.Fatalf("got %v with %d configs, want shop saved as store", err, len(registry.projects))
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		err := app.NewEditProject(inMemoryRegistry{projects: map[string]domain.Project{}}, storedDocuments{},
			&fakeForm{}, &scriptedEditor{}, answer(true)).Handle(rootOf(t, "shop"))
		if !errors.Is(err, domain.ErrProjectNotFound) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})
}

func TestEditProjectRaw(t *testing.T) {
	edit := func(documents storedDocuments, editor *scriptedEditor, confirm app.Confirmer) (bool, error) {
		return app.NewEditProject(inMemoryRegistry{}, documents, &fakeForm{}, editor, confirm).HandleRaw(rootOf(t, "shop"))
	}

	t.Run("stores a valid edit", func(t *testing.T) {
		documents := storedDocuments{"shop": "old"}
		changed, err := edit(documents, &scriptedEditor{saves: []string{"new"}}, &countingConfirmer{})
		if err != nil || !changed || documents["shop"] != "new" {
			t.Fatalf("got changed=%v (%v), stored %q; want new stored", changed, err, documents["shop"])
		}
	})

	t.Run("reports an unchanged config without storing it", func(t *testing.T) {
		documents := storedDocuments{"shop": "old"}
		changed, err := edit(documents, &scriptedEditor{saves: []string{"old"}}, &countingConfirmer{})
		if err != nil || changed {
			t.Fatalf("got changed=%v (%v), want unchanged", changed, err)
		}
	})

	t.Run("reopens the editor on what the user wrote to fix an invalid config", func(t *testing.T) {
		documents := storedDocuments{"shop": "old"}
		editor := &scriptedEditor{saves: []string{"invalid", "fixed"}}
		changed, err := edit(documents, editor, &countingConfirmer{answer: false})
		if err != nil || !changed || documents["shop"] != "fixed" {
			t.Fatalf("got changed=%v (%v), stored %q; want fixed stored", changed, err, documents["shop"])
		}
		if want := []string{"old", "invalid"}; !slices.Equal(editor.opened, want) {
			t.Fatalf("the editor opened %q, want %q", editor.opened, want)
		}
	})

	t.Run("opens the stored config's file each time, keeping its format", func(t *testing.T) {
		editor := &scriptedEditor{saves: []string{"invalid", "fixed"}}
		if _, err := edit(storedDocuments{"shop": "old"}, editor, &countingConfirmer{answer: false}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"/srv/shop/weaver.yml", "/srv/shop/weaver.yml"}; !slices.Equal(editor.files, want) {
			t.Fatalf("the editor opened %q, want %q", editor.files, want)
		}
	})

	t.Run("keeps the stored config when the user discards an invalid one", func(t *testing.T) {
		documents := storedDocuments{"shop": "old"}
		changed, err := edit(documents, &scriptedEditor{saves: []string{"invalid"}}, &countingConfirmer{answer: true})
		if !errors.Is(err, domain.ErrInvalidConfig) || changed || documents["shop"] != "old" {
			t.Fatalf("got changed=%v (%v), stored %q; want ErrInvalidConfig and old kept", changed, err, documents["shop"])
		}
	})

	t.Run("stops without asking when the editor fails", func(t *testing.T) {
		confirm := &countingConfirmer{answer: true}
		_, err := edit(storedDocuments{"shop": "old"}, &scriptedEditor{err: errors.New("vi: not found")}, confirm)
		if err == nil || confirm.asked != 0 {
			t.Fatalf("got %v after %d questions, want the editor's error and none", err, confirm.asked)
		}
	})

	t.Run("says nothing was saved when the editor fails", func(t *testing.T) {
		_, err := edit(storedDocuments{"shop": "old"}, &scriptedEditor{err: errors.New("exit status 1")}, &countingConfirmer{})
		if err == nil || !strings.Contains(err.Error(), "nothing was saved") {
			t.Fatalf("got %v, want it to say nothing was saved", err)
		}
	})

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		_, err := edit(storedDocuments{}, &scriptedEditor{}, &countingConfirmer{})
		if !errors.Is(err, domain.ErrProjectNotFound) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})
}
