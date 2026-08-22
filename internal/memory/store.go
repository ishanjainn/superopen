package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	_ "modernc.org/sqlite"
)

const (
	KindPrompt      = "prompt"
	KindTool        = "tool"
	KindWorking     = "working"
	KindSession     = "session"
	KindPin         = "pin"
	KindTeaching    = "teaching"
	KindObservation = "observation"

	SourceSpan     = "span"
	SourceAgent    = "agent"
	SourceTeach    = "teach"
	SourceHeadless = "headless"
	SourceObserver = "observer"

	// ObservationType* taxonomy values stored on topic.
	ObservationDecision  = "decision"
	ObservationBugfix    = "bugfix"
	ObservationFeature   = "feature"
	ObservationRefactor  = "refactor"
	ObservationDiscovery = "discovery"
	ObservationChange    = "change"

	EdgeContradicts  = "contradicts"
	EdgeSameSession  = "same_session"
	EdgeRolledUpFrom = "rolled_up_from"
	EdgeTaughtFrom   = "taught_from"
	EdgeNext         = "next"

	metaEmbedder        = "embedder_id"
	metaPending         = "pending_distill"
	metaDistillPaused   = "distill_paused"
	quantizationInt8    = "int8-unit"
	memorySchemaVersion = "1"
)

const memoryDDL = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS memory_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_episodes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uid TEXT NOT NULL UNIQUE,
  session_id TEXT NOT NULL DEFAULT '',
  span_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'span',
  title TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  files TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  tokens INTEGER NOT NULL DEFAULT 0,
  pinned INTEGER NOT NULL DEFAULT 0,
  faded INTEGER NOT NULL DEFAULT 0,
  embedding_pending INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  valid_from TEXT NOT NULL DEFAULT '',
  valid_to TEXT NOT NULL DEFAULT '',
  faded_at TEXT NOT NULL DEFAULT '',
  last_accessed_at TEXT NOT NULL DEFAULT '',
  community_id TEXT NOT NULL DEFAULT '',
  centrality REAL NOT NULL DEFAULT 0,
  tier TEXT NOT NULL DEFAULT '',
  never_decay INTEGER NOT NULL DEFAULT 0,
  tags TEXT NOT NULL DEFAULT '',
  fading INTEGER NOT NULL DEFAULT 0,
  topic TEXT NOT NULL DEFAULT '',
  facts TEXT NOT NULL DEFAULT '',
  narrative TEXT NOT NULL DEFAULT '',
  concepts TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS memory_vectors (
  episode_id INTEGER PRIMARY KEY REFERENCES memory_episodes(id) ON DELETE CASCADE,
  embedder_id TEXT NOT NULL,
  dimensions INTEGER NOT NULL,
  quantization TEXT NOT NULL,
  vector BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_edges (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id INTEGER NOT NULL REFERENCES memory_episodes(id) ON DELETE CASCADE,
  target_id INTEGER NOT NULL REFERENCES memory_episodes(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  weight REAL NOT NULL DEFAULT 1,
  updated_at TEXT NOT NULL DEFAULT '',
  UNIQUE(source_id, target_id, type)
);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_episodes_fts USING fts5(
  title, text, files, tool_name,
  content='memory_episodes',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE TRIGGER IF NOT EXISTS memory_episodes_ai AFTER INSERT ON memory_episodes BEGIN
  INSERT INTO memory_episodes_fts(rowid, title, text, files, tool_name)
  VALUES (new.id, new.title, new.text, new.files, new.tool_name);
END;
CREATE TRIGGER IF NOT EXISTS memory_episodes_ad AFTER DELETE ON memory_episodes BEGIN
  INSERT INTO memory_episodes_fts(memory_episodes_fts, rowid, title, text, files, tool_name)
  VALUES ('delete', old.id, old.title, old.text, old.files, old.tool_name);
END;
CREATE TRIGGER IF NOT EXISTS memory_episodes_au AFTER UPDATE ON memory_episodes BEGIN
  INSERT INTO memory_episodes_fts(memory_episodes_fts, rowid, title, text, files, tool_name)
  VALUES ('delete', old.id, old.title, old.text, old.files, old.tool_name);
  INSERT INTO memory_episodes_fts(rowid, title, text, files, tool_name)
  VALUES (new.id, new.title, new.text, new.files, new.tool_name);
END;
CREATE TABLE IF NOT EXISTS memory_topics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  label TEXT NOT NULL,
  community INTEGER NOT NULL,
  episode_ids TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_economy (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  packs_served INTEGER NOT NULL DEFAULT 0,
  tokens_injected INTEGER NOT NULL DEFAULT 0,
  fallback_searches INTEGER NOT NULL DEFAULT 0,
  tokens_saved INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS memory_episodes_session ON memory_episodes(session_id, created_at);
CREATE INDEX IF NOT EXISTS memory_episodes_kind ON memory_episodes(kind, created_at);
CREATE INDEX IF NOT EXISTS memory_episodes_topic ON memory_episodes(topic, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS memory_episodes_content_hash ON memory_episodes(content_hash) WHERE content_hash != '';
CREATE TABLE IF NOT EXISTS memory_shapes (
  episode_id INTEGER PRIMARY KEY REFERENCES memory_episodes(id) ON DELETE CASCADE,
  blob BLOB NOT NULL
);
`

type Store struct {
	db   *sql.DB
	path string
	key  []byte
}

type Episode struct {
	ID               int64    `json:"id"`
	UID              string   `json:"uid"`
	SessionID        string   `json:"session_id,omitempty"`
	SpanID           string   `json:"span_id,omitempty"`
	Kind             string   `json:"kind"`
	Source           string   `json:"source"`
	Title            string   `json:"title"`
	Text             string   `json:"text,omitempty"`
	Files            []string `json:"files,omitempty"`
	ToolName         string   `json:"tool_name,omitempty"`
	Tokens           int      `json:"tokens"`
	Pinned           bool     `json:"pinned"`
	Faded            bool     `json:"faded"`
	Fading           bool     `json:"fading,omitempty"`
	EmbeddingPending bool     `json:"embedding_pending,omitempty"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	ValidFrom        string   `json:"valid_from,omitempty"`
	ValidTo          string   `json:"valid_to,omitempty"`
	CommunityID      string   `json:"community_id,omitempty"`
	Centrality       float64  `json:"centrality,omitempty"`
	Tier             string   `json:"tier,omitempty"`
	NeverDecay       bool     `json:"never_decay,omitempty"`
	Tags             string   `json:"tags,omitempty"`
	Topic            string   `json:"topic,omitempty"`
	Facts            []string `json:"facts,omitempty"`
	Narrative        string   `json:"narrative,omitempty"`
	Concepts         []string `json:"concepts,omitempty"`
	ContentHash      string   `json:"content_hash,omitempty"`
	Score            float64  `json:"score,omitempty"`
}

type Hit struct {
	Episode
	Snippet string `json:"snippet,omitempty"`
}

type Economy struct {
	PacksServed      int    `json:"packs_served"`
	TokensInjected   int    `json:"tokens_injected"`
	FallbackSearches int    `json:"fallback_searches"`
	TokensSaved      int    `json:"tokens_saved"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

type Topic struct {
	ID         int64   `json:"id"`
	Label      string  `json:"label"`
	Community  int     `json:"community"`
	EpisodeIDs []int64 `json:"episode_ids"`
	Size       int     `json:"size"`
	UpdatedAt  string  `json:"updated_at"`
}

type MemoryCounts struct {
	Episodic   int `json:"episodic"`
	Semantic   int `json:"semantic"`
	Procedural int `json:"procedural"`
	Working    int `json:"working"`
	Tombstoned int `json:"tombstoned"`
	Edges      int `json:"edges"`
	Pins       int `json:"pins"`
	Fading     int `json:"fading"`
}

type ActivityBucket struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

type Status struct {
	Episodes       int              `json:"episodes"`
	Vectors        int              `json:"vectors"`
	Edges          int              `json:"edges"`
	Topics         int              `json:"topics"`
	Teachings      int              `json:"teachings"`
	Pins           int              `json:"pins"`
	Faded          int              `json:"faded"`
	Fading         int              `json:"fading"`
	RolledUp       int              `json:"rolled_up"`
	PendingDistill []string         `json:"pending_distill"`
	DistillPaused  bool             `json:"distill_paused"`
	EmbedderID     string           `json:"embedder_id"`
	RolledUpPct    float64          `json:"rolled_up_pct"`
	FadePct        float64          `json:"fade_pct"`
	EdgeDensity    float64          `json:"edge_density"`
	Coverage       float64          `json:"coverage"`
	Live           int              `json:"live"`
	Lifecycle      string           `json:"lifecycle"`
	KnowledgePct   float64          `json:"knowledge_pct"`
	Connected      float64          `json:"connected"`
	CleanedPct     float64          `json:"cleaned_pct"`
	Counts         MemoryCounts     `json:"counts"`
	Activity       []ActivityBucket `json:"activity,omitempty"`
	ActivityPeak   int              `json:"activity_peak"`
	Economy        Economy          `json:"economy"`
	SchemaVersion  string           `json:"schema_version"`
	TopicsDetail   []Topic          `json:"topics_detail,omitempty"`
}

type CaptureInput struct {
	SessionID    string
	Kind         string
	Source       string
	Title        string
	Text         string
	Files        []string
	ToolName     string
	ContradictOf int64
	Pin          bool
	Topic        string
	Facts        []string
	Narrative    string
	Concepts     []string
}

func OpenRoot(root string) (*Store, error) {
	layout := paths.Resolve(root)
	if !layout.Exists() {
		return nil, fmt.Errorf("%s", paths.UnmanagedMessage)
	}
	if err := layout.EnsureDirs(); err != nil {
		return nil, err
	}
	return Open(layout.Database)
}

// HasEpisodes is true when this repository's store already has at least one
// memory moment or rollup. Used to keep cold-clone tool lists graph-only.
func HasEpisodes(root string) bool {
	layout := paths.Resolve(root)
	store, err := OpenQuick(layout.Database)
	if err != nil {
		return false
	}
	defer store.Close()
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM memory_episodes`).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func Open(path string) (*Store, error) {
	return open(path, 10000)
}

func OpenQuick(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return open(path, 800)
}

func open(path string, busyMs int) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		fmt.Sprintf("PRAGMA busy_timeout = %d", busyMs),
		"PRAGMA temp_store = MEMORY",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			s.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(memoryDDL); err != nil {
		s.Close()
		return nil, fmt.Errorf("initialize memory schema: %w", err)
	}
	if err := s.ensureKnobs(); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.ensureKey(); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.ensureEmbedder(); err != nil {
		s.Close()
		return nil, err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO memory_economy(id, packs_served, tokens_injected, fallback_searches, tokens_saved, updated_at) VALUES(1,0,0,0,0,?)`, nowRFC()); err != nil {
		s.Close()
		return nil, err
	}
	_ = s.setMeta("schema_version", memorySchemaVersion)
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ensureEmbedder() error {
	existing, _ := s.meta(metaEmbedder)
	if existing == "" {
		return s.setMeta(metaEmbedder, CurrentEmbedder())
	}
	if existing != CurrentEmbedder() {
		return fmt.Errorf("refuse mixed embedder generations: store %s process %s", existing, CurrentEmbedder())
	}
	return nil
}

func (s *Store) meta(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM memory_meta WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) setMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO memory_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) upsertEpisode(ep Episode, vector Vector, embed bool) (int64, bool, error) {
	now := nowRFC()
	if ep.CreatedAt == "" {
		ep.CreatedAt = now
	}
	ep.UpdatedAt = now
	if ep.ValidFrom == "" {
		ep.ValidFrom = now
	}
	files := strings.Join(normalizeFiles(ep.Files), "\n")
	pending := 1
	if embed && !isZero(vector) {
		pending = 0
	}
	plain := ep.Text
	stored := s.sealText(ep.UID, plain)
	facts := marshalStringList(ep.Facts)
	concepts := marshalStringList(ep.Concepts)
	res, err := s.db.Exec(`
INSERT INTO memory_episodes(uid,session_id,span_id,kind,source,title,text,files,tool_name,tokens,pinned,faded,embedding_pending,created_at,updated_at,valid_from,valid_to,faded_at,last_accessed_at,community_id,centrality,tier,never_decay,tags,fading,topic,facts,narrative,concepts,content_hash)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(uid) DO NOTHING`,
		ep.UID, ep.SessionID, ep.SpanID, ep.Kind, ep.Source, ep.Title, stored, files, ep.ToolName,
		ep.Tokens, boolInt(ep.Pinned), boolInt(ep.Faded), pending, ep.CreatedAt, ep.UpdatedAt, ep.ValidFrom, ep.ValidTo, "", "",
		ep.CommunityID, ep.Centrality, ep.Tier, boolInt(ep.NeverDecay), ep.Tags, boolInt(ep.Fading),
		ep.Topic, facts, ep.Narrative, concepts, ep.ContentHash)
	if err != nil {
		if ep.ContentHash != "" {
			var id int64
			if err2 := s.db.QueryRow(`SELECT id FROM memory_episodes WHERE content_hash=?`, ep.ContentHash).Scan(&id); err2 == nil {
				return id, false, nil
			}
		}
		return 0, false, err
	}
	n, _ := res.RowsAffected()
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM memory_episodes WHERE uid=?`, ep.UID).Scan(&id); err != nil {
		if ep.ContentHash != "" {
			if err2 := s.db.QueryRow(`SELECT id FROM memory_episodes WHERE content_hash=?`, ep.ContentHash).Scan(&id); err2 == nil {
				return id, false, nil
			}
		}
		return 0, false, err
	}
	inserted := n > 0
	if embed && !isZero(vector) {
		if err := s.writeVector(id, vector); err != nil {
			return id, inserted, err
		}
		_, _ = s.db.Exec(`UPDATE memory_episodes SET embedding_pending=0, updated_at=? WHERE id=?`, now, id)
	}
	return id, inserted, nil
}

func (s *Store) writeVector(id int64, vector Vector) error {
	_, err := s.db.Exec(`
INSERT INTO memory_vectors(episode_id,embedder_id,dimensions,quantization,vector)
VALUES(?,?,?,?,?)
ON CONFLICT(episode_id) DO UPDATE SET embedder_id=excluded.embedder_id, dimensions=excluded.dimensions, quantization=excluded.quantization, vector=excluded.vector`,
		id, CurrentEmbedder(), embedDimensions, quantizationInt8, vector.Bytes())
	return err
}

func (s *Store) addEdge(source, target int64, edgeType string) error {
	return s.addWeightedEdge(source, target, edgeType, 1)
}

func (s *Store) addWeightedEdge(source, target int64, edgeType string, weight float64) error {
	if source == 0 || target == 0 || source == target {
		return nil
	}
	if weight <= 0 {
		weight = 1
	}
	now := nowRFC()
	_, err := s.db.Exec(`
INSERT INTO memory_edges(source_id,target_id,type,weight,updated_at) VALUES(?,?,?,?,?)
ON CONFLICT(source_id,target_id,type) DO UPDATE SET weight=memory_edges.weight+excluded.weight, updated_at=excluded.updated_at`,
		source, target, edgeType, weight, now)
	return err
}

func (s *Store) Get(id int64) (Episode, error) {
	ep, err := s.scanOne(`SELECT `+episodeCols+` FROM memory_episodes WHERE id=?`, id)
	if err != nil {
		return ep, err
	}
	now := nowRFC()
	_, _ = s.db.Exec(`UPDATE memory_episodes SET last_accessed_at=? WHERE id=?`, now, id)
	return ep, nil
}

func (s *Store) GetMany(ids []int64) ([]Episode, error) {
	out := make([]Episode, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		ep, err := s.Get(id)
		if err != nil {
			return out, err
		}
		out = append(out, ep)
	}
	return out, nil
}

func (s *Store) Pin(id int64) error {
	_, err := s.db.Exec(`UPDATE memory_episodes SET pinned=1, faded=0, fading=0, never_decay=1, faded_at='', updated_at=? WHERE id=?`, nowRFC(), id)
	return err
}

func (s *Store) Fade(id int64) error {
	now := nowRFC()
	_, err := s.db.Exec(`UPDATE memory_episodes SET fading=1, faded=0, faded_at=?, updated_at=? WHERE id=? AND pinned=0`, now, now, id)
	return err
}

func (s *Store) Rescue(id int64) error {
	_, err := s.db.Exec(`UPDATE memory_episodes SET faded=0, fading=0, faded_at='', updated_at=? WHERE id=?`, nowRFC(), id)
	return err
}

func (s *Store) PendingDistill() []string {
	raw, _ := s.meta(metaPending)
	return splitCSV(raw)
}

func (s *Store) MarkPending(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, id := range s.PendingDistill() {
		set[id] = struct{}{}
	}
	set[sessionID] = struct{}{}
	return s.setMeta(metaPending, joinCSV(setKeys(set)))
}

func (s *Store) ClearPending(sessionID string) error {
	set := map[string]struct{}{}
	for _, id := range s.PendingDistill() {
		if id != sessionID {
			set[id] = struct{}{}
		}
	}
	return s.setMeta(metaPending, joinCSV(setKeys(set)))
}

func (s *Store) DistillPaused() bool {
	v, _ := s.meta(metaDistillPaused)
	return v == "1" || strings.EqualFold(v, "true")
}

func (s *Store) SetDistillPaused(paused bool) error {
	if paused {
		return s.setMeta(metaDistillPaused, "1")
	}
	return s.setMeta(metaDistillPaused, "0")
}

func (s *Store) HasSessionRollup(sessionID string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE session_id=? AND kind=? AND faded=0`, sessionID, KindSession).Scan(&n)
	return n > 0
}

func (s *Store) HasKindPrompt(sessionID string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE session_id=? AND kind=? AND faded=0`, sessionID, KindPrompt).Scan(&n)
	return n > 0
}

func (s *Store) LatestSessionKind(kind string) (Episode, error) {
	return s.scanOne(`SELECT `+episodeCols+` FROM memory_episodes WHERE kind=? AND faded=0 ORDER BY created_at DESC, id DESC LIMIT 1`, kind)
}

func (s *Store) Status() (Status, error) {
	st := Status{EmbedderID: CurrentEmbedder(), SchemaVersion: memorySchemaVersion, PendingDistill: s.PendingDistill(), DistillPaused: s.DistillPaused()}
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes`).Scan(&st.Episodes)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_vectors`).Scan(&st.Vectors)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_edges`).Scan(&st.Edges)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_topics`).Scan(&st.Topics)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE kind=?`, KindTeaching).Scan(&st.Teachings)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE pinned=1 AND faded=0`).Scan(&st.Pins)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE faded=1`).Scan(&st.Faded)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE fading=1 AND faded=0`).Scan(&st.Fading)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE kind=?`, KindSession).Scan(&st.RolledUp)
	liveSQL := `faded=0 AND kind != 'tool'`
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE `+liveSQL+` AND tier=?`, tierEpisodic).Scan(&st.Counts.Episodic)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE `+liveSQL+` AND tier=?`, tierSemantic).Scan(&st.Counts.Semantic)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE `+liveSQL+` AND tier=?`, tierProcedural).Scan(&st.Counts.Procedural)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE `+liveSQL+` AND tier=?`, tierWorking).Scan(&st.Counts.Working)
	st.Counts.Tombstoned = st.Faded
	st.Counts.Edges = st.Edges
	st.Counts.Pins = st.Pins
	st.Counts.Fading = st.Fading
	st.Live = st.Counts.Episodic + st.Counts.Semantic + st.Counts.Procedural
	st.Coverage = s.knowledgeCoverage()
	if st.Live > 0 {
		if st.Coverage == 0 && st.Counts.Semantic > 0 {
			st.Coverage = float64(st.Counts.Semantic) / float64(st.Live)
		}
		st.Connected = float64(st.Edges) / float64(st.Live)
		st.KnowledgePct = 100 * st.Coverage
		st.EdgeDensity = st.Connected
	}
	total := st.Live + st.Faded
	if total > 0 {
		st.CleanedPct = 100 * float64(st.Faded) / float64(total)
		st.FadePct = st.CleanedPct
	}
	if st.Episodes > 0 {
		st.RolledUpPct = 100 * float64(st.RolledUp) / float64(st.Episodes)
	}
	st.Lifecycle = "awake"
	if st.DistillPaused {
		st.Lifecycle = "resting"
	} else if len(st.PendingDistill) > 0 {
		st.Lifecycle = "sorting"
	}
	st.Activity, st.ActivityPeak = s.activitySeries(14)
	st.Economy, _ = s.ReadEconomy()
	st.TopicsDetail, _ = s.ListTopics()
	return st, nil
}

func (s *Store) knowledgeCoverage() float64 {
	var epi int
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE faded=0 AND kind != 'tool' AND tier=?`, tierEpisodic).Scan(&epi)
	if epi == 0 {
		return 0
	}
	var covered int
	_ = s.db.QueryRow(`
SELECT count(DISTINCT epi.id)
FROM memory_episodes epi
JOIN memory_edges e ON e.type=? AND (e.source_id=epi.id OR e.target_id=epi.id)
JOIN memory_episodes sem ON sem.faded=0 AND sem.kind=? AND (sem.id=e.source_id OR sem.id=e.target_id) AND sem.id!=epi.id
WHERE epi.faded=0 AND epi.kind != 'tool' AND epi.tier=?`, EdgeRolledUpFrom, KindSession, tierEpisodic).Scan(&covered)
	return float64(covered) / float64(epi)
}

