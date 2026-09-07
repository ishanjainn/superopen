package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProbeDirtyCleanSidecarDoesNotRequireDatabase(t *testing.T) {
	root := initGitRepo(t, map[string]string{"a.go": "package a\n"})
	if err := WriteFingerprint(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	dirty, err := ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Fatal("matching size+mtime sidecar must be clean without opening SQLite")
	}
}

func TestFingerprintStampIsNeverUnknown(t *testing.T) {
	stamp := fingerprintStamp()
	if stamp == "" || strings.Contains(strings.ToLower(stamp), "unknown") {
		t.Fatalf("stamp must be a real identity, got %q", stamp)
	}
	if !strings.Contains(stamp, AssetRevision) {
		t.Fatalf("stamp must include AssetRevision: %q", stamp)
	}
}

func TestProbeDirtyMissingSidecarIsDirtyNotUnknown(t *testing.T) {
	root := initGitRepo(t, map[string]string{"a.go": "package a\n"})
	seedGraphDB(t, root, map[string]string{"a.go": "package a\n"})
	dirty, err := ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("missing fingerprint sidecar must be dirty (null identity), not a synthetic unknown stamp")
	}
}

func TestCleanProbeDoesNotHashWhenSizeMtimeMatch(t *testing.T) {
	root := initGitRepo(t, map[string]string{"a.go": "aaaa"})
	seedGraphDB(t, root, map[string]string{"a.go": "aaaa"})
	if err := WriteFingerprint(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "a.go")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	dirty, err := ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Fatal("size+mtime match must reuse the sidecar hash and stay clean")
	}
	t.Setenv(refreshModeEnv, "hash")
	dirty, err = ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("SUPEROPEN_REFRESH=hash must hash bytes and see the content change")
	}
}

func TestProbeDirtyWhenExcludesChange(t *testing.T) {
	root := initGitRepo(t, map[string]string{"a.go": "package a\n", "vendor/x.go": "package x\n"})
	seedGraphDB(t, root, map[string]string{"a.go": "package a\n"})
	if err := WriteFingerprint(context.Background(), root, []string{"vendor"}); err != nil {
		t.Fatal(err)
	}
	dirty, err := ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("exclude-list change must invalidate the sidecar")
	}
}

func TestProbeDirtySeesSecondEdit(t *testing.T) {
	root := initGitRepo(t, map[string]string{"a.go": "one"})
	seedGraphDB(t, root, map[string]string{"a.go": "one"})
	if err := WriteFingerprint(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "a.go")
	if err := os.WriteFile(file, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("first edit must dirty the probe")
	}
	if err := os.WriteFile(file, []byte("three"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("second edit of an already-dirty file must still be dirty")
	}
}

func initGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "so@example.com")
	runGit(t, root, "config", "user.name", "so")
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
	return root
}

func seedGraphDB(t *testing.T, root string, files map[string]string) {
	t.Helper()
	project, err := ProjectName(root)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := CachePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := OpenWritable(paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(context.Background(), func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: project, RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		for rel, body := range files {
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			if err := builder.PutFile(FileRecord{
				Project: project, Path: rel, SHA256: fileContentDigest([]byte(body)),
				MTimeNS: info.ModTime().UnixNano(), Size: info.Size(), Language: "go",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
	}
}
