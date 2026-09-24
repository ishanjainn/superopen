package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/scope"
)

func TestGraphNodesStayInsideTenant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "so.db")
	ctx := context.Background()
	local := scope.Scope{TenantID: "local", PrincipalID: "ada", ProjectID: "p1"}
	other := scope.Scope{TenantID: "other", PrincipalID: "ada", ProjectID: "p1"}
	store, err := OpenWritableScoped(path, local)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(b *Builder) error {
		if err := b.PutProject(ProjectRecord{Name: "demo", RootPath: "/repo", Generation: "g1"}); err != nil {
			return err
		}
		_, err := b.PutNode(api.Node{Project: "demo", Label: "Function", Name: "Run", QualifiedName: "demo.Run"})
		return err
	})
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	again, err := OpenWritableScoped(path, other)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	var n int
	if err := again.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes WHERE project=?`, "demo").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("other tenant saw %d nodes", n)
	}
}

func TestScopeSQLRejectsMissingTenant(t *testing.T) {
	if _, err := scopeSQL("SELECT count(*) FROM nodes WHERE project=?", "", "", ""); err == nil {
		t.Fatal("graph query without a tenant must fail")
	}
}
