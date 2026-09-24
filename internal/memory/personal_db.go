package memory

import (
	"context"
	"database/sql"
	"strings"

	"github.com/ishanjainn/superopen/internal/scope"
)

type personalDB struct {
	*sql.DB
	tenant    string
	principal string
	project   string
}

func (d *personalDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	q, err := personalSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.QueryContext(ctx, q, args...)
}

func (d *personalDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	q, err := personalSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return d.DB.QueryRowContext(ctx, `SELECT 1 WHERE 0`)
	}
	return d.DB.QueryRowContext(ctx, q, args...)
}

func (d *personalDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	q, err := personalSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.ExecContext(ctx, q, args...)
}

func (d *personalDB) Query(query string, args ...any) (*sql.Rows, error) {
	q, err := personalSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.Query(q, args...)
}

func (d *personalDB) QueryRow(query string, args ...any) *sql.Row {
	q, err := personalSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return d.DB.QueryRow(`SELECT 1 WHERE 0`)
	}
	return d.DB.QueryRow(q, args...)
}

func (d *personalDB) Exec(query string, args ...any) (sql.Result, error) {
	q, err := personalSQL(query, d.tenant, d.principal, d.project)
	if err != nil {
		return nil, err
	}
	return d.DB.Exec(q, args...)
}

func personalSQL(query, tenant, principal, project string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(upper, "INSERT") || strings.HasPrefix(upper, "CREATE") || strings.HasPrefix(upper, "PRAGMA") || strings.HasPrefix(upper, "DROP") || strings.HasPrefix(upper, "ANALYZE") {
		return query, nil
	}
	if strings.Contains(query, "memory_episodes") {
		if err := scope.Check(scope.Scope{TenantID: tenant, PrincipalID: principal, ProjectID: project}); err != nil {
			return "", err
		}
	}
	if strings.Contains(query, "tenant_id=") {
		return query, nil
	}
	pred := "memory_episodes.tenant_id=" + scope.SQLLiteral(tenant) + " AND memory_episodes.principal_id=" + scope.SQLLiteral(principal)
	if strings.Contains(query, "FROM memory_episodes WHERE") {
		return strings.Replace(query, "FROM memory_episodes WHERE", "FROM memory_episodes WHERE "+pred+" AND ", 1), nil
	}
	if strings.Contains(query, "UPDATE memory_episodes SET") && strings.Contains(query, " WHERE ") {
		return strings.Replace(query, " WHERE ", " WHERE "+pred+" AND ", 1), nil
	}
	if strings.Contains(query, "DELETE FROM memory_episodes") && strings.Contains(query, " WHERE ") {
		return strings.Replace(query, " WHERE ", " WHERE "+pred+" AND ", 1), nil
	}
	if strings.Contains(query, "FROM memory_episodes") && !strings.Contains(query, "memory_episodes_fts") && !strings.Contains(query, "WHERE") {
		if i := strings.Index(query, "ORDER BY"); i >= 0 {
			return query[:i] + "WHERE " + pred + " " + query[i:], nil
		}
		return query + " WHERE " + pred, nil
	}
	return query, nil
}
