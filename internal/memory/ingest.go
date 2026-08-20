package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

type IngestResult struct {
	SessionID string `json:"session_id"`
	Inserted  int    `json:"inserted"`
	Existing  int    `json:"existing"`
	Skipped   int    `json:"skipped"`
}

func IngestSession(root, sessionID string) (IngestResult, error) {
	layout := paths.Resolve(root)
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return IngestResult{}, fmt.Errorf("session id required")
	}
	spans, err := loadSpans(layout, id)
	if err != nil {
		return IngestResult{}, err
	}
	store, err := OpenRoot(root)
	if err != nil {
		return IngestResult{}, err
	}
	defer store.Close()
	return store.ingest(id, spans, session.NewStore(layout))
}

func IngestAll(root string) ([]IngestResult, error) {
	layout := paths.Resolve(root)
	entries, err := os.ReadDir(layout.SessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []IngestResult
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		res, err := IngestSession(root, e.Name())
		if err != nil {
			continue
		}
		out = append(out, res)
	}
	return out, nil
}

func (s *Store) ingest(sessionID string, spans []trace.Span, sessions *session.Store) (IngestResult, error) {
	res := IngestResult{SessionID: sessionID}
	var promptIDs []int64
	var prompts []string
	fileSet := map[string]struct{}{}
	seenText := map[string]struct{}{}
	var pendingTools []string
	for _, sp := range spans {
		if tool := toolNameFromSpan(sp); tool != "" && isToolSpan(sp) {
			pendingTools = append(pendingTools, tool)
			res.Skipped++
			if len(promptIDs) > 0 {
				_ = s.appendToolsTrailer(promptIDs[len(promptIDs)-1], []string{tool})
			}
			continue
		}
		ep, ok := projectSpan(sessionID, sp)
		if !ok {
			res.Skipped++
			continue
		}
		if Sanitize(ep.Text) == "" && Sanitize(ep.Title) == "" {
			res.Skipped++
			continue
		}
		key := strings.ToLower(strings.TrimSpace(ep.Text))
		if _, dup := seenText[key]; dup && ep.Kind == KindPrompt {
			res.Skipped++
			continue
		}
		if ep.Kind == KindPrompt && len(pendingTools) > 0 {
			ep.Text = strings.TrimSpace(ep.Text + toolsTrailer(pendingTools))
			pendingTools = nil
			if ep.Title == "" {
				ep.Title = firstLine(ep.Text, 80)
			}
		}
		if key != "" {
			seenText[key] = struct{}{}
		}
		id, inserted, err := s.storeEpisode(ep)
		if err != nil {
			return res, err
		}
		if id == 0 {
			res.Skipped++
			continue
		}
		if inserted {
			res.Inserted++
		} else {
			res.Existing++
		}
		if ep.Kind == KindPrompt {
			promptIDs = append(promptIDs, id)
			prompts = append(prompts, ep.Title)
		}
		for _, f := range ep.Files {
			fileSet[f] = struct{}{}
		}
	}
	if len(pendingTools) > 0 && len(promptIDs) > 0 {
		_ = s.appendToolsTrailer(promptIDs[len(promptIDs)-1], pendingTools)
	}
	if fp, err := sessions.GetFootprint(sessionID); err == nil {
		for _, f := range fp.Files {
			if f.Path != "" {
				fileSet[f.Path] = struct{}{}
			}
		}
	}
	files := setKeys(fileSet)
	meta, _ := sessions.Get(sessionID)
	working := workingEpisode(sessionID, meta, prompts, files)
	if _, _, err := s.storeEpisode(working); err != nil {
		return res, err
	}
	for i := 0; i < len(promptIDs); i++ {
		for j := i + 1; j < len(promptIDs) && j < i+8; j++ {
			_ = s.addEdge(promptIDs[i], promptIDs[j], EdgeSameSession)
		}
	}
	if len(promptIDs) > 0 {
		for i := 0; i+1 < len(promptIDs); i++ {
			_ = s.addEdge(promptIDs[i], promptIDs[i+1], EdgeNext)
		}
	}
	_ = s.ClusterTopics()
	return res, nil
}

