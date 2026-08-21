package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ishanjainn/superopen/internal/graph/api"
	_ "modernc.org/sqlite"
)

const schemaDDL = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS store_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  name TEXT PRIMARY KEY,
  indexed_at TEXT NOT NULL,
  root_path TEXT NOT NULL,
  generation TEXT NOT NULL,
  source_revision TEXT NOT NULL DEFAULT '',
  engine_version TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS file_hashes (
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  rel_path TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  mtime_ns INTEGER NOT NULL DEFAULT 0,
  size INTEGER NOT NULL DEFAULT 0,
  language TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, rel_path)
);
CREATE TABLE IF NOT EXISTS nodes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  label TEXT NOT NULL,
  name TEXT NOT NULL,
  qualified_name TEXT NOT NULL,
  file_path TEXT NOT NULL DEFAULT '',
  start_line INTEGER NOT NULL DEFAULT 0,
  start_column INTEGER NOT NULL DEFAULT 0,
  end_line INTEGER NOT NULL DEFAULT 0,
  end_column INTEGER NOT NULL DEFAULT 0,
  properties TEXT NOT NULL DEFAULT '{}',
  UNIQUE(project, qualified_name)
);
CREATE TABLE IF NOT EXISTS edges (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  source_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  properties TEXT NOT NULL DEFAULT '{}',
  evidence TEXT NOT NULL DEFAULT '{}',
  local_name TEXT NOT NULL DEFAULT '',
  UNIQUE(project, source_id, target_id, type, local_name)
);
CREATE TABLE IF NOT EXISTS project_summaries (
  project TEXT PRIMARY KEY REFERENCES projects(name) ON DELETE CASCADE,
  summary TEXT NOT NULL,
  source_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS lsp_surface (
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  rel_path TEXT NOT NULL,
  surface_sha TEXT NOT NULL,
  defs_json TEXT NOT NULL,
  ref_bloom BLOB,
  config_ctx TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, rel_path)
);
CREATE TABLE IF NOT EXISTS index_coverage (
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  rel_path TEXT NOT NULL,
  kind TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, rel_path, kind)
);
CREATE TABLE IF NOT EXISTS index_coverage_meta (
  project TEXT PRIMARY KEY REFERENCES projects(name) ON DELETE CASCADE,
  generation TEXT NOT NULL,
  index_mode TEXT NOT NULL,
  recorded_at TEXT NOT NULL,
  recording_status TEXT NOT NULL,
  ignored_files_stored INTEGER NOT NULL DEFAULT 0,
  ignored_files_total INTEGER NOT NULL DEFAULT 0,
  coverage_version INTEGER NOT NULL DEFAULT 1,
  hash_records_complete INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS node_vectors (
  node_id INTEGER PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  dimensions INTEGER NOT NULL,
  quantization TEXT NOT NULL,
  scale REAL NOT NULL DEFAULT 1,
  offset REAL NOT NULL DEFAULT 0,
  code_sum INTEGER NOT NULL DEFAULT 0,
  vector BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS token_vectors (
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  token TEXT NOT NULL,
  dimensions INTEGER NOT NULL,
  quantization TEXT NOT NULL,
  idf REAL NOT NULL DEFAULT 0,
  vector BLOB NOT NULL,
  PRIMARY KEY (project, token)
);
CREATE TABLE IF NOT EXISTS unresolved_relationships (
  id INTEGER PRIMARY KEY,
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  source_qn TEXT NOT NULL,
  target_text TEXT NOT NULL,
  type TEXT NOT NULL,
  properties TEXT NOT NULL DEFAULT '{}',
  evidence TEXT NOT NULL DEFAULT '{}',
  UNIQUE(project, source_qn, target_text, type)
);
CREATE INDEX IF NOT EXISTS unresolved_source_idx ON unresolved_relationships(project, source_qn);
CREATE TABLE IF NOT EXISTS communities (
  project TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
  id INTEGER NOT NULL,
  name TEXT NOT NULL,
  hub_node_id INTEGER REFERENCES nodes(id) ON DELETE SET NULL,
  properties TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY (project, id)
);
CREATE TABLE IF NOT EXISTS community_nodes (
  project TEXT NOT NULL,
  community_id INTEGER NOT NULL,
  node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  PRIMARY KEY (project, community_id, node_id),
  FOREIGN KEY (project, community_id) REFERENCES communities(project, id) ON DELETE CASCADE
);
`

const schemaFTSDDL = `
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
  name,
  qualified_name,
  label,
  file_path,
  content='',
  tokenize='unicode61 remove_diacritics 2'
);
`

const schemaIndexDDL = `
CREATE INDEX IF NOT EXISTS idx_nodes_label ON nodes(project, label);
CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(project, name);
CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(project, file_path);
CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(project, source_id, type);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(project, target_id, type);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(project, type);
CREATE INDEX IF NOT EXISTS idx_file_hashes_language ON file_hashes(project, language);
`

const schemaSearchDDL = schemaFTSDDL + schemaIndexDDL

type Store struct {
	db   *sql.DB
	path string
}

type ProjectRecord struct {
	Name           string
	RootPath       string
	Generation     string
	SourceRevision string
	EngineVersion  string
	IndexedAt      time.Time
}

type FileRecord struct {
	Project  string
	Path     string
	SHA256   string
	MTimeNS  int64
	Size     int64
	Language string
}

func OpenWritable(path string) (*Store, error) {
	return openWritable(path, true)
}

func OpenWritableFresh(path string) (*Store, error) {
	return openWritable(path, false)
}

func openWritable(path string, withSearch bool) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.configure(false); err != nil {
		s.Close()
		return nil, err
	}
	if _, err := db.Exec(schemaDDL); err != nil {
		s.Close()
		return nil, fmt.Errorf("initialize graph schema: %w", err)
	}
	if withSearch {
		if _, err := db.Exec(schemaSearchDDL); err != nil {
			s.Close()
			return nil, fmt.Errorf("initialize graph search schema: %w", err)
		}
	}
	meta := map[string]string{
		"schema_version":   fmt.Sprint(api.SchemaVersion),
		"protocol_version": fmt.Sprint(api.ProtocolVersion),
		"asset_revision":   AssetRevision,
	}
	for key, value := range meta {
		if _, err := db.Exec(`INSERT INTO store_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

func OpenReadOnly(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro"
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	s := &Store{db: db, path: path}
	if err := s.configure(true); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.verifySchema(context.Background()); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) configure(readOnly bool) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 10000",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 67108864",
	}
	if !readOnly {
		pragmas = append(pragmas,
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = NORMAL",
			"PRAGMA journal_size_limit = 268435456",
		)
	}
	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("%s: %w", pragma, err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string { return s.path }

type Builder struct {
	tx         *sql.Tx
	insertOnly bool
	nextNodeID int64
	nodeStmt   *sql.Stmt
	ftsStmt    *sql.Stmt
	edgeStmt   *sql.Stmt
	nodeBatch  [][]any
	ftsBatch   [][]any
	edgeBatch  [][]any
}

func (s *Store) Build(ctx context.Context, fn func(*Builder) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	b := &Builder{tx: tx}
	if err := fn(b); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit graph: %w", err)
	}
	return nil
}

