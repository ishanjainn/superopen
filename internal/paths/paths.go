package paths

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ishanjainn/superopen/internal/scope"
	_ "modernc.org/sqlite"
)

const (
	DirName      = ".so"
	SessionsName = "sessions"
	DBName       = "db"
	DatabaseFile = "so.db"
	BuildLock    = "build.lock"
)

// Paths contains repository-local Superopen storage under .so/.
// The shared SQLite store lives in DBDir (graph + memory).
type Paths struct {
	RepoRoot      string
	Root          string
	TracesDir     string
	SessionsDir   string
	SessionsIndex string
	DBDir         string
	Database      string
	BuildLock     string
	Scope         scope.Scope
}

// GitignoreContents is written to .so/.gitignore on first init or worktree seed.
const GitignoreContents = "# Superopen machine-local data (do not commit).\nsessions/\ndb/\nharvest/\n"

func FindRoot(start string) (string, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	// Walk with filepath (Windows-safe; .git may be a dir or a gitfile).
	// Remember the nearest managed .so/. If a git top-level is also managed,
	// that wins (one graph per repo; --root still for package graphs). If the
	// git top-level is unmanaged, an inited subfolder keeps its own store.
	var nearestManaged, gitRoot string
	for dir := absolute; ; dir = filepath.Dir(dir) {
		if gitRoot == "" && isGitDir(dir) {
			gitRoot = dir
			if Managed(dir) {
				return dir, nil
			}
			if nearestManaged != "" {
				return nearestManaged, nil
			}
			return dir, nil
		}
		if nearestManaged == "" && Managed(dir) {
			nearestManaged = dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			if nearestManaged != "" {
				return nearestManaged, nil
			}
			return absolute, nil
		}
	}
}

func isGitDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

func Resolve(repoRoot string) Paths {
	root := filepath.Join(repoRoot, DirName)
	sessions := filepath.Join(root, SessionsName)
	dbDir := filepath.Join(root, DBName)
	layout := Paths{
		RepoRoot:      repoRoot,
		Root:          root,
		TracesDir:     sessions,
		SessionsDir:   sessions,
		SessionsIndex: filepath.Join(sessions, "index.json"),
		DBDir:         dbDir,
		Database:      filepath.Join(dbDir, DatabaseFile),
		BuildLock:     filepath.Join(dbDir, BuildLock),
	}
	if sc, err := scope.Current(repoRoot); err == nil {
		layout.Scope = sc
	}
	return layout
}

// ResetStaleStore removes a database that is not the current shape, plus session files.
func ResetStaleStore(layout Paths) error {
	if _, err := os.Stat(layout.Database); err != nil {
		return nil
	}
	if storeHasScopeColumns(layout.Database) {
		return nil
	}
	for _, path := range []string{layout.Database, layout.Database + "-wal", layout.Database + "-shm", layout.Database + ".key"} {
		_ = os.Remove(path)
	}
	if layout.SessionsDir != "" {
		_ = os.RemoveAll(layout.SessionsDir)
	}
	return nil
}

func (paths Paths) Exists() bool {
	info, err := os.Stat(paths.Root)
	return err == nil && info.IsDir()
}

// UnmanagedMessage is the CLI skip text when a tree has not been inited.
const UnmanagedMessage = "not a Superopen repo; run so init"

// Managed reports whether repoRoot has a .so/ directory (so init opt-in).
func Managed(repoRoot string) bool {
	return Resolve(repoRoot).Exists()
}

func (paths Paths) EnsureDirs() error {
	for _, dir := range []string{paths.Root, paths.SessionsDir, paths.DBDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func (paths Paths) SessionDir(id string) string {
	return filepath.Join(paths.SessionsDir, id)
}

func storeHasScopeColumns(dbPath string) bool {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return false
	}
	defer db.Close()
	return !DatabaseMissingScope(db)
}

// DatabaseMissingScope reports a database that has tables but not the current scope columns.
func DatabaseMissingScope(db *sql.DB) bool {
	var projects, episodes int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='projects'`).Scan(&projects)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memory_episodes'`).Scan(&episodes)
	if projects == 0 && episodes == 0 {
		return false
	}
	if projects > 0 {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='tenant_id'`).Scan(&n); err != nil || n == 0 {
			return true
		}
	}
	if episodes > 0 {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('memory_episodes') WHERE name='tenant_id'`).Scan(&n); err != nil || n == 0 {
			return true
		}
	}
	return false
}
