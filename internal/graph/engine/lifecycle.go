package engine

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func (s *Store) DeleteProject(ctx context.Context, project string) (bool, error) {
	if project == "" {
		return false, errors.New("project is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE name=?`, project)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func Diagnose(ctx context.Context, repoRoot, project string) (api.DiagnosticsResult, error) {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return api.DiagnosticsResult{}, err
	}
	store, err := OpenReadOnly(paths.Database)
	if err != nil {
		return api.DiagnosticsResult{Healthy: false, Database: paths.Database}, err
	}
	defer store.Close()
	result := api.DiagnosticsResult{Healthy: true, Database: paths.Database}
	if err := store.Verify(ctx); err != nil {
		result.Healthy = false
		result.Diagnostics = append(result.Diagnostics, api.Diagnostic{Severity: "error", Code: "integrity", Message: err.Error()})
		return result, nil
	}
	if project != "" {
		var one int
		if err := store.db.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE name=?`, project).Scan(&one); errors.Is(err, sql.ErrNoRows) {
			result.Healthy = false
			result.Diagnostics = append(result.Diagnostics, api.Diagnostic{Severity: "error", Code: "project_missing", Message: "project is not indexed"})
		} else if err != nil {
			return result, err
		}
	}
	return result, nil
}