func (b *Builder) PutProject(p ProjectRecord) error {
	if p.Name == "" || p.RootPath == "" || p.Generation == "" {
		return errors.New("project name, root path, and generation are required")
	}
	if p.IndexedAt.IsZero() {
		p.IndexedAt = time.Now().UTC()
	}
	_, err := b.tx.Exec(`INSERT INTO projects(name,indexed_at,root_path,generation,source_revision,engine_version)
		VALUES(?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET indexed_at=excluded.indexed_at,
		root_path=excluded.root_path,generation=excluded.generation,
		source_revision=excluded.source_revision,engine_version=excluded.engine_version`,
		p.Name, p.IndexedAt.Format(time.RFC3339Nano), p.RootPath, p.Generation, p.SourceRevision, p.EngineVersion)
	return err
}

func (b *Builder) PutFile(f FileRecord) error {
	_, err := b.tx.Exec(`INSERT INTO file_hashes(project,rel_path,sha256,mtime_ns,size,language)
		VALUES(?,?,?,?,?,?) ON CONFLICT(project,rel_path) DO UPDATE SET sha256=excluded.sha256,
		mtime_ns=excluded.mtime_ns,size=excluded.size,language=excluded.language`,
		f.Project, filepath.ToSlash(f.Path), f.SHA256, f.MTimeNS, f.Size, f.Language)
	return err
}