func (s *Store) appendToolsTrailer(id int64, tools []string) error {
	trailer := toolsTrailer(tools)
	if trailer == "" {
		return nil
	}
	var uid, stored string
	if err := s.db.QueryRow(`SELECT uid, text FROM memory_episodes WHERE id=?`, id).Scan(&uid, &stored); err != nil {
		return err
	}
	plain := s.openText(uid, stored)
	if strings.Contains(plain, "[tools:") {
		return nil
	}
	sealed := s.sealText(uid, plain+trailer)
	_, err := s.db.Exec(`UPDATE memory_episodes SET text=?, updated_at=? WHERE id=?`, sealed, nowRFC(), id)
	return err
}

func (s *Store) storeEpisode(ep Episode) (int64, bool, error) {
	ep.Title = Sanitize(ep.Title)
	ep.Text = Sanitize(ep.Text)
	if ep.Title == "" && ep.Text == "" {
		return 0, false, nil
	}
	if ep.Kind == KindPrompt {
		if noisyCapture(ep.Text) || noisyCapture(ep.Title) || blockedCapture(ep.Text) || tooShort(ep.Text+ep.Title) || undeclaredNonEnglish(ep.Text) {
			return 0, false, nil
		}
		ep.Text = clipCapture(ep.Text)
	}
	if ep.Title == "" {
		ep.Title = firstLine(ep.Text, 80)
	}
	if ep.Tokens == 0 {
		ep.Tokens = EstimateTokens(ep.Title + " " + ep.Text)
	}
	if ep.Tags == "" {
		ep.Tags = entityTags(ep.Text)
	}
	if ep.Tier == "" {
		ep.Tier = tierForKind(ep.Kind)
	}
	vec := EmbedText(ep.Title + "\n" + ep.Text)
	if ep.Kind == KindTeaching {
		if dup := s.nearDuplicate(vec, ep.Kind); dup > 0 {
			_ = s.Reinforce(dup)
			return dup, false, nil
		}
	}
	id, inserted, err := s.upsertEpisode(ep, vec, true)
	if err != nil || id == 0 {
		return id, inserted, err
	}
	_ = s.writeShape(id, vec)
	return id, inserted, err
}

func projectSpan(sessionID string, sp trace.Span) (Episode, bool) {
	attrs := sp.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	name := strings.ToLower(sp.Name)
	created := time.Unix(0, sp.StartTimeUnixN).UTC().Format(time.RFC3339Nano)
	if sp.StartTimeUnixN == 0 {
		created = nowRFC()
	}
	files := filesFromAttrs(attrs)
	if strings.Contains(name, "llm.turn") || strings.Contains(name, "completion") {
		thought := strings.TrimSpace(attrs["coding_agent.llm.thought.text"])
		prompt := firstAttr(attrs, "gen_ai.prompt", "gen_ai.content.prompt")
		completion := firstAttr(attrs, "gen_ai.completion", "gen_ai.content.completion")
		if prompt == "" && thought != "" && completion == "" {
			return Episode{}, false
		}
		text := strings.TrimSpace(prompt)
		if completion != "" {
			if text != "" {
				text += "\n\n" + completion
			} else {
				text = completion
			}
		}
		if strings.Contains(strings.ToLower(text), "<private>") || skipPrivate(text) || noisyCapture(text) || blockedCapture(text) {
			return Episode{}, false
		}
		text = clipCapture(text)
		if tooShort(text) {
			return Episode{}, false
		}
		title := firstLine(prompt, 80)
		if title == "" {
			title = firstLine(text, 80)
		}
		if title == "" {
			return Episode{}, false
		}
		return Episode{
			UID:       episodeUID(sessionID, sp.SpanID, KindPrompt, text),
			SessionID: sessionID,
			SpanID:    sp.SpanID,
			Kind:      KindPrompt,
			Source:    SourceSpan,
			Title:     title,
			Text:      text,
			Files:     files,
			CreatedAt: created,
			Tier:      tierEpisodic,
		}, true
	}
	return Episode{}, false
}

