package memory

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var copyTables = []struct {
	name string
	cols []string
}{
	{name: "memory_episodes", cols: []string{
		"id", "tenant_id", "principal_id", "uid", "session_id", "span_id", "kind", "source", "title", "text", "files", "tool_name",
		"tokens", "pinned", "faded", "embedding_pending", "created_at", "updated_at", "valid_from", "valid_to",
		"faded_at", "last_accessed_at", "community_id", "centrality", "tier", "horizon", "keep_until_session", "never_decay", "tags", "fading",
		"topic", "facts", "narrative", "concepts", "content_hash",
	}},
	{name: "memory_vectors", cols: []string{"episode_id", "embedder_id", "dimensions", "quantization", "vector"}},
	{name: "memory_edges", cols: []string{"id", "source_id", "target_id", "type", "weight", "updated_at"}},
	{name: "memory_topics", cols: []string{"id", "label", "community", "episode_ids", "size", "updated_at"}},
	{name: "memory_economy", cols: []string{"id", "packs_served", "tokens_injected", "fallback_searches", "tokens_saved", "updated_at"}},
	{name: "memory_meta", cols: []string{"key", "value"}},
	{name: "memory_passages", cols: []string{"id", "episode_id", "ord", "text", "embedder_id", "dimensions", "quantization", "vector"}},
}

// CopyInto copies memory tables from srcPath into dstPath.
// Graph publish replaces so.db with a staged graph; this keeps the project diary.
// No-op if src is missing or has no memory_episodes table.
func CopyInto(srcPath, dstPath string) error {
	srcPath, err := filepath.Abs(srcPath)
	if err != nil {
		return err
	}
	dstPath, err = filepath.Abs(dstPath)
	if err != nil {
		return err
	}
	if srcPath == dstPath {
		return nil
	}
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	src, err := sql.Open("sqlite", srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	var n int
	if err := src.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memory_episodes'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if err := copyMemoryKey(srcPath, dstPath); err != nil {
		return err
	}

	dst, err := Open(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := dst.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	if _, err := dst.db.Exec(`ATTACH DATABASE ? AS memsrc`, srcPath); err != nil {
		return fmt.Errorf("attach memory source: %w", err)
	}
	defer func() {
		_, _ = dst.db.Exec(`DETACH DATABASE memsrc`)
		_, _ = dst.db.Exec(`PRAGMA foreign_keys = ON`)
	}()

	tx, err := dst.db.Begin()
	if err != nil {
		return err
	}
	for _, table := range []string{"memory_passages", "memory_edges", "memory_vectors", "memory_topics", "memory_episodes", "memory_economy", "memory_meta"} {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("preserve memory delete %s: %w", table, err)
		}
	}
	for _, table := range copyTables {
		cols := strings.Join(table.cols, ",")
		q := fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM memsrc.%s`, table.name, cols, cols, table.name)
		if _, err := tx.Exec(q); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("preserve memory (%s): %w", table.name, err)
		}
	}
	if _, err := tx.Exec(`
INSERT OR REPLACE INTO sqlite_sequence(name, seq)
SELECT name, seq FROM memsrc.sqlite_sequence WHERE name LIKE 'memory_%'`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			_ = tx.Rollback()
			return fmt.Errorf("preserve memory sequence: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, _ = dst.db.Exec(`DETACH DATABASE memsrc`)
	_, _ = dst.db.Exec(`PRAGMA foreign_keys = ON`)
	if err := dst.rebuildFTS(); err != nil {
		return fmt.Errorf("preserve memory fts: %w", err)
	}
	if _, err := dst.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint preserved memory: %w", err)
	}
	return nil
}

func copyMemoryKey(srcDB, dstDB string) error {
	dst := dstDB + ".key"
	in, err := os.Open(srcDB + ".key")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