func (b *Builder) PutNode(n api.Node) (int64, error) {
	props, err := marshalObject(n.Properties)
	if err != nil {
		return 0, err
	}
	if b.insertOnly {
		return b.putNodeInsert(n, props)
	}
	var oldID int64
	var oldName, oldQN, oldLabel, oldFile string
	oldErr := b.tx.QueryRow(`SELECT id,name,qualified_name,label,file_path FROM nodes WHERE project=? AND qualified_name=?`,
		n.Project, n.QualifiedName).Scan(&oldID, &oldName, &oldQN, &oldLabel, &oldFile)
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return 0, oldErr
	}
	if oldErr == nil {
		if err := b.deleteFTSNode(oldID, oldName, oldQN, oldLabel, oldFile); err != nil {
			return 0, err
		}
	}
	row := b.tx.QueryRow(`INSERT INTO nodes(project,label,name,qualified_name,file_path,start_line,start_column,end_line,end_column,properties)
		VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(project,qualified_name) DO UPDATE SET
		label=excluded.label,name=excluded.name,file_path=excluded.file_path,start_line=excluded.start_line,
		start_column=excluded.start_column,end_line=excluded.end_line,end_column=excluded.end_column,
		properties=excluded.properties RETURNING id`, n.Project, n.Label, n.Name, n.QualifiedName,
		filepath.ToSlash(n.Location.File), n.Location.StartLine, n.Location.StartColumn,
		n.Location.EndLine, n.Location.EndColumn, props)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	_, err = b.tx.Exec(`INSERT INTO nodes_fts(rowid,name,qualified_name,label,file_path) VALUES(?,?,?,?,?)`,
		id, camelTerms(n.Name), camelTerms(n.QualifiedName), n.Label, filepath.ToSlash(n.Location.File))
	return id, err
}

func (b *Builder) deleteFTSNode(id int64, name, qualifiedName, label, file string) error {
	_, err := b.tx.Exec(`INSERT INTO nodes_fts(nodes_fts,rowid,name,qualified_name,label,file_path)
		VALUES('delete',?,?,?,?,?)`, id, camelTerms(name), camelTerms(qualifiedName), label, filepath.ToSlash(file))
	return err
}

func (b *Builder) PutEdge(e api.Edge) (int64, error) {
	props, err := marshalObject(e.Properties)
	if err != nil {
		return 0, err
	}
	evidence, err := json.Marshal(e.Evidence)
	if err != nil {
		return 0, err
	}
	localName := ""
	if e.Type == "IMPORTS" {
		localName, _ = e.Properties["local_name"].(string)
	}
	if b.insertOnly {
		return b.queueEdgeInsert(e.Project, e.SourceID, e.TargetID, e.Type, props, string(evidence), localName)
	}
	row := b.tx.QueryRow(`INSERT INTO edges(project,source_id,target_id,type,properties,evidence,local_name)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(project,source_id,target_id,type,local_name) DO UPDATE SET
		properties=excluded.properties,evidence=excluded.evidence RETURNING id`,
		e.Project, e.SourceID, e.TargetID, e.Type, props, string(evidence), localName)
	var id int64
	return id, row.Scan(&id)
}