func (s *Store) activitySeries(days int) ([]ActivityBucket, int) {
	if days <= 0 {
		days = 14
	}
	rows, err := s.db.Query(`
SELECT substr(created_at,1,10) AS day, count(*)
FROM memory_episodes
WHERE kind != 'tool' AND created_at >= datetime('now', ?)
GROUP BY day ORDER BY day`, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, 0
	}
	defer rows.Close()
	var out []ActivityBucket
	peak := 0
	for rows.Next() {
		var b ActivityBucket
		if rows.Scan(&b.Day, &b.Count) != nil {
			continue
		}
		out = append(out, b)
		if b.Count > peak {
			peak = b.Count
		}
	}
	return out, peak
}

func (s *Store) ReadEconomy() (Economy, error) {
	var e Economy
	err := s.db.QueryRow(`SELECT packs_served,tokens_injected,fallback_searches,tokens_saved,updated_at FROM memory_economy WHERE id=1`).
		Scan(&e.PacksServed, &e.TokensInjected, &e.FallbackSearches, &e.TokensSaved, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return Economy{}, nil
	}
	return e, err
}

func (s *Store) RecordPack(tokensInjected, tokensSaved int) error {
	_, err := s.db.Exec(`
UPDATE memory_economy SET packs_served=packs_served+1, tokens_injected=tokens_injected+?, tokens_saved=tokens_saved+?, updated_at=? WHERE id=1`,
		tokensInjected, tokensSaved, nowRFC())
	return err
}

