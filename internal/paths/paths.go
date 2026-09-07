package paths

import (
	"fmt"
	"os"
	"path/filepath"
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
	return Paths{
		RepoRoot:      repoRoot,
		Root:          root,
		TracesDir:     sessions,
		SessionsDir:   sessions,
		SessionsIndex: filepath.Join(sessions, "index.json"),
		DBDir:         dbDir,
		Database:      filepath.Join(dbDir, DatabaseFile),
		BuildLock:     filepath.Join(dbDir, BuildLock),
	}
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
