package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type incrementalSnapshot struct {
	files    map[string]string
	revision string
}

// PlanIncremental computes a content-based change set against the last valid
// published graph. It never trusts mtimes, so checkout/branch changes and
// timestamp-preserving editors cannot silently reuse stale syntax.
func PlanIncremental(ctx context.Context, repoRoot, project string, excludes []string) (api.ChangeSet, error) {
	root, err := CanonicalRoot(repoRoot)
	if err != nil {
		return api.ChangeSet{}, err
	}
	if project == "" {
		project, err = ProjectName(root)
		if err != nil {
			return api.ChangeSet{}, err
		}
	}
	paths, err := CachePaths(root)
	if err != nil {
		return api.ChangeSet{}, err
	}
	prior := incrementalSnapshot{files: map[string]string{}}
	store, err := OpenReadOnly(paths.Database)
	if err == nil {
		defer store.Close()
		rows, queryErr := store.db.QueryContext(ctx, `SELECT rel_path,sha256 FROM file_hashes WHERE project=? ORDER BY rel_path`, project)
		if queryErr != nil {
			return api.ChangeSet{}, queryErr
		}
		for rows.Next() {
			var path, digest string
			if err := rows.Scan(&path, &digest); err != nil {
				rows.Close()
				return api.ChangeSet{}, err
			}
			prior.files[path] = digest
		}
		if err := rows.Close(); err != nil {
			return api.ChangeSet{}, err
		}
		if err := store.db.QueryRowContext(ctx, `SELECT source_revision FROM projects WHERE name=?`, project).Scan(&prior.revision); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return api.ChangeSet{}, err
		}
	} else if !os.IsNotExist(err) {
		return api.ChangeSet{}, err
	}
	files, err := discoverTrackedFiles(ctx, root, excludes)
	if err != nil {
		return api.ChangeSet{}, err
	}
	current := make(map[string]string, len(files))
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return api.ChangeSet{}, err
		}
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return api.ChangeSet{}, err
		}
		digest := sha256.Sum256(body)
		current[filepath.ToSlash(rel)] = hex.EncodeToString(digest[:])
	}
	return planIncrementalChanges(prior, current, gitRevision(ctx, root)), nil
}

func planIncrementalChanges(prior incrementalSnapshot, current map[string]string, revision string) api.ChangeSet {
	result := api.ChangeSet{SourceRevision: revision, RevisionChanged: prior.revision != "" && prior.revision != revision}
	added := map[string]string{}
	deleted := map[string]string{}
	for path, digest := range current {
		if old, ok := prior.files[path]; !ok {
			added[path] = digest
		} else if old != digest {
			result.Modified = append(result.Modified, api.FileChange{Kind: "modified", Path: path, SHA256: digest})
		} else {
			result.Unchanged++
		}
	}
	for path, digest := range prior.files {
		if _, ok := current[path]; !ok {
			deleted[path] = digest
		}
	}
	// Rename detection is deliberately one-to-one. Duplicate-content files are
	// left as add/delete pairs instead of guessing and corrupting identity.
	addedByHash := pathsByDigest(added)
	deletedByHash := pathsByDigest(deleted)
	for digest, newPaths := range addedByHash {
		oldPaths := deletedByHash[digest]
		if len(newPaths) == 1 && len(oldPaths) == 1 {
			result.Renamed = append(result.Renamed, api.FileChange{Kind: "renamed", Path: newPaths[0], OldPath: oldPaths[0], SHA256: digest})
			delete(added, newPaths[0])
			delete(deleted, oldPaths[0])
		}
	}
	for path, digest := range added {
		result.Added = append(result.Added, api.FileChange{Kind: "added", Path: path, SHA256: digest})
	}
	for path, digest := range deleted {
		result.Deleted = append(result.Deleted, api.FileChange{Kind: "deleted", Path: path, SHA256: digest})
	}
	sortChanges := func(values []api.FileChange) {
		sort.Slice(values, func(i, j int) bool {
			if values[i].Path != values[j].Path {
				return values[i].Path < values[j].Path
			}
			return values[i].OldPath < values[j].OldPath
		})
	}
	sortChanges(result.Added)
	sortChanges(result.Modified)
	sortChanges(result.Deleted)
	sortChanges(result.Renamed)
	if len(prior.files) == 0 {
		result.RequiresFull = true
		result.Reason = "no compatible prior generation"
	}
	return result
}

func pathsByDigest(files map[string]string) map[string][]string {
	result := map[string][]string{}
	for path, digest := range files {
		result[digest] = append(result[digest], path)
	}
	for digest := range result {
		sort.Strings(result[digest])
	}
	return result
}
