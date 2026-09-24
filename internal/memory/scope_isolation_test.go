package memory

import (
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/scope"
)

func TestTwoPrincipalsDoNotSeeEachOther(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "so.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a.scope = scope.Scope{TenantID: "local", PrincipalID: "ada", ProjectID: "p1"}
	a.db.tenant = "local"
	a.db.principal = "ada"
	a.db.project = "p1"
	if _, _, err := a.upsertEpisode(Episode{
		UID: "a1", Kind: KindSession, Title: "ada note", Text: "only ada",
	}, Vector{}, false); err != nil {
		t.Fatal(err)
	}
	a.Close()

	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	b.scope = scope.Scope{TenantID: "local", PrincipalID: "grace", ProjectID: "p1"}
	b.db.tenant = "local"
	b.db.principal = "grace"
	b.db.project = "p1"
	var n int
	if err := b.db.QueryRow(`SELECT count(*) FROM memory_episodes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("grace saw %d episodes", n)
	}
}

func TestMissingScopeRejected(t *testing.T) {
	if err := scope.Check(scope.Scope{}); err == nil {
		t.Fatal("empty scope must fail")
	}
	if _, err := personalSQL("SELECT count(*) FROM memory_episodes", "", "", ""); err == nil {
		t.Fatal("memory query without a tenant must fail")
	}
}
