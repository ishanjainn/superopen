package harvest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	_ "modernc.org/sqlite"
)

const harvestDDL = `
CREATE TABLE IF NOT EXISTS harvest_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  status TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT '',
  skipped TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS harvest_runs_session ON harvest_runs(session_id, status);

CREATE TABLE IF NOT EXISTS harvest_proposals (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  kind TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  issue TEXT NOT NULL DEFAULT '',
  suggestion TEXT NOT NULL DEFAULT '',
  diff TEXT NOT NULL DEFAULT '',
  base_hash TEXT NOT NULL DEFAULT '',
  base_mtime TEXT NOT NULL DEFAULT '',
  evidence TEXT NOT NULL DEFAULT '[]',
  plus INTEGER NOT NULL DEFAULT 0,
  minus INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS harvest_proposals_status ON harvest_proposals(status, created_at);
CREATE INDEX IF NOT EXISTS harvest_proposals_session ON harvest_proposals(session_id, status);
`

type Store struct {
	db   *sql.DB
	root string
	path string
}

func OpenRoot(root string) (*Store, error) {
	layout := paths.Resolve(root)
	if !layout.Exists() {
		return nil, fmt.Errorf("%s", paths.UnmanagedMessage)
	}
	if err := layout.EnsureDirs(); err != nil {
		return nil, err
	}
	return open(root, layout.Database)
}

func open(root, dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, root: root, path: dbPath}
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			s.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(harvestDDL); err != nil {
		s.Close()
		return nil, fmt.Errorf("initialize harvest schema: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) harvestDir() string {
	return filepath.Join(paths.Resolve(s.root).Root, "harvest")
}

func (s *Store) SuccessfulRun(sessionID string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM harvest_runs WHERE session_id=? AND status=?`, sessionID, StatusProposed).Scan(&n)
	return n > 0
}

// HasAttempt is true when SessionEnd already queued or ran harvest for this session.
func (s *Store) HasAttempt(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM harvest_runs WHERE session_id=?`, sessionID).Scan(&n)
	return n > 0
}

func (s *Store) ProviderFailCount(sessionID, provider string) int {
	sessionID = strings.TrimSpace(sessionID)
	provider = strings.TrimSpace(provider)
	if sessionID == "" || provider == "" {
		return 0
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM harvest_runs WHERE session_id=? AND provider=? AND status IN (?,?)`,
		sessionID, provider, StatusPending, StatusFailed).Scan(&n)
	return n
}

func (s *Store) HasOpenForSession(sessionID string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM harvest_proposals WHERE session_id=? AND status=?`, sessionID, StatusOpen).Scan(&n)
	return n > 0
}

func (s *Store) OpenCount() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM harvest_proposals WHERE status=?`, StatusOpen).Scan(&n)
	return n
}

func (s *Store) PendingSession() string {
	var id string
	err := s.db.QueryRow(`SELECT session_id FROM harvest_runs WHERE status=? ORDER BY id DESC LIMIT 1`, StatusPending).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

// ResolvePending flips every pending harvest_runs row for sessionID to status.
// If none exist, it inserts one so skip/propose still leave a terminal record.
func (s *Store) ResolvePending(sessionID, status, note string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	now := nowRFC()
	res, err := s.db.Exec(`UPDATE harvest_runs SET status=?, skipped=?, updated_at=? WHERE session_id=? AND status=?`,
		status, note, now, sessionID, StatusPending)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	_, err = s.InsertRun(sessionID, status, "", note)
	return err
}

func (s *Store) InsertRun(sessionID, status, provider, skipped string) (int64, error) {
	now := nowRFC()
	res, err := s.db.Exec(`INSERT INTO harvest_runs(session_id,status,provider,skipped,created_at,updated_at) VALUES(?,?,?,?,?,?)`,
		sessionID, status, provider, skipped, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) InsertProposal(p Proposal) (Proposal, error) {
	now := nowRFC()
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = StatusOpen
	}
	ev, _ := json.Marshal(p.Evidence)
	if len(ev) == 0 {
		ev = []byte("[]")
	}
	res, err := s.db.Exec(`INSERT INTO harvest_proposals(
		session_id,status,kind,target,title,reason,issue,suggestion,diff,base_hash,base_mtime,evidence,plus,minus,created_at,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.SessionID, p.Status, p.Kind, p.Target, p.Title, p.Reason, p.Issue, p.Suggestion, p.Diff,
		p.BaseHash, p.BaseMtime, string(ev), p.Plus, p.Minus, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return p, err
	}
	p.ID, err = res.LastInsertId()
	if err != nil {
		return p, err
	}
	if strings.TrimSpace(p.Diff) != "" {
		_ = os.MkdirAll(s.harvestDir(), 0o755)
		_ = os.WriteFile(filepath.Join(s.harvestDir(), fmt.Sprintf("%d.diff", p.ID)), []byte(p.Diff), 0o644)
	}
	return p, nil
}

