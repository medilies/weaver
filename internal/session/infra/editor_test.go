package infra_test

import (
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/infra"
)

func TestEditor(t *testing.T) {
	edit := func(t *testing.T, visual, editor string) (string, error) {
		t.Helper()
		t.Setenv("VISUAL", visual)
		t.Setenv("EDITOR", editor)
		edited, err := infra.NewEditor().Edit(app.ConfigDocument{File: "shop.yml", Content: []byte("root: /srv/shop")})
		return string(edited), err
	}

	t.Run("returns the document as the editor saved it", func(t *testing.T) {
		if got, err := edit(t, "", "sed -i s/shop/store/"); err != nil || got != "root: /srv/store" {
			t.Fatalf("got %q (%v), want the root renamed", got, err)
		}
	})

	t.Run("prefers $VISUAL to $EDITOR", func(t *testing.T) {
		if got, err := edit(t, "sed -i s/shop/visual/", "sed -i s/shop/editor/"); err != nil || got != "root: /srv/visual" {
			t.Fatalf("got %q (%v), want $VISUAL's edit", got, err)
		}
	})

	t.Run("edits next to the JSON Schema the config points to", func(t *testing.T) {
		schemaCheck := `f() { test -f "$(dirname "$1")/weaver.schema.json" && sed -i s/shop/checked/ "$1"; }; f`
		if got, err := edit(t, "", schemaCheck); err != nil || got != "root: /srv/checked" {
			t.Fatalf("got %q (%v), want the schema found next to the document", got, err)
		}
	})

	t.Run("names the copy like the stored config", func(t *testing.T) {
		nameCheck := `f() { case "$1" in */shop.yml) sed -i s/shop/named/ "$1";; esac; }; f`
		if got, err := edit(t, "", nameCheck); err != nil || got != "root: /srv/named" {
			t.Fatalf("got %q (%v), want the copy named shop.yml", got, err)
		}
	})

	t.Run("reports an editor that fails", func(t *testing.T) {
		if _, err := edit(t, "", "false"); err == nil {
			t.Fatal("got no error, want the editor's failure")
		}
	})
}