func isToolSpan(sp trace.Span) bool {
	name := strings.ToLower(sp.Name)
	return strings.Contains(name, "tool") || strings.Contains(name, "shell") || strings.Contains(name, "edit")
}

func toolNameFromSpan(sp trace.Span) string {
	if sp.Attributes == nil {
		return ""
	}
	return cleanToolName(firstAttr(sp.Attributes, "gen_ai.tool.name", "coding_agent.tool.name"))
}

func workingEpisode(sessionID string, meta session.Meta, prompts, files []string) Episode {
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = strings.TrimSpace(meta.PromptPreview)
	}
	if title == "" && len(prompts) > 0 {
		title = prompts[len(prompts)-1]
	}
	if title == "" {
		title = "working " + sessionID
	}
	var b strings.Builder
	if meta.PromptPreview != "" {
		b.WriteString(meta.PromptPreview)
		b.WriteString("\n")
	}
	start := 0
	if len(prompts) > 3 {
		start = len(prompts) - 3
	}
	for _, p := range prompts[start:] {
		if p != "" && p != meta.PromptPreview {
			b.WriteString(p)
			b.WriteString("\n")
		}
	}
	text := Sanitize(b.String())
	return Episode{
		UID:       episodeUID(sessionID, "", KindWorking, sessionID),
		SessionID: sessionID,
		Kind:      KindWorking,
		Source:    SourceSpan,
		Title:     firstLine(title, 80),
		Text:      text,
		Files:     files,
		CreatedAt: meta.StartedAt.UTC().Format(time.RFC3339Nano),
	}
}

func loadSpans(layout paths.Paths, id string) ([]trace.Span, error) {
	path := filepath.Join(layout.SessionDir(id), "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var spans []trace.Span
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			break
		}
		var probe map[string]any
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		if _, ok := probe["trace_id"]; !ok {
			continue
		}
		var sp trace.Span
		if json.Unmarshal(raw, &sp) != nil || sp.Name == "" {
			continue
		}
		spans = append(spans, sp)
	}
	return spans, nil
}

func filesFromAttrs(attrs map[string]string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(firstAttr(attrs, "coding_agent.file_path", "code.file.path"))
	for _, key := range []string{"gen_ai.tool.arguments", "coding_agent.tool.arguments", "gen_ai.input.messages"} {
		raw := attrs[key]
		if raw == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			for _, k := range []string{"file_path", "filePath", "path", "notebook_path"} {
				if v, ok := m[k].(string); ok {
					add(v)
				}
			}
		}
	}
	return out
}

func firstAttr(attrs map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(attrs[k]); v != "" {
			return v
		}
	}
	return ""
}

func episodeUID(sessionID, spanID, kind, text string) string {
	sum := sha256.Sum256([]byte("v1|" + sessionID + "|" + spanID + "|" + kind + "|" + clip(text, 200)))
	return hex.EncodeToString(sum[:])
}

func firstLine(s string, limit int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	runes := []rune(s)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return s
}

func clip(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func nonempty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

const nearDupCosine = 0.95

func (s *Store) nearDuplicate(vec Vector, kind string) int64 {
	if isZero(vec) {
		return 0
	}
	rows, err := s.db.Query(`
SELECT e.id, v.vector FROM memory_vectors v
JOIN memory_episodes e ON e.id=v.episode_id
WHERE e.kind=? AND e.faded=0 AND v.embedder_id=?
LIMIT 400`, kind, CurrentEmbedder())
	if err != nil {
		return 0
	}
	defer rows.Close()
	bestID, best := int64(0), nearDupCosine
	for rows.Next() {
		var id int64
		var blob []byte
		if rows.Scan(&id, &blob) != nil {
			continue
		}
		c := Cosine(vec, blob)
		if c >= best {
			best = c
			bestID = id
		}
	}
	return bestID
}