func (b *Builder) PutUnresolved(relationship api.UnresolvedRelationship) error {
	if relationship.Project == "" || relationship.Source == "" || relationship.TargetText == "" || relationship.Type == "" {
		return errors.New("unresolved relationship project, source, target, and type are required")
	}
	properties, err := marshalObject(relationship.Properties)
	if err != nil {
		return err
	}
	evidence, err := json.Marshal(relationship.Evidence)
	if err != nil {
		return err
	}
	_, err = b.tx.Exec(`INSERT INTO unresolved_relationships(project,source_qn,target_text,type,properties,evidence)
		VALUES(?,?,?,?,?,?) ON CONFLICT(project,source_qn,target_text,type) DO UPDATE SET
		properties=excluded.properties,evidence=excluded.evidence`, relationship.Project, relationship.Source,
		relationship.TargetText, relationship.Type, properties, string(evidence))
	return err
}

func (b *Builder) PutSemanticVector(nodeID int64, project string, vector semanticVector) error {
	if nodeID <= 0 || project == "" {
		return errors.New("semantic vector node and project are required")
	}
	normalizeSemantic(&vector)
	// The persisted vector-search representation at the Superopen asset commit
	// is the normalized dense 768-dimensional vector quantized to signed int8.
	// RotSQ is used transiently by semantic-edge candidate scoring, but storing
	// that code here changes observable vector-search ranking.
	encoded := make([]byte, semanticDimensions)
	for dimension, value := range vector {
		if value > 1 {
			value = 1
		} else if value < -1 {
			value = -1
		}
		encoded[dimension] = byte(int8(value * 127))
	}
	result, err := b.tx.Exec(`INSERT INTO node_vectors(node_id,project,dimensions,quantization,scale,offset,code_sum,vector)
		SELECT id,project,?,'int8-unit',1,0,0,? FROM nodes WHERE id=? AND project=?
		ON CONFLICT(node_id) DO UPDATE SET project=excluded.project,dimensions=excluded.dimensions,
		quantization=excluded.quantization,scale=excluded.scale,offset=excluded.offset,
		code_sum=excluded.code_sum,vector=excluded.vector`,
		semanticDimensions, encoded, nodeID, project)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("semantic vector node does not exist in project")
	}
	return nil
}

func (b *Builder) PutSemanticToken(project, token string, vector semanticVector, idf float32) error {
	if project == "" || token == "" {
		return errors.New("semantic token project and token are required")
	}
	normalizeSemantic(&vector)
	encoded := make([]byte, semanticDimensions)
	for dimension, value := range vector {
		scaled := value * 127
		if scaled > 127 {
			scaled = 127
		} else if scaled < -127 {
			scaled = -127
		}
		encoded[dimension] = byte(int8(scaled))
	}
	_, err := b.tx.Exec(`INSERT INTO token_vectors(project,token,dimensions,quantization,idf,vector)
		VALUES(?,?,?,'int8-unit',?,?) ON CONFLICT(project,token) DO UPDATE SET
		dimensions=excluded.dimensions,quantization=excluded.quantization,idf=excluded.idf,vector=excluded.vector`,
		project, token, semanticDimensions, idf, encoded)
	return err
}

func (b *Builder) PutCoverage(project string, coverage api.Coverage) error {
	if _, err := b.tx.Exec(`DELETE FROM index_coverage WHERE project=?`, project); err != nil {
		return err
	}
	for _, row := range coverage.Rows {
		if _, err := b.tx.Exec(`INSERT INTO index_coverage(project,rel_path,kind,detail) VALUES(?,?,?,?)
			ON CONFLICT(project,rel_path,kind) DO UPDATE SET detail=excluded.detail`,
			project, filepath.ToSlash(row.Path), row.Kind, row.Detail); err != nil {
			return err
		}
	}
	recordedAt := time.Now().UTC()
	if coverage.RecordedAt != nil {
		recordedAt = coverage.RecordedAt.UTC()
	}
	_, err := b.tx.Exec(`INSERT INTO index_coverage_meta(project,generation,index_mode,recorded_at,recording_status,coverage_version,hash_records_complete)
		VALUES(?,?,?,?,?,1,?) ON CONFLICT(project) DO UPDATE SET generation=excluded.generation,
		index_mode=excluded.index_mode,recorded_at=excluded.recorded_at,recording_status=excluded.recording_status,
		coverage_version=excluded.coverage_version,hash_records_complete=excluded.hash_records_complete`,
		project, coverage.Generation, coverage.IndexMode, recordedAt.Format(time.RFC3339Nano), coverage.RecordingStatus, coverage.HashRecordsComplete)
	if err != nil {
		return err
	}
	return b.rebuildMissedGraph(project, coverage)
}