func (s *Store) GetProposal(id int64) (Proposal, error) {
	row := s.db.QueryRow(`SELECT id,session_id,status,kind,target,title,reason,issue,suggestion,diff,base_hash,base_mtime,evidence,plus,minus,created_at,updated_at
		FROM harvest_proposals WHERE id=?`, id)
	return scanProposal(row)
}

func (s *Store) BackdateRun(id int64, at time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("harvest store closed")
	}
	ts := at.UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE harvest_runs SET created_at=?, updated_at=? WHERE id=?`, ts, ts, id)
	return err
}

func (s *Store) LatestRun() (Run, bool) {
	row := s.db.QueryRow(`SELECT id,session_id,status,provider,skipped,created_at,updated_at
		FROM harvest_runs ORDER BY id DESC LIMIT 1`)
	r, err := scanRun(row)
	if err != nil {
		return Run{}, false
	}
	return r, true
}

func (s *Store) ListHistory(since time.Time) ([]HistoryItem, error) {
	sinceRFC := ""
	if !since.IsZero() {
		sinceRFC = since.UTC().Format(time.RFC3339)
	}
	propQuery := `SELECT id,session_id,status,kind,target,title,reason,created_at,updated_at
		FROM harvest_proposals WHERE status IN (?,?,?,?)`
	propArgs := []any{StatusApplied, StatusDeclined, StatusNoop, StatusStale}
	if sinceRFC != "" {
		propQuery += ` AND created_at >= ?`
		propArgs = append(propArgs, sinceRFC)
	}
	rows, err := s.db.Query(propQuery, propArgs...)
	if err != nil {
		return nil, err
	}
	var out []HistoryItem
	for rows.Next() {
		var it HistoryItem
		if err := rows.Scan(&it.ID, &it.SessionID, &it.Status, &it.Kind, &it.Target, &it.Title, &it.Reason, &it.CreatedAt, &it.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		it.Source = "proposal"
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	runQuery := `SELECT id,session_id,status,skipped,created_at,updated_at
		FROM harvest_runs WHERE status IN (?,?)`
	runArgs := []any{StatusSkipped, StatusFailed}
	if sinceRFC != "" {
		runQuery += ` AND created_at >= ?`
		runArgs = append(runArgs, sinceRFC)
	}
	runRows, err := s.db.Query(runQuery, runArgs...)
	if err != nil {
		return nil, err
	}
	defer runRows.Close()
	for runRows.Next() {
		var (
			it      HistoryItem
			skipped string
		)
		if err := runRows.Scan(&it.ID, &it.SessionID, &it.Status, &skipped, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.Source = "run"
		it.Kind = "run"
		it.Reason = skipped
		if it.Status == StatusSkipped {
			it.Title = "Nothing to change"
		} else {
			it.Title = "Harvest failed"
		}
		out = append(out, it)
	}
	if err := runRows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) DeleteExpired(cutoff time.Time) (int, error) {
	if cutoff.IsZero() {
		return 0, nil
	}
	cut := cutoff.UTC().Format(time.RFC3339)
	n := 0
	ids, err := s.proposalIDs(`SELECT id FROM harvest_proposals WHERE status IN (?,?,?,?) AND created_at < ?`,
		StatusApplied, StatusDeclined, StatusNoop, StatusStale, cut)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM harvest_proposals WHERE status IN (?,?,?,?) AND created_at < ?`,
		StatusApplied, StatusDeclined, StatusNoop, StatusStale, cut)
	if err != nil {
		return 0, err
	}
	if k, _ := res.RowsAffected(); k > 0 {
		n += int(k)
	}
	s.removeDiffs(ids)
	res, err = s.db.Exec(`DELETE FROM harvest_runs WHERE status != ? AND created_at < ?`, StatusPending, cut)
	if err != nil {
		return n, err
	}
	if k, _ := res.RowsAffected(); k > 0 {
		n += int(k)
	}
	return n, nil
}

