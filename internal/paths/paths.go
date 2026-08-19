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
// The shared SQLite store lives in DBDir (graph today; more features later).
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

func FindRoot(start string) (string, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	// Prefer the git top-level so nested package .so dirs do not become the
	// registered project root. Explicit --root / SUPEROPEN_ROOT still win.
	var soRoot string
	for dir := absolute; ; dir = filepath.Dir(dir) {
		if info, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir, nil
		}
		if soRoot == "" {
			if info, statErr := os.Stat(filepath.Join(dir, DirName)); statErr == nil && info.IsDir() {
				soRoot = dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			if soRoot != "" {
				return soRoot, nil
			}
			return absolute, nil
		}
	}
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
