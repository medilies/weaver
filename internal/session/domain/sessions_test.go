package domain_test

import (
	"testing"

	"github.com/medilies/weaver/internal/session/domain"
)

func ref(t *testing.T, root, name string) domain.ProjectRef {
	t.Helper()
	return domain.NewProjectRef(projectRoot(t, root), projectName(t, name))
}

func TestSessionNamesArePrefixed(t *testing.T) {
	usual, apart := ref(t, "/home/me/dev/api", "api").SessionNames()
	if usual != "w/api" || apart != "w/home/me/dev/api" {
		t.Fatalf("got %q and %q, want w/api and w/home/me/dev/api", usual, apart)
	}
}

func TestSessionNamesWriteWhatTmuxRewritesAsItDoes(t *testing.T) {
	usual, apart := ref(t, "/srv/weaver.nvim", "weaver.nvim").SessionNames()
	if usual != "w/weaver_nvim" || apart != "w/srv/weaver_nvim" {
		t.Fatalf("got %q and %q, want w/weaver_nvim and w/srv/weaver_nvim", usual, apart)
	}
}

func TestSessionAmongPrefersTheOneWeaverStartedForTheRoot(t *testing.T) {
	sessions := []domain.ListedSession{
		domain.NewListedSession("w/api", "/srv/api"),
		domain.NewListedSession("w/renamed", "/tmp").StartedFor("/srv/api"),
	}
	session, ok := ref(t, "/srv/api", "api").SessionAmong(sessions)
	if !ok || session.Name() != "w/renamed" || !session.Owned() {
		t.Fatalf("got %v, %v; want the owned w/renamed", session, ok)
	}
}

func TestSessionAmongTakesOneStartedByHandInTheRootUnderItsName(t *testing.T) {
	sessions := []domain.ListedSession{domain.NewListedSession("w/api", "/srv/api/")}
	session, ok := ref(t, "/srv/api", "api").SessionAmong(sessions)
	if !ok || session.Owned() {
		t.Fatalf("got %v, %v; want w/api, not owned", session, ok)
	}
}

func TestSessionAmongSkips(t *testing.T) {
	cases := map[string]domain.ListedSession{
		"a session elsewhere under its name":          domain.NewListedSession("w/api", "/srv/other"),
		"a session in its root under another name":    domain.NewListedSession("api", "/srv/api"),
		"another project's session under its name":    domain.NewListedSession("w/api", "/srv/api").StartedFor("/srv/other"),
		"a session in its root under another project": domain.NewListedSession("w/web", "/srv/api"),
	}
	for name, session := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := ref(t, "/srv/api", "api").SessionAmong([]domain.ListedSession{session}); ok {
				t.Fatal("found a session that isn't the project's")
			}
		})
	}
}

func TestSessionAmongTakesTheApartName(t *testing.T) {
	sessions := []domain.ListedSession{domain.NewListedSession("w/srv/b/api", "/srv/b/api")}
	if _, ok := ref(t, "/srv/b/api", "api").SessionAmong(sessions); !ok {
		t.Fatal("missed the session under the project's apart name")
	}
}