// rebuildMissedGraph matches Superopen's derived <project>::missed graph. It is
// deliberately rebuilt from authoritative coverage rows in the same
// transaction, so a failed publication cannot leave coverage and its graph out
// of sync. Entries beginning with not_indexed describe intentional exclusions,
// not extraction failures, and therefore never appear in this graph.
func (b *Builder) rebuildMissedGraph(project string, coverage api.Coverage) error {
	shadow := project + "::missed"
	rows, err := b.tx.Query(`SELECT id,name,qualified_name,label,file_path FROM nodes WHERE project=? ORDER BY id`, shadow)
	if err != nil {
		return err
	}
	type ftsNode struct {
		id                               int64
		name, qualifiedName, label, file string
	}
	var oldNodes []ftsNode
	for rows.Next() {
		var node ftsNode
		if err := rows.Scan(&node.id, &node.name, &node.qualifiedName, &node.label, &node.file); err != nil {
			rows.Close()
			return err
		}
		oldNodes = append(oldNodes, node)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, node := range oldNodes {
		if err := b.deleteFTSNode(node.id, node.name, node.qualifiedName, node.label, node.file); err != nil {
			return err
		}
	}
	if _, err := b.tx.Exec(`DELETE FROM projects WHERE name=?`, shadow); err != nil {
		return err
	}
	failures := make([]api.CoverageRow, 0, len(coverage.Rows))
	for _, row := range coverage.Rows {
		if row.Path == "" || strings.HasPrefix(row.Kind, "not_indexed") {
			continue
		}
		failures = append(failures, row)
	}
	if len(failures) == 0 {
		return nil
	}
	var parent ProjectRecord
	var indexedAt string
	if err := b.tx.QueryRow(`SELECT indexed_at,root_path,generation,source_revision,engine_version FROM projects WHERE name=?`, project).
		Scan(&indexedAt, &parent.RootPath, &parent.Generation, &parent.SourceRevision, &parent.EngineVersion); err != nil {
		return fmt.Errorf("missed graph parent project: %w", err)
	}
	parent.Name = shadow
	parent.IndexedAt, _ = time.Parse(time.RFC3339Nano, indexedAt)
	if err := b.PutProject(parent); err != nil {
		return err
	}
	rootID, err := b.PutNode(api.Node{Project: shadow, Label: "Project", Name: project, QualifiedName: shadow})
	if err != nil {
		return err
	}
	folderIDs := map[string]int64{"": rootID}
	for _, row := range failures {
		rel := filepath.ToSlash(filepath.Clean(row.Path))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(row.Path) {
			continue
		}
		parentID := rootID
		directory := filepath.ToSlash(filepath.Dir(rel))
		if directory != "." {
			prefix := ""
			for _, segment := range strings.Split(directory, "/") {
				if segment == "" || segment == "." {
					continue
				}
				if prefix == "" {
					prefix = segment
				} else {
					prefix += "/" + segment
				}
				if id, ok := folderIDs[prefix]; ok {
					parentID = id
					continue
				}
				id, err := b.PutNode(api.Node{Project: shadow, Label: "Folder", Name: segment,
					QualifiedName: prefix, Location: api.Location{File: prefix}})
				if err != nil {
					return err
				}
				if _, err := b.PutEdge(api.Edge{Project: shadow, SourceID: parentID, TargetID: id, Type: "CONTAINS_FOLDER"}); err != nil {
					return err
				}
				folderIDs[prefix] = id
				parentID = id
			}
		}
		fileID, err := b.PutNode(api.Node{Project: shadow, Label: "File", Name: filepath.Base(rel),
			QualifiedName: rel, Location: api.Location{File: rel},
			Properties: api.Properties{"kind": row.Kind, "detail": row.Detail}})
		if err != nil {
			return err
		}
		if _, err := b.PutEdge(api.Edge{Project: shadow, SourceID: parentID, TargetID: fileID, Type: "CONTAINS_FILE"}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Verify(ctx context.Context) error {
	var check string
	if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return fmt.Errorf("quick_check: %w", err)
	}
	if check != "ok" {
		return fmt.Errorf("quick_check: %s", check)
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("foreign key check failed")
	}
	return s.verifySchema(ctx)
}

func (s *Store) verifySchema(ctx context.Context) error {
	var schema, protocol string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM store_meta WHERE key='schema_version'`).Scan(&schema); err != nil {
		return fmt.Errorf("graph schema metadata: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM store_meta WHERE key='protocol_version'`).Scan(&protocol); err != nil {
		return fmt.Errorf("graph protocol metadata: %w", err)
	}
	if schema != fmt.Sprint(api.SchemaVersion) || protocol != fmt.Sprint(api.ProtocolVersion) {
		return fmt.Errorf("incompatible graph database: schema=%s protocol=%s", schema, protocol)
	}
	return nil
}

func (s *Store) Seal(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "ANALYZE"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	return s.Verify(ctx)
}

func (s *Store) Status(ctx context.Context, project string) (api.Status, error) {
	status := api.Status{
		Engine:       api.EngineName,
		Protocol:     api.ProtocolVersion,
		Schema:       api.SchemaVersion,
		State:        "ready",
		Database:     s.path,
		Capabilities: Capabilities(),
	}
	query := `SELECT name,indexed_at,generation,
		(SELECT count(*) FROM nodes WHERE nodes.project=projects.name),
		(SELECT count(*) FROM edges WHERE edges.project=projects.name),
		(SELECT count(*) FROM file_hashes WHERE file_hashes.project=projects.name),engine_version
		FROM projects`
	args := []any{}
	if project != "" {
		query += " WHERE name=?"
		args = append(args, project)
	} else {
		query += " WHERE substr(name, -8) != '::missed'"
	}
	query += " ORDER BY indexed_at DESC LIMIT 1"
	var indexed string
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&status.Project, &indexed, &status.Generation,
		&status.NodeCount, &status.EdgeCount, &status.FileCount, &status.EngineVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			status.State = "missing"
			return status, nil
		}
		return api.Status{}, err
	}
	if parsed, err := time.Parse(time.RFC3339Nano, indexed); err == nil {
		status.IndexedAt = &parsed
	}
	return status, nil
}

