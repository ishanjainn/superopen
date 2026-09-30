package session

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/scope"
)

func TestTwoPrincipalsDoNotSeeSessions(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	layout.Scope = scope.Scope{TenantID: "local", PrincipalID: "ada", ProjectID: "p1"}
	ada := NewStore(layout)
	if err := ada.Start(Meta{ID: "s-ada", Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	graceLayout := layout
	graceLayout.Scope.PrincipalID = "grace"
	grace := NewStore(graceLayout)
	list, err := grace.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("grace saw %d sessions", len(list))
	}
	if _, err := grace.Get("s-ada"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("grace get: %v", err)
	}
	adaList, err := ada.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(adaList) != 1 || adaList[0].PrincipalID != "ada" {
		t.Fatalf("ada list: %+v", adaList)
	}
}
