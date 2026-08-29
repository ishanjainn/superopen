package harvest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
