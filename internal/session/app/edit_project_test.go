package app_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// storedDocuments is an in-memory app.ProjectDocuments, as YAML files,
// where "invalid" is the only invalid document.
type storedDocuments map[string]string

func (d storedDocuments) Document(name domain.ProjectName) (app.ConfigDocument, error) {
	content, ok := d[name.String()]
	if !ok {
		return app.ConfigDocument{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	return app.ConfigDocument{File: name.String() + ".yml", Content: []byte(content)}, nil
}

func (d storedDocuments) ReplaceDocument(name domain.ProjectName, content []byte) error {
	if string(content) == "invalid" {
		return domain.ErrInvalidConfig
	}
	d[name.String()] = string(content)
	return nil
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

	t.Run("opens the stored config's file each time, keeping its format", func(t *testing.T) {
		editor := &scriptedEditor{saves: []string{"invalid", "fixed"}}
		if _, err := edit(storedDocuments{"shop": "old"}, editor, &countingConfirmer{answer: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"shop.yml", "shop.yml"}; !slices.Equal(editor.files, want) {
			t.Fatalf("the editor opened %q, want %q", editor.files, want)
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
