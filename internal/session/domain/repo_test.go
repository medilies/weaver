package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func repo(t *testing.T, dir, remote string) domain.Repo {
	t.Helper()
	r, err := domain.NewRepo(dir, remote)
	if err != nil {
		t.Fatalf("NewRepo(%q, %q): %v", dir, remote, err)
	}
	return r
}

func TestNewRepo(t *testing.T) {
	t.Run("treats a blank directory as the project root", func(t *testing.T) {
		if got := repo(t, "", "git@host:repo.git").Dir(); got != "." {
			t.Fatalf("got %q, want %q", got, ".")
		}
	})

	t.Run("rejects an absolute directory", func(t *testing.T) {
		_, err := domain.NewRepo("/srv/back", "git@host:repo.git")
		if !errors.Is(err, domain.ErrInvalidRepoDir) {
			t.Fatalf("got %v, want ErrInvalidRepoDir", err)
		}
	})

	t.Run("rejects a directory outside the project root", func(t *testing.T) {
		_, err := domain.NewRepo("back/../../elsewhere", "git@host:repo.git")
		if !errors.Is(err, domain.ErrInvalidRepoDir) {
			t.Fatalf("got %v, want ErrInvalidRepoDir", err)
		}
	})

	t.Run("rejects a blank remote", func(t *testing.T) {
		_, err := domain.NewRepo("back", " ")
		if !errors.Is(err, domain.ErrEmptyRemote) {
			t.Fatalf("got %v, want ErrEmptyRemote", err)
		}
	})
}

func TestNewRepos(t *testing.T) {
	t.Run("orders the root first and parents before nested repositories", func(t *testing.T) {
		repos, err := domain.NewRepos(
			repo(t, "front/docs", "git@host:docs.git"),
			repo(t, "back", "git@host:back.git"),
			repo(t, "front", "git@host:front.git"),
			repo(t, ".", "git@host:root.git"),
			repo(t, "-old", "git@host:old.git"), // sorts before "."
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var dirs []string
		for _, r := range repos.List() {
			dirs = append(dirs, r.Dir())
		}
		if want := []string{".", "-old", "back", "front", "front/docs"}; !slices.Equal(dirs, want) {
			t.Fatalf("got %q, want %q", dirs, want)
		}
	})

	t.Run("rejects two repositories in one directory", func(t *testing.T) {
		_, err := domain.NewRepos(
			repo(t, "back", "git@host:back.git"),
			repo(t, "back/", "git@host:other.git"),
		)
		if !errors.Is(err, domain.ErrDuplicateRepo) {
			t.Fatalf("got %v, want ErrDuplicateRepo", err)
		}
	})
}
