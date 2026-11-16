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
		if got, err := edit(t, "", "sed -i.bak s/shop/store/"); err != nil || got != "root: /srv/store" {
			t.Fatalf("got %q (%v), want the root renamed", got, err)
		}
	})

	t.Run("prefers $VISUAL to $EDITOR", func(t *testing.T) {
		if got, err := edit(t, "sed -i.bak s/shop/visual/", "sed -i.bak s/shop/editor/"); err != nil || got != "root: /srv/visual" {
			t.Fatalf("got %q (%v), want $VISUAL's edit", got, err)
		}
	})

	t.Run("names the copy like the stored config", func(t *testing.T) {
		nameCheck := `f() { case "$1" in */shop.yml) sed -i.bak s/shop/named/ "$1";; esac; }; f`
		if got, err := edit(t, "", nameCheck); err != nil || got != "root: /srv/named" {
			t.Fatalf("got %q (%v), want the copy named shop.yml", got, err)
		}
	})

	t.Run("prefers the settings' editor to $VISUAL", func(t *testing.T) {
		t.Setenv("VISUAL", "sed -i.bak s/shop/visual/")
		preferred := func() string { return "sed -i.bak s/shop/settings/" }
		edited, err := infra.NewEditor().Preferring(preferred).Edit(app.ConfigDocument{File: "shop.yml", Content: []byte("root: /srv/shop")})
		if err != nil || string(edited) != "root: /srv/settings" {
			t.Fatalf("got %q (%v), want the settings' editor's edit", edited, err)
		}
	})

	t.Run("reports an editor that fails", func(t *testing.T) {
		if _, err := edit(t, "", "false"); err == nil {
			t.Fatal("got no error, want the editor's failure")
		}
	})
}
