package engine

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

const noSeedEnv = "SUPEROPEN_NO_SEED"

// SeedLinkedWorktree copies the parent checkout's so.db into an uninited
// linked worktree. Fail-open: errors leave the worktree unmanaged.
func SeedLinkedWorktree(repoRoot string) {
	if seedDisabled() {
		return
	}
	root, err := CanonicalRoot(repoRoot)
	if err != nil {
		return
	}
	if paths.Managed(root) {
		return
	}
	parent, ok := paths.LinkedWorktreeParent(root)
	if !ok || !paths.Managed(parent) {
		return
	}
	parentDB := paths.Resolve(parent).Database
	if _, err := os.Stat(parentDB); err != nil {
		return
	}
	tmpSO, err := os.MkdirTemp(root, "so-seed-*")
	if err != nil {
		return
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpSO)
		}
	}()
	if err := os.MkdirAll(filepath.Join(tmpSO, paths.DBName), 0o755); err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Join(tmpSO, paths.SessionsName), 0o755); err != nil {
		return
	}
	tmpDB := filepath.Join(tmpSO, paths.DBName, paths.DatabaseFile)
	if err := backupSQLite(parentDB, tmpDB); err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(tmpSO, ".gitignore"), []byte(paths.GitignoreContents), 0o644); err != nil {
		return
	}
	if paths.Managed(root) {
		return
	}
	dest := paths.Resolve(root).Root
	// Do not hold a file handle inside tmpSO: Windows cannot rename a
	// directory while a file in it is open (and copying that lock file
	// then fails with a sharing violation).
	if err := os.Rename(tmpSO, dest); err != nil {
		if copyErr := copyDir(tmpSO, dest); copyErr != nil {
			_ = os.RemoveAll(dest)
			return
		}
	}
	cleanup = false
	_, _ = paths.EnsureRepoIgnore(root)
}

func seedDisabled() bool {
	v := strings.TrimSpace(os.Getenv(noSeedEnv))
	return v == "1" || strings.EqualFold(v, "true")
}

func backupSQLite(src, dest string) error {
	if err := vacuumInto(src, dest); err == nil {
		return nil
	}
	if !isSQLiteFile(src) {
		return fmt.Errorf("not sqlite")
	}
	return copyFile(src, dest)
}

func isSQLiteFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var hdr [16]byte
	n, err := f.Read(hdr[:])
	if err != nil || n < 16 {
		return false
	}
	return string(hdr[:]) == "SQLite format 3\x00"
}

func vacuumInto(src, dest string) error {
	dsn := "file:" + filepath.ToSlash(src) + "?mode=ro"
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("VACUUM INTO " + sqliteStringLiteral(filepath.ToSlash(dest)))
	return err
}

func sqliteStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func copyDir(from, to string) error {
	if err := os.RemoveAll(to); err != nil {
		return err
	}
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Name() == paths.BuildLock {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}