func (s *Store) DeleteClosedForSessions(sessionIDs []string) (int, error) {
	ids := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	n := 0
	propQuery := `SELECT id FROM harvest_proposals WHERE status != ? AND session_id IN (` + placeholders + `)`
	propArgs := append([]any{StatusOpen}, args...)
	propIDs, err := s.proposalIDs(propQuery, propArgs...)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM harvest_proposals WHERE status != ? AND session_id IN (`+placeholders+`)`, propArgs...)
	if err != nil {
		return 0, err
	}
	if k, _ := res.RowsAffected(); k > 0 {
		n += int(k)
	}
	s.removeDiffs(propIDs)
	runArgs := append([]any{StatusPending}, args...)
	res, err = s.db.Exec(`DELETE FROM harvest_runs WHERE status != ? AND session_id IN (`+placeholders+`)`, runArgs...)
	if err != nil {
		return n, err
	}
	if k, _ := res.RowsAffected(); k > 0 {
		n += int(k)
	}
	return n, nil
}

func (s *Store) proposalIDs(query string, args ...any) ([]int64, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) removeDiffs(ids []int64) {
	dir := s.harvestDir()
	for _, id := range ids {
		_ = os.Remove(filepath.Join(dir, fmt.Sprintf("%d.diff", id)))
	}
}

func scanRun(row interface{ Scan(dest ...any) error }) (Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.SessionID, &r.Status, &r.Provider, &r.Skipped, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) List(status string) ([]Proposal, error) {
	status = strings.TrimSpace(status)
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.Query(`SELECT id,session_id,status,kind,target,title,reason,issue,suggestion,diff,base_hash,base_mtime,evidence,plus,minus,created_at,updated_at
			FROM harvest_proposals WHERE status=? ORDER BY id DESC`, StatusOpen)
	} else {
		rows, err = s.db.Query(`SELECT id,session_id,status,kind,target,title,reason,issue,suggestion,diff,base_hash,base_mtime,evidence,plus,minus,created_at,updated_at
			FROM harvest_proposals WHERE status=? ORDER BY id DESC`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Proposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) FindOpenDup(target, title string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM harvest_proposals WHERE status=? AND target=? AND lower(title)=lower(?)`,
		StatusOpen, target, strings.TrimSpace(title)).Scan(&n)
	return n > 0
}

func (s *Store) SetStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE harvest_proposals SET status=?, updated_at=? WHERE id=?`, status, nowRFC(), id)
	return err
}

func scanProposal(row interface{ Scan(dest ...any) error }) (Proposal, error) {
	var p Proposal
	var ev string
	err := row.Scan(&p.ID, &p.SessionID, &p.Status, &p.Kind, &p.Target, &p.Title, &p.Reason, &p.Issue, &p.Suggestion, &p.Diff, &p.BaseHash, &p.BaseMtime, &ev, &p.Plus, &p.Minus, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if strings.TrimSpace(ev) != "" {
		_ = json.Unmarshal([]byte(ev), &p.Evidence)
	}
	return p, nil
}
