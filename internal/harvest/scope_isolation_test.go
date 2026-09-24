package harvest

import (
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/scope"
)

func TestTwoPrincipalsDoNotSeeProposals(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "so.db")
	ada, err := open(root, path)
	if err != nil {
		t.Fatal(err)
	}
	ada.scope = scope.Scope{TenantID: "local", PrincipalID: "ada", ProjectID: "p1"}
	ada.db.tenant = "local"
	ada.db.principal = "ada"
	ada.db.project = "p1"
	if _, err := ada.InsertProposal(Proposal{
		SessionID: "s-ada", Status: StatusOpen, Kind: "memory", Target: "note", Title: "ada only",
	}); err != nil {
		t.Fatal(err)
	}
	ada.Close()

	grace, err := open(root, path)
	if err != nil {
		t.Fatal(err)
	}
	defer grace.Close()
	grace.scope = scope.Scope{TenantID: "local", PrincipalID: "grace", ProjectID: "p1"}
	grace.db.tenant = "local"
	grace.db.principal = "grace"
	grace.db.project = "p1"
	var n int
	if err := grace.db.QueryRow(`SELECT count(*) FROM harvest_proposals WHERE status=?`, StatusOpen).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("grace saw %d proposals", n)
	}
}

func TestHarvestSQLRejectsMissingTenant(t *testing.T) {
	if _, err := harvestSQL("SELECT count(*) FROM harvest_runs WHERE status=?", "", "", ""); err == nil {
		t.Fatal("harvest query without a tenant must fail")
	}
}
