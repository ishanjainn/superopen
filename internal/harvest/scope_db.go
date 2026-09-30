package harvest

import (
	"context"
	"database/sql"
	"strings"

	"github.com/ishanjainn/superopen/internal/scope"
)

type harvestDB struct {
	*sql.DB
	tenant    string
	principal string
	project   string
}

func (d *harvestDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	q, err := harvestSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.QueryContext(ctx, q, args...)
}

func (d *harvestDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	q, err := harvestSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return d.DB.QueryRowContext(ctx, `SELECT 1 WHERE 0`)
	}
	return d.DB.QueryRowContext(ctx, q, args...)
}

func (d *harvestDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	q, err := harvestSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.ExecContext(ctx, q, args...)
}

func (d *harvestDB) Query(query string, args ...any) (*sql.Rows, error) {
	q, err := harvestSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.Query(q, args...)
}

func (d *harvestDB) QueryRow(query string, args ...any) *sql.Row {
	q, err := harvestSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return d.DB.QueryRow(`SELECT 1 WHERE 0`)
	}
	return d.DB.QueryRow(q, args...)
}

func (d *harvestDB) Exec(query string, args ...any) (sql.Result, error) {
	q, err := harvestSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.Exec(q, args...)
}

func harvestSQL(query, tenant, principal, project string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(upper, "INSERT") || strings.HasPrefix(upper, "CREATE") || strings.HasPrefix(upper, "PRAGMA") || strings.HasPrefix(upper, "DROP") {
		return query, nil
	}
	if !strings.Contains(query, "harvest_runs") && !strings.Contains(query, "harvest_proposals") {
		return query, nil
	}
	if err := scope.Check(scope.Scope{TenantID: tenant, PrincipalID: principal, ProjectID: project}); err != nil {
		return "", err
	}
	if strings.Contains(query, "tenant_id=") {
		return query, nil
	}
	pred := "tenant_id=" + scope.SQLLiteral(tenant) + " AND principal_id=" + scope.SQLLiteral(principal)
	if strings.Contains(query, "WHERE") {
		return strings.Replace(query, "WHERE", "WHERE "+pred+" AND ", 1), nil
	}
	for _, sep := range []string{" ORDER BY", " GROUP BY", " LIMIT"} {
		if i := strings.Index(query, sep); i >= 0 {
			return query[:i] + " WHERE " + pred + query[i:], nil
		}
	}
	return query + " WHERE " + pred, nil
}
