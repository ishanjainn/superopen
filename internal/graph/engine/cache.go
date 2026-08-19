package engine

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/paths"
)

// AssetRevision pins the Superopen grammar and model asset bundle used by the
// native graph engine. Update it together with checked-in assets under
// internal/graph/engine/assets and internal/graph/langspec/assets.
const AssetRevision = "41d240accf91641b197620f7c33a4d2d60451d0b"

type Paths struct {
	Root     string
	Database string
	Lock     string
}

func CanonicalRoot(repoRoot string) (string, error) {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("absolute repository path: %w", err)
	}
	abs = filepath.Clean(abs)
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = filepath.Clean(real)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("repository root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository root is not a directory: %s", abs)
	}
	return abs, nil
}

// CachePaths resolves the shared Superopen store under <repo>/.so/db/.
// The database file is so.db (graph schema today; other features later).
func CachePaths(repoRoot string) (Paths, error) {
	canonical, err := CanonicalRoot(repoRoot)
	if err != nil {
		return Paths{}, err
	}
	layout := paths.Resolve(canonical)
	return Paths{
		Root:     layout.DBDir,
		Database: layout.Database,
		Lock:     layout.BuildLock,
	}, nil
}

// LegacyCachePaths is the pre-.so/db user-cache location used for one-time migration.
func LegacyCachePaths(repoRoot string) (Paths, error) {
	canonical, err := CanonicalRoot(repoRoot)
	if err != nil {
		return Paths{}, err
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, fmt.Errorf("user cache directory: %w", err)
	}
	sum := sha256.Sum256([]byte(canonical))
	key := fmt.Sprintf("%s-%x", slug(filepath.Base(canonical)), sum[:12])
	root := filepath.Join(base, "superopen", "graph", fmt.Sprintf("v%d", api.SchemaVersion), key)
	return Paths{
		Root:     root,
		Database: filepath.Join(root, "graph.db"),
		Lock:     filepath.Join(root, "build.lock"),
	}, nil
}

// MigrateLegacyCacheIfNeeded copies a legacy cache graph.db into .so/db/so.db
// when the new store is missing. Best-effort; never overwrites an existing so.db.
func MigrateLegacyCacheIfNeeded(repoRoot string) error {
	dst, err := CachePaths(repoRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dst.Database); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	src, err := LegacyCachePaths(repoRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(src.Database); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dst.Root, 0o755); err != nil {
		return err
	}
	return copyFile(src.Database, dst.Database)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func ProjectName(repoRoot string) (string, error) {
	canonical, err := CanonicalRoot(repoRoot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%s-%x", slug(filepath.Base(canonical)), sum[:4]), nil
}

func slug(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "repository"
	}
	return out
}
