package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// registry is an inMemoryRegistry of projects.
func registry(projects ...domain.Project) inMemoryRegistry {
	configs := inMemoryRegistry{projects: map[string]domain.Project{}}
	for i, p := range projects {
		// Keyed so they list in the order given, sharing a name or not.
		configs.projects[string(rune('a'+i))+p.Name().String()] = p
	}
	return configs
}

func TestFindProjectByDirectory(t *testing.T) {
	found := func(t *testing.T, workspace *fakeWorkspace, arg string, projects ...domain.Project) string {
		t.Helper()
		ref, err := app.NewFindProject(registry(projects...), workspace).Handle(arg, "/srv")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return ref.Name().String()
	}

	t.Run("finds the project whose root contains the directory", func(t *testing.T) {
		if got := found(t, everything(), "/srv/shop/front/src", shop(t), rootedAt(t, "blog", "/srv/blog")); got != "shop" {
			t.Fatalf("got %s, want shop", got)
		}
	})

	t.Run("reads a relative directory from the working directory", func(t *testing.T) {
		if got := found(t, everything(), "./shop/front", shop(t)); got != "shop" {
			t.Fatalf("got %s, want shop", got)
		}
	})

	t.Run("prefers the innermost root when roots nest", func(t *testing.T) {
		if got := found(t, everything(), "/srv/shop/back/app", shop(t), rootedAt(t, "shop-back", "/srv/shop/back")); got != "shop-back" {
			t.Fatalf("got %s, want shop-back", got)
		}
	})

	t.Run("finds the project whose root a symlinked directory leads into", func(t *testing.T) {
		workspace := everything()
		workspace.links = map[string]string{"/home/me/shop/front": "/srv/shop/front"}
		if got := found(t, workspace, "/home/me/shop/front", shop(t)); got != "shop" {
			t.Fatalf("got %s, want shop", got)
		}
	})

	t.Run("finds the project whose root is written through a symlink", func(t *testing.T) {
		workspace := everything()
		workspace.links = map[string]string{"/home/me/blog": "/srv/blog"}
		if got := found(t, workspace, "/srv/blog/posts", rootedAt(t, "blog", "/home/me/blog")); got != "blog" {
			t.Fatalf("got %s, want blog", got)
		}
	})

	t.Run("prefers the innermost root when only one is written through a symlink", func(t *testing.T) {
		workspace := everything()
		workspace.links = map[string]string{"/home/me/projects/shop": "/srv/shop", "/home/me/projects/shop/front": "/srv/shop/front"}
		got := found(t, workspace, "/home/me/projects/shop/front",
			rootedAt(t, "outer", "/home/me/projects/shop"), rootedAt(t, "inner", "/srv/shop/front"))
		if got != "inner" {
			t.Fatalf("got %s, want inner", got)
		}
	})

	t.Run("finds the project a symlink inside it leads out of", func(t *testing.T) {
		workspace := everything()
		workspace.links = map[string]string{"/srv/shop/vendor": "/opt/vendor"}
		if got := found(t, workspace, "/srv/shop/vendor", shop(t)); got != "shop" {
			t.Fatalf("got %s, want shop", got)
		}
	})

	t.Run("finds the project a symlink leads into over the one it is in", func(t *testing.T) {
		workspace := everything()
		workspace.links = map[string]string{"/srv/shop/data": "/srv/big/data"}
		if got := found(t, workspace, "/srv/shop/data", shop(t), rootedAt(t, "big", "/srv/big")); got != "big" {
			t.Fatalf("got %s, want big", got)
		}
	})

	t.Run("reports a directory no project contains, saying how to register it", func(t *testing.T) {
		_, err := app.NewFindProject(registry(shop(t)), everything()).Handle("/home/me", "/")
		if !errors.Is(err, app.ErrNoProjectAt) || !strings.Contains(err.Error(), "weave bind") {
			t.Fatalf("got %v, want ErrNoProjectAt suggesting weave bind", err)
		}
	})
}

func TestFindProjectByName(t *testing.T) {
	t.Run("finds the one project with the name", func(t *testing.T) {
		ref, err := app.NewFindProject(registry(shop(t), project(t, "blog")), everything()).Handle("blog", "/")
		if err != nil || ref.Root().String() != "/srv/blog" {
			t.Fatalf("got %v, %v; want blog at /srv/blog", ref.Root(), err)
		}
	})

	t.Run("finds a project named after a command", func(t *testing.T) {
		ref, err := app.NewFindProject(registry(project(t, "kill")), everything()).Handle("kill", "/")
		if err != nil || ref.Name().String() != "kill" {
			t.Fatalf("got %v, %v; want kill", ref, err)
		}
	})

	t.Run("names the roots of projects sharing the name", func(t *testing.T) {
		configs := registry(rootedAt(t, "api", "/srv/a/api"), rootedAt(t, "api", "/srv/b/api"))
		_, err := app.NewFindProject(configs, everything()).Handle("api", "/")
		if !errors.Is(err, domain.ErrAmbiguousProject) || !strings.Contains(err.Error(), "/srv/a/api and /srv/b/api") {
			t.Fatalf("got %v, want ErrAmbiguousProject naming both roots", err)
		}
	})

	t.Run("reports a name no project has", func(t *testing.T) {
		_, err := app.NewFindProject(registry(shop(t)), everything()).Handle("blog", "/")
		if !errors.Is(err, domain.ErrProjectNotFound) {
			t.Fatalf("got %v, want ErrProjectNotFound", err)
		}
	})
}
