package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

const (
	fingerprintName  = "fingerprint.json"
	QueryRefreshWait = 2 * time.Second
	GraphStaleHeader = "[!] graph stale: edit not indexed yet"
	noRefreshEnv     = "SUPEROPEN_NO_REFRESH"
	refreshModeEnv   = "SUPEROPEN_REFRESH"
)

type fingerprintEntry struct {
	Size    int64  `json:"size"`
	MtimeMs int64  `json:"mtime_ms"`
	SHA256  string `json:"sha256"`
}

type fingerprintDoc struct {
	Stamp    string                      `json:"stamp"`
	Excludes []string                    `json:"excludes"`
	Files    map[string]fingerprintEntry `json:"files"`
}

func fingerprintStamp() string {
	return AssetRevision + ":" + strconv.Itoa(api.SchemaVersion)
}

func fingerprintPath(repoRoot string) string {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return ""
	}
	return filepath.Join(paths.Root, fingerprintName)
}

func loadFingerprint(repoRoot string) *fingerprintDoc {
	path := fingerprintPath(repoRoot)
	if path == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc fingerprintDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil
	}
	if doc.Stamp != fingerprintStamp() || doc.Files == nil {
		return nil
	}
	return &doc
}

func excludesMatch(a, b []string) bool {
	left := append([]string{}, a...)
	right := append([]string{}, b...)
	sort.Strings(left)
	sort.Strings(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func hashRefreshMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(refreshModeEnv)), "hash")
}

func RefreshDisabled() bool {
	v := strings.TrimSpace(os.Getenv(noRefreshEnv))
	return v == "1" || strings.EqualFold(v, "true")
}

// ProbeDirty reports whether the working tree differs from the fingerprint sidecar.
// Identity failure (missing/corrupt sidecar) is dirty, never a synthetic "unknown" stamp.
// A complete sidecar with matching size+mtime is clean without opening SQLite.
func ProbeDirty(ctx context.Context, repoRoot string, excludes []string) (bool, error) {
	root, err := CanonicalRoot(repoRoot)
	if err != nil {
		return false, err
	}
	fp := loadFingerprint(root)
	if fp == nil || !excludesMatch(fp.Excludes, excludes) {
		return true, nil
	}
	if hashRefreshMode() {
		changes, err := PlanIncrementalFromProbe(ctx, root, "", excludes)
		if err != nil {
			return false, err
		}
		return changeVolume(changes) > 0 || changes.RequiresFull, nil
	}
	files, err := discoverTrackedFiles(ctx, root, excludes)
	if err != nil {
		return false, err
	}
	seen := make(map[string]struct{}, len(files))
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		key := filepath.ToSlash(rel)
		entry, ok := fp.Files[key]
		if !ok {
			return true, nil
		}
		info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if statErr != nil {
			return true, nil
		}
		if entry.Size != info.Size() || entry.MtimeMs != mtimeMs(info) {
			return true, nil
		}
		seen[key] = struct{}{}
	}
	if len(seen) != len(fp.Files) {
		return true, nil
	}
	return false, nil
}

// PlanIncrementalFromProbe hashes only files whose size+mtime (or identity) changed.
func PlanIncrementalFromProbe(ctx context.Context, repoRoot, project string, excludes []string) (api.ChangeSet, error) {
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
	if os.IsNotExist(err) {
		return api.ChangeSet{RequiresFull: true, Reason: "no compatible prior generation", SourceRevision: gitRevision(ctx, root)}, nil
	}
	if err != nil {
		return api.ChangeSet{}, err
	}
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
	files, err := discoverTrackedFiles(ctx, root, excludes)
	if err != nil {
		return api.ChangeSet{}, err
	}
	fp := loadFingerprint(root)
	if fp != nil && !excludesMatch(fp.Excludes, excludes) {
		fp = nil
	}
	forceHash := hashRefreshMode() || fp == nil
	current := make(map[string]string, len(files))
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return api.ChangeSet{}, err
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		info, statErr := os.Stat(abs)
		key := filepath.ToSlash(rel)
		if statErr != nil {
			continue
		}
		entry, ok := fingerprintEntry{}, false
		if fp != nil {
			entry, ok = fp.Files[key]
		}
		if !forceHash && ok && entry.Size == info.Size() && entry.MtimeMs == mtimeMs(info) && entry.SHA256 != "" {
			current[key] = entry.SHA256
			continue
		}
		body, err := os.ReadFile(abs)
		if err != nil {
			return api.ChangeSet{}, err
		}
		digest := fileContentDigest(body)
		if ok && entry.SHA256 == digest {
			current[key] = digest
			continue
		}
		current[key] = digest
	}
	return planIncrementalChanges(prior, current, gitRevision(ctx, root)), nil
}

func mtimeMs(info os.FileInfo) int64 {
	return info.ModTime().UnixMilli()
}

// WriteFingerprint records size+mtime+hash for the current working tree.
func WriteFingerprint(ctx context.Context, repoRoot string, excludes []string) error {
	root, err := CanonicalRoot(repoRoot)
	if err != nil {
		return err
	}
	files, err := discoverTrackedFiles(ctx, root, excludes)
	if err != nil {
		return err
	}
	doc := fingerprintDoc{Stamp: fingerprintStamp(), Excludes: append([]string{}, excludes...), Files: map[string]fingerprintEntry{}}
	for _, rel := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		body, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		doc.Files[filepath.ToSlash(rel)] = fingerprintEntry{
			Size:    info.Size(),
			MtimeMs: mtimeMs(info),
			SHA256:  fileContentDigest(body),
		}
	}
	path := fingerprintPath(root)
	if path == "" {
		return fmt.Errorf("fingerprint path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

// WaitUntilFresh polls the probe until clean or budget expires.
func WaitUntilFresh(ctx context.Context, root string, excludes []string, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		dirty, err := ProbeDirty(ctx, root, excludes)
		if err == nil && !dirty {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	dirty, err := ProbeDirty(ctx, root, excludes)
	return err == nil && !dirty
}