func (s *Store) Search(ctx context.Context, req api.SearchRequest) (api.SearchResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	semanticOnly := len(req.SemanticQuery) > 0 && !hasLexicalSearchFilters(req)
	if semanticOnly {
		semantic, err := s.SemanticSearchTerms(ctx, req.Project, req.SemanticQuery, req.Limit)
		if err != nil {
			return api.SearchResult{}, err
		}
		return api.SearchResult{Semantic: semantic, Page: api.Page{Limit: normalizedSearchLimit(req.Limit), Total: len(semantic)}}, nil
	}
	result, err := s.searchLexical(ctx, req)
	if err != nil || len(req.SemanticQuery) == 0 {
		return result, err
	}
	result.Semantic, err = s.SemanticSearchTerms(ctx, req.Project, req.SemanticQuery, req.Limit)
	return result, err
}

func (s *Store) searchLexical(ctx context.Context, req api.SearchRequest) (api.SearchResult, error) {
	limit := normalizedSearchLimit(req.Limit)
	terms := ftsQuery(req.Query)
	if terms == "" {
		return s.searchPattern(ctx, req, limit)
	}
	return s.searchFTS(ctx, req, limit, terms)
}

func normalizedSearchLimit(limit int) int {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}

func hasLexicalSearchFilters(req api.SearchRequest) bool {
	return req.Query != "" || req.NamePattern != "" || req.QualifiedNamePattern != "" || req.FilePattern != "" ||
		len(req.Labels) > 0 || len(req.Languages) > 0 || req.PathPrefix != "" || req.Relationship != "" ||
		req.MinDegree != nil || req.MaxDegree != nil || req.ExcludeEntryPoints
}

