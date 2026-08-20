package memory

import (
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

func (s *Store) Capture(in CaptureInput) (Episode, error) {
	in.Kind = strings.TrimSpace(in.Kind)
	if in.Kind == "" {
		in.Kind = KindSession
	}
	in.Source = strings.TrimSpace(in.Source)
	if in.Source == "" {
		in.Source = SourceAgent
	}
	text := Sanitize(strings.TrimSpace(in.Text))
	title := Sanitize(strings.TrimSpace(in.Title))
	if title == "" {
		title = firstLine(text, 80)
	}
	if title == "" && text == "" {
		return Episode{}, fmt.Errorf("capture requires title or text")
	}
	if blockedCapture(text) || noisyCapture(text) || undeclaredNonEnglish(text) {
		return Episode{}, fmt.Errorf("capture skipped")
	}
	text = clipCapture(text)
	ep := Episode{
		UID:       episodeUID(in.SessionID, "", in.Kind+"|"+in.Source, title+"\n"+text+"|"+nowRFC()),
		SessionID: in.SessionID,
		Kind:      in.Kind,
		Source:    in.Source,
		Title:     title,
		Text:      text,
		Files:     in.Files,
		ToolName:  in.ToolName,
		Pinned:    in.Pin,
		Tokens:    EstimateTokens(title + " " + text),
	}
	id, _, err := s.storeEpisode(ep)
	if err != nil {
		return Episode{}, err
	}
	if in.ContradictOf > 0 {
		if err := s.contradict(id, in.ContradictOf); err != nil {
			return Episode{}, err
		}
	} else if in.Kind == KindSession && in.SessionID != "" {
		rows, err := s.db.Query(`SELECT id FROM memory_episodes WHERE session_id=? AND kind=? AND id!=? AND faded=0 ORDER BY id`, in.SessionID, KindSession, id)
		if err == nil {
			var oldIDs []int64
			for rows.Next() {
				var old int64
				if rows.Scan(&old) == nil {
					oldIDs = append(oldIDs, old)
				}
			}
			_ = rows.Close()
			for _, old := range oldIDs {
				_ = s.contradict(id, old)
			}
		}
	}
	got, err := s.Get(id)
	if err != nil {
		return Episode{}, err
	}
	if in.Kind == KindSession && in.SessionID != "" {
		_ = s.linkRollup(id, in.SessionID)
	}
	return got, nil
}

func (s *Store) linkRollup(sessionEpID int64, sessionID string) error {
	rows, err := s.db.Query(`SELECT id FROM memory_episodes WHERE session_id=? AND kind IN (?,?) AND id!=? AND faded=0`, sessionID, KindPrompt, KindWorking, sessionEpID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var src int64
		if rows.Scan(&src) == nil {
			ids = append(ids, src)
		}
	}
	_ = rows.Close()
	for _, src := range ids {
		_ = s.addEdge(sessionEpID, src, EdgeRolledUpFrom)
	}
	return nil
}

func (s *Store) Contradict(oldID int64, in CaptureInput) (Episode, error) {
	in.ContradictOf = oldID
	if in.Kind == "" {
		old, err := s.Get(oldID)
		if err != nil {
			return Episode{}, err
		}
		in.Kind = old.Kind
		if in.SessionID == "" {
			in.SessionID = old.SessionID
		}
	}
	return s.Capture(in)
}

func (s *Store) contradict(newID, oldID int64) error {
	if err := s.addEdge(newID, oldID, EdgeContradicts); err != nil {
		return err
	}
	now := nowRFC()
	_, err := s.db.Exec(`UPDATE memory_episodes SET valid_to=? WHERE id=? AND (valid_to='' OR valid_to IS NULL)`, now, oldID)
	return err
}

func CaptureRoot(root string, in CaptureInput) (Episode, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return Episode{}, err
	}
	defer store.Close()
	ep, err := store.Capture(in)
	if err != nil {
		return ep, err
	}
	if in.Kind == KindSession && in.SessionID != "" {
		_ = writeSessionSummary(root, in.SessionID, in.Text)
		_ = store.ClearPending(in.SessionID)
	}
	return ep, nil
}

func writeSessionSummary(root, sessionID, summary string) error {
	summary = Sanitize(firstLine(summary, 400))
	if summary == "" {
		return nil
	}
	store := session.NewStore(paths.Resolve(root))
	meta, err := store.Get(sessionID)
	if err != nil {
		return err
	}
	meta.Summary = summary
	return store.UpdateMeta(meta)
}