func (s *Store) RecordSearch() error {
	_, err := s.db.Exec(`UPDATE memory_economy SET fallback_searches=fallback_searches+1, updated_at=? WHERE id=1`, nowRFC())
	return err
}

func (s *Store) ListTopics() ([]Topic, error) {
	rows, err := s.db.Query(`SELECT id,label,community,episode_ids,size,updated_at FROM memory_topics ORDER BY size DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Topic
	for rows.Next() {
		var t Topic
		var raw string
		if err := rows.Scan(&t.ID, &t.Label, &t.Community, &raw, &t.Size, &t.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &t.EpisodeIDs)
		out = append(out, t)
	}
	return out, rows.Err()
}

const episodeCols = `id,uid,session_id,span_id,kind,source,title,text,files,tool_name,tokens,pinned,faded,embedding_pending,created_at,updated_at,valid_from,valid_to,community_id,centrality,tier,never_decay,tags,fading,topic,facts,narrative,concepts,content_hash`

type scanner interface {
	Scan(dest ...any) error
}

func scanEpisode(row scanner) (Episode, error) {
	return scanEpisodeWithKey(nil, row)
}

func (s *Store) scanOne(query string, args ...any) (Episode, error) {
	row := s.db.QueryRow(query, args...)
	ep, err := scanEpisodeWithKey(s, row)
	if err == sql.ErrNoRows {
		return Episode{}, fmt.Errorf("memory episode not found")
	}
	return ep, err
}

func scanEpisodes(rows *sql.Rows) ([]Episode, error) {
	return scanEpisodesKeyed(nil, rows)
}

func (s *Store) scanEpisodes(rows *sql.Rows) ([]Episode, error) {
	return scanEpisodesKeyed(s, rows)
}

func scanEpisodesKeyed(s *Store, rows *sql.Rows) ([]Episode, error) {
	defer rows.Close()
	var out []Episode
	for rows.Next() {
		ep, err := scanEpisodeWithKey(s, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

func scanEpisodeWithKey(s *Store, row scanner) (Episode, error) {
	var ep Episode
	var files, facts, concepts string
	var pinned, faded, pending, neverDecay, fading int
	if err := row.Scan(
		&ep.ID, &ep.UID, &ep.SessionID, &ep.SpanID, &ep.Kind, &ep.Source, &ep.Title, &ep.Text, &files, &ep.ToolName,
		&ep.Tokens, &pinned, &faded, &pending, &ep.CreatedAt, &ep.UpdatedAt, &ep.ValidFrom, &ep.ValidTo,
		&ep.CommunityID, &ep.Centrality, &ep.Tier, &neverDecay, &ep.Tags, &fading,
		&ep.Topic, &facts, &ep.Narrative, &concepts, &ep.ContentHash,
	); err != nil {
		return ep, err
	}
	ep.Pinned = pinned != 0
	ep.Faded = faded != 0
	ep.Fading = fading != 0
	ep.EmbeddingPending = pending != 0
	ep.NeverDecay = neverDecay != 0
	if s != nil {
		ep.Text = s.openText(ep.UID, ep.Text)
	}
	if files != "" {
		ep.Files = splitFiles(files)
	}
	ep.Facts = unmarshalStringList(facts)
	ep.Concepts = unmarshalStringList(concepts)
	return ep, nil
}

func nowRFC() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func isZero(v Vector) bool {
	for _, x := range v {
		if x != 0 {
			return false
		}
	}
	return true
}

func splitFiles(s string) []string {
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinCSV(ids []string) string {
	return strings.Join(ids, ",")
}

func setKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

func (s *Store) backfillTags() error {
	rows, err := s.db.Query(`SELECT id, uid, text, tags FROM memory_episodes WHERE tags='' LIMIT 400`)
	if err != nil {
		return err
	}
	type row struct {
		id   int64
		uid  string
		text string
		tags string
	}
	var all []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.uid, &r.text, &r.tags) == nil {
			all = append(all, r)
		}
	}
	_ = rows.Close()
	for _, r := range all {
		plain := s.openText(r.uid, r.text)
		tags := entityTags(plain)
		if tags == "" {
			continue
		}
		_, _ = s.db.Exec(`UPDATE memory_episodes SET tags=? WHERE id=?`, tags, r.id)
	}
	return nil
}

func (s *Store) tableColumns(table string) (map[string]bool, error) {
	return pragmaColumns(s.db, table)
}

func marshalStringList(in []string) string {
	if len(in) == 0 {
		return ""
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return ""
	}
	return string(raw)
}

func unmarshalStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return out
}

// Ping is a cheap health check used by fail-open hook paths.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.QueryRowContext(ctx, `SELECT 1`).Scan(new(int))
}
