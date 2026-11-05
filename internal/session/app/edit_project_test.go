package app_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// storedDocuments is an in-memory app.ProjectDocuments where "invalid"
// is the only invalid document.
type storedDocuments map[string]string

func (d storedDocuments) Document(name domain.ProjectName) ([]byte, error) {
	document, ok := d[name.String()]
	if !ok {
		return nil, domain.NewProjectNotFound(name)
	}
	return []byte(document), nil
}

func (d storedDocuments) ReplaceDocument(name domain.ProjectName, document []byte) error {
	if string(document) == "invalid" {
		return domain.ErrInvalidConfig
	}
	d[name.String()] = string(document)
	return nil
}

// scriptedEditor is an app.ConfigEditor saving the next scripted text on
// each edit, and recording what each edit opened.
type scriptedEditor struct {
	saves  []string
	opened []string
	err    error
}

func (e *scriptedEditor) Edit(_ domain.ProjectName, document []byte) ([]byte, error) {
	e.opened = append(e.opened, string(document))
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
	edit := func(documents storedDocuments, editor *scriptedEditor, confirm app.Confirmer) (bool, error) {
		return app.NewEditProject(documents, editor, confirm).Handle("shop")
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
		changed, err := edit(documents, editor, &countingConfirmer{answer: true})
		if err != nil || !changed || documents["shop"] != "fixed" {
			t.Fatalf("got changed=%v (%v), stored %q; want fixed stored", changed, err, documents["shop"])
		}
		if want := []string{"old", "invalid"}; !slices.Equal(editor.opened, want) {
			t.Fatalf("the editor opened %q, want %q", editor.opened, want)
		}
	})

	t.Run("keeps the stored config when the user gives up on an invalid one", func(t *testing.T) {
		documents := storedDocuments{"shop": "old"}
		changed, err := edit(documents, &scriptedEditor{saves: []string{"invalid"}}, &countingConfirmer{answer: false})
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

	t.Run("reports a project that isn't registered", func(t *testing.T) {
		_, err := edit(storedDocuments{}, &scriptedEditor{}, &countingConfirmer{})
		if _, ok := errors.AsType[domain.ProjectNotFound](err); !ok {
			t.Fatalf("got %v, want ProjectNotFound", err)
		}
	})
}
