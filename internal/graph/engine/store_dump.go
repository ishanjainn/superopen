package engine

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

const (
	dumpPartitionSize = 65536
	dumpInsertBatch   = 128
)

func (s *Store) BuildFresh(ctx context.Context, fn func(*Builder) error) error {
	if _, err := s.db.Exec(`PRAGMA synchronous = OFF`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, schemaFTSDDL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("create fts table: %w", err)
	}
	b := &Builder{tx: tx, insertOnly: true}
	dumpStarted := time.Now()
	buildErr := fn(b)
	if buildErr == nil {
		buildErr = b.flushInsertBatches()
	}
	if buildErr != nil {
		_ = tx.Rollback()
		return buildErr
	}
	reportIndexProgress("dump done elapsed=%s", indexElapsed(dumpStarted))
	reportIndexProgress("Indexing search tables...")
	ftsStarted := time.Now()
	if err := b.flushSearchIndex(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	reportIndexProgress("fts done elapsed=%s", indexElapsed(ftsStarted))
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit graph: %w", err)
	}
	_, _ = s.db.Exec(`PRAGMA foreign_keys = ON`)
	_, _ = s.db.Exec(`PRAGMA synchronous = NORMAL`)
	return nil
}

func (b *Builder) putNodeInsert(n api.Node, props string) (int64, error) {
	b.nextNodeID++
	id := b.nextNodeID
	file := filepath.ToSlash(n.Location.File)
	b.nodeBatch = append(b.nodeBatch, []any{
		id, n.Project, n.Label, n.Name, n.QualifiedName,
		file, n.Location.StartLine, n.Location.StartColumn,
		n.Location.EndLine, n.Location.EndColumn, props,
	})
	b.ftsBatch = append(b.ftsBatch, []any{id, camelTerms(n.Name), camelTerms(n.QualifiedName), n.Label, file})
	if len(b.nodeBatch) >= dumpInsertBatch {
		if err := b.flushNodeBatch(); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (b *Builder) queueEdgeInsert(project string, sourceID, targetID int64, edgeType, props, evidence, localName string) (int64, error) {
	b.edgeBatch = append(b.edgeBatch, []any{project, sourceID, targetID, edgeType, props, evidence, localName})
	if len(b.edgeBatch) >= dumpInsertBatch {
		return 0, b.flushEdgeBatch()
	}
	return 0, nil
}

func (b *Builder) flushInsertBatches() error {
	if err := b.flushNodeBatch(); err != nil {
		return err
	}
	return b.flushEdgeBatch()
}

func (b *Builder) flushNodeBatch() error {
	if err := execInsertBatch(b.tx, `INSERT INTO nodes(id,project,label,name,qualified_name,file_path,start_line,start_column,end_line,end_column,properties) VALUES`, 11, b.nodeBatch); err != nil {
		return err
	}
	b.nodeBatch = b.nodeBatch[:0]
	if err := execInsertBatch(b.tx, `INSERT INTO nodes_fts(rowid,name,qualified_name,label,file_path) VALUES`, 5, b.ftsBatch); err != nil {
		return err
	}
	b.ftsBatch = b.ftsBatch[:0]
	return nil
}

func (b *Builder) flushEdgeBatch() error {
	if err := execInsertBatch(b.tx, `INSERT INTO edges(project,source_id,target_id,type,properties,evidence,local_name) VALUES`, 7, b.edgeBatch); err != nil {
		return err
	}
	b.edgeBatch = b.edgeBatch[:0]
	return nil
}

func execInsertBatch(tx *sql.Tx, prefix string, width int, rows [][]any) error {
	if tx == nil || len(rows) == 0 {
		return nil
	}
	var sql strings.Builder
	args := make([]any, 0, len(rows)*width)
	sql.WriteString(prefix)
	for i, row := range rows {
		if i > 0 {
			sql.WriteByte(',')
		}
		sql.WriteByte('(')
		for j := 0; j < width; j++ {
			if j > 0 {
				sql.WriteByte(',')
			}
			sql.WriteByte('?')
		}
		sql.WriteByte(')')
		args = append(args, row...)
	}
	_, err := tx.Exec(sql.String(), args...)
	return err
}

func (b *Builder) flushSearchIndex(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := b.tx.ExecContext(ctx, schemaIndexDDL); err != nil {
		return fmt.Errorf("create search indexes: %w", err)
	}
	return nil
}