func (s *Store) searchFTS(ctx context.Context, req api.SearchRequest, limit int, terms string) (api.SearchResult, error) {
	where := []string{"nodes_fts MATCH ?"}
	args := []any{terms}
	if req.Project != "" {
		where = append(where, "n.project=?")
		args = append(args, req.Project)
	}
	if req.PathPrefix != "" {
		where = append(where, "n.file_path LIKE ? ESCAPE '\\'")
		args = append(args, escapeLike(filepath.ToSlash(req.PathPrefix))+"%")
	}
	where = append(where, "n.label NOT IN ('File','Folder','Module','Section','Variable','Project')")
	if req.FilePattern != "" {
		fileLike := globToLike(req.FilePattern)
		if !strings.ContainsAny(req.FilePattern, "*?") {
			fileLike = "%" + fileLike + "%"
		}
		where = append(where, "n.file_path LIKE ?")
		args = append(args, fileLike)
	}
	generation := ""
	if req.Project != "" {
		_ = s.db.QueryRowContext(ctx, `SELECT generation FROM projects WHERE name=?`, req.Project).Scan(&generation)
	}
	fingerprint := searchFingerprint(req)
	offset := 0
	if req.Cursor != "" {
		cursor, err := decodeSearchCursor(req.Cursor)
		if err != nil || cursor.Fingerprint != fingerprint {
			return api.SearchResult{}, errors.New("invalid search cursor")
		}
		if cursor.Generation != generation {
			return api.SearchResult{}, errors.New("stale search cursor")
		}
		offset = cursor.Offset
	}
	countArgs := append([]any(nil), args...)
	var total int
	countQuery := `SELECT count(*) FROM nodes_fts JOIN nodes n ON n.id=nodes_fts.rowid WHERE ` + strings.Join(where, " AND ")
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return api.SearchResult{}, err
	}
	args = append(args, limit+1, offset)
	boost := `CASE n.label WHEN 'Function' THEN 10 WHEN 'Method' THEN 10 WHEN 'Route' THEN 8 WHEN 'Class' THEN 5 WHEN 'Interface' THEN 5 ELSE 0 END`
	query := `SELECT n.id,n.project,n.label,n.name,n.qualified_name,n.file_path,
		n.start_line,n.start_column,n.end_line,n.end_column,n.properties,-bm25(nodes_fts,8.0,5.0,2.0,1.0)+` + boost + ` score
		FROM nodes_fts JOIN nodes n ON n.id=nodes_fts.rowid WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY score DESC,n.qualified_name LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return api.SearchResult{}, err
	}
	defer rows.Close()
	result := api.SearchResult{Page: api.Page{Limit: limit, Cursor: req.Cursor, Total: total}}
	for rows.Next() {
		var n api.RankedNode
		var props string
		if err := rows.Scan(&n.ID, &n.Project, &n.Label, &n.Name, &n.QualifiedName, &n.Location.File,
			&n.Location.StartLine, &n.Location.StartColumn, &n.Location.EndLine, &n.Location.EndColumn,
			&props, &n.Score); err != nil {
			return api.SearchResult{}, err
		}
		result.Matches = append(result.Matches, n)
	}
	if err := rows.Err(); err != nil {
		return api.SearchResult{}, err
	}
	if len(result.Matches) > limit {
		result.Matches = result.Matches[:limit]
		result.Page.Truncated = true
	}
	if req.Budget > 0 {
		used := 0
		kept := result.Matches[:0]
		for _, match := range result.Matches {
			cost := 12 + (len(match.QualifiedName)+len(match.Location.File)+3)/4
			if len(kept) > 0 && used+cost > req.Budget {
				result.Budget.Truncated = true
				break
			}
			used += cost
			kept = append(kept, match)
		}
		result.Matches = kept
		result.Budget.RequestedTokens = req.Budget
		result.Budget.ReturnedTokens = used
	}
	nextOffset := offset + len(result.Matches)
	result.Page.Truncated = result.Page.Truncated || result.Budget.Truncated || nextOffset < total
	if nextOffset < total {
		result.Page.NextCursor = encodeSearchCursor(searchCursor{Version: 1, Offset: nextOffset, Fingerprint: fingerprint, Generation: generation})
	}
	return result, nil
}

type searchCursor struct {
	Version     int    `json:"v"`
	Offset      int    `json:"o"`
	Fingerprint string `json:"f"`
	Generation  string `json:"g"`
}

func searchFingerprint(req api.SearchRequest) string {
	copy := req
	copy.Cursor = ""
	copy.Limit = 0
	body, _ := json.Marshal(copy)
	hash := sha256.Sum256(body)
	return fmt.Sprintf("%x", hash[:16])
}

func encodeSearchCursor(cursor searchCursor) string {
	body, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeSearchCursor(value string) (searchCursor, error) {
	var cursor searchCursor
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor, err
	}
	if err := json.Unmarshal(body, &cursor); err != nil {
		return cursor, err
	}
	if cursor.Version != 1 || cursor.Offset < 0 || cursor.Fingerprint == "" {
		return searchCursor{}, errors.New("invalid cursor fields")
	}
	return cursor, nil
}

func (s *Store) Projects(ctx context.Context) ([]api.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name,root_path,indexed_at,generation,
		(SELECT count(*) FROM nodes WHERE nodes.project=projects.name),
		(SELECT count(*) FROM edges WHERE edges.project=projects.name) FROM projects
		WHERE substr(name, -8) != '::missed' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []api.Project
	for rows.Next() {
		var p api.Project
		var indexed string
		if err := rows.Scan(&p.Name, &p.RootPath, &indexed, &p.Generation, &p.NodeCount, &p.EdgeCount); err != nil {
			return nil, err
		}
		p.Database = s.path
		p.State = "ready"
		p.IndexedAt, _ = time.Parse(time.RFC3339Nano, indexed)
		result = append(result, p)
	}
	return result, rows.Err()
}

func marshalObject(v any) (string, error) {
	// Typed nil maps (e.g. api.Properties(nil)) are non-nil interfaces, so
	// json.Marshal would emit "null". Superopen stores empty objects as "{}".
	if v == nil {
		return "{}", nil
	}
	switch value := v.(type) {
	case api.Properties:
		if value == nil {
			return "{}", nil
		}
	case map[string]any:
		if value == nil {
			return "{}", nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func camelTerms(input string) string {
	if input == "" {
		return ""
	}
	var split strings.Builder
	runes := []rune(input)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) ||
			(i+1 < len(runes) && unicode.IsUpper(runes[i-1]) && unicode.IsLower(runes[i+1]))) {
			split.WriteByte(' ')
		}
		split.WriteRune(r)
	}
	return input + " " + split.String()
}

var searchToken = regexp.MustCompile(`[\pL\pN_]+`)

func ftsQuery(input string) string {
	tokens := searchToken.FindAllString(input, -1)
	seen := map[string]bool{}
	filtered := tokens[:0]
	for _, token := range tokens {
		token = strings.ToLower(token)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		filtered = append(filtered, `"`+strings.ReplaceAll(token, `"`, `""`)+`"*`)
	}
	sort.Strings(filtered)
	return strings.Join(filtered, " OR ")
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

func globToLike(pattern string) string {
	var result strings.Builder
	result.Grow(len(pattern))
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				value := result.String()
				if strings.HasSuffix(value, "/") {
					result.Reset()
					result.WriteString(strings.TrimSuffix(value, "/"))
				}
				result.WriteByte('%')
				index++
				if index+1 < len(pattern) && pattern[index+1] == '/' {
					index++
				}
			} else {
				result.WriteByte('%')
			}
		case '?':
			result.WriteByte('_')
		default:
			result.WriteByte(pattern[index])
		}
	}
	return result.String()
}
