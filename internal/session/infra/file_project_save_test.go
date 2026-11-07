package infra_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/medilies/weaver/internal/session/domain"
	"github.com/medilies/weaver/internal/session/infra"
)

// gridDoc splits a pane that isn't the previous one, so its saved form
// needs "split.of".
const gridDoc = `root: /srv/grid
windows:
  - name: main
    panes:
      - name: left
      - name: right
        split: { direction: right }
      - name: under left
        split: { of: left, direction: below }
`

// save saves project into a fresh directory and returns the directory and
// the saved config.
func save(t *testing.T, project domain.Project) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "projects")
	if err := infra.NewFileProjectRepository(dir).Save(project); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doc, err := os.ReadFile(filepath.Join(dir, project.Name().String()+".yml"))
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	return dir, string(doc)
}

func mustLoad(t *testing.T, name, doc string) domain.Project {
	t.Helper()
	project, err := load(t, name, doc)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return project
}

func TestFileProjectRepositorySave(t *testing.T) {
	t.Setenv("HOME", "/home/someone")

	for _, fixture := range []string{"nimbus", "shop"} {
		t.Run("writes the "+fixture+" config back unchanged", func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", fixture+".yml"))
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}
			if _, got := save(t, loadFixture(t, fixture)); got != string(want) {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}

	t.Run("saves a project opening its default window as its root alone", func(t *testing.T) {
		_, got := save(t, mustLoad(t, "weaver", "root: /home/someone/dev/weaver\nwindows: [{name: weaver}]\n"))
		want := "# yaml-language-server: $schema=./weaver.schema.json\nroot: ~/dev/weaver\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps commands to run first in a project's default window", func(t *testing.T) {
		_, got := save(t, mustLoad(t, "weaver", "root: /srv/weaver\nwindows: [{name: weaver, before: [nvm use 20]}]\n"))
		if !strings.Contains(got, "    before:\n      - nvm use 20\n") {
			t.Fatalf("got:\n%s\nwant the default window kept with its before", got)
		}
	})

	t.Run("keeps commands to run first in a window's only pane", func(t *testing.T) {
		_, got := save(t, mustLoad(t, "shop", "root: /srv/shop\nwindows: [{name: repo, panes: [{before: [git fetch]}]}]\n"))
		if !strings.Contains(got, "    panes:\n      - before:\n          - git fetch\n") {
			t.Fatalf("got:\n%s\nwant the pane's before kept", got)
		}
	})

	t.Run("names a split's target only when it isn't the previous pane", func(t *testing.T) {
		_, got := save(t, mustLoad(t, "grid", gridDoc))
		if n := strings.Count(got, "of:"); n != 1 || !strings.Contains(got, "of: left") {
			t.Fatalf("got %d split targets in:\n%s\nwant only the one to left", n, got)
		}
	})

	t.Run("writes pane directories under the root relative to it", func(t *testing.T) {
		_, got := save(t, mustLoad(t, "shop", `root: /srv/shop
windows:
  - name: shop
    panes:
      - name: back
        dir: /srv/shop/back
      - name: logs
        dir: /var/log
        split: { direction: right }
`))
		if !strings.Contains(got, "dir: back\n") || !strings.Contains(got, "dir: /var/log\n") {
			t.Fatalf("got:\n%s\nwant back relative and /var/log as is", got)
		}
	})

	t.Run("keeps shell operators readable", func(t *testing.T) {
		_, got := save(t, mustLoad(t, "ops", `root: /srv/ops
windows:
  - name: ops
    panes:
      - command: npm i && npm run dev > dev.log
`))
		if !strings.Contains(got, "command: npm i && npm run dev > dev.log\n") {
			t.Fatalf("got:\n%s\nwant the command unescaped", got)
		}
	})

	t.Run("replaces the project's config", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "shop.yml"), []byte("root: /srv/old\n"), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		configs := infra.NewFileProjectRepository(dir)
		saved := mustLoad(t, "shop", "root: /srv/shop\n")
		if err := configs.Save(saved); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, err := configs.FindByName(saved.Name()); err != nil || got.Root().String() != "/srv/shop" {
			t.Fatalf("got %v (%v), want the saved project", got.Root(), err)
		}
	})

	t.Run("saves the JSON Schema next to the configs", func(t *testing.T) {
		dir, _ := save(t, mustLoad(t, "grid", gridDoc))
		if _, err := os.Stat(filepath.Join(dir, "weaver.schema.json")); err != nil {
			t.Fatalf("got no schema: %v", err)
		}
	})

	t.Run("lists the saved project, not the schema", func(t *testing.T) {
		dir, _ := save(t, mustLoad(t, "grid", gridDoc))
		names, err := infra.NewFileProjectRepository(dir).Names()
		if err != nil || len(names) != 1 || names[0].String() != "grid" {
			t.Fatalf("got %v (%v), want grid only", names, err)
		}
	})
}

// schemaFor saves a project and returns the schema saved next to it.
func schemaFor(t *testing.T) map[string]any {
	t.Helper()
	dir, _ := save(t, mustLoad(t, "grid", gridDoc))
	raw, err := os.ReadFile(filepath.Join(dir, "weaver.schema.json"))
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("parsing the schema: %v", err)
	}
	return schema
}

// undeclared lists the keys of value, at path, that the schema node
// doesn't declare, resolving "#/definitions/..." references.
func undeclared(schema, node map[string]any, value any, path string) []string {
	if ref, ok := node["$ref"].(string); ok {
		node = schema["definitions"].(map[string]any)[strings.TrimPrefix(ref, "#/definitions/")].(map[string]any)
	}
	var missing []string
	switch v := value.(type) {
	case map[string]any:
		properties, declares := node["properties"].(map[string]any)
		for key, child := range v {
			if !declares {
				continue // a map of names, e.g. env or repositories
			}
			childNode, ok := properties[key].(map[string]any)
			if !ok {
				missing = append(missing, path+"."+key)
				continue
			}
			missing = append(missing, undeclared(schema, childNode, child, path+"."+key)...)
		}
	case []any:
		items, _ := node["items"].(map[string]any)
		for i, child := range v {
			missing = append(missing, undeclared(schema, items, child, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return missing
}

func TestJSONSchema(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	schema := schemaFor(t)

	saved := map[string]domain.Project{
		"nimbus": loadFixture(t, "nimbus"),
		"shop":   loadFixture(t, "shop"),
		"grid":   mustLoad(t, "grid", gridDoc),
	}
	for name, project := range saved {
		t.Run("declares every field of the saved "+name+" config", func(t *testing.T) {
			_, doc := save(t, project)
			var value any
			if err := yaml.Unmarshal([]byte(doc), &value); err != nil {
				t.Fatalf("parsing the saved config: %v", err)
			}
			if missing := undeclared(schema, schema, value, "$"); len(missing) != 0 {
				t.Fatalf("the schema doesn't declare %q", missing)
			}
		})
	}

}
