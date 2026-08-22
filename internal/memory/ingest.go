package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/repofile"
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

const ingestBackfillLimit = 8

// IngestBackfill ingests at most `limit` recent other sessions that have
// spans but no KindPrompt. Fail-open. Bound for SessionStart (5s hook).
func IngestBackfill(root, currentSession string, limit int) []IngestResult {
	if limit <= 0 {
		limit = ingestBackfillLimit
	}
	layout := paths.Resolve(root)
	entries, err := os.ReadDir(layout.SessionsDir)
	if err != nil {
		return nil
	}
	type row struct {
		id    string
		mtime int64
	}
	var sessions []row
	currentSession = strings.TrimSpace(currentSession)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if id == currentSession {
			continue
		}
		info, err := os.Stat(filepath.Join(layout.SessionDir(id), "events.jsonl"))
		if err != nil {
			continue
		}
		sessions = append(sessions, row{id: id, mtime: info.ModTime().UnixNano()})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].mtime > sessions[j].mtime })
	store, err := OpenRoot(root)
	if err != nil {
		return nil
	}
	var missing []string
	for _, sess := range sessions {
		if store.HasKindPrompt(sess.id) {
			continue
		}
		missing = append(missing, sess.id)
		if len(missing) >= limit {
			break
		}
	}
	store.Close()
	var out []IngestResult
	for _, id := range missing {
		res, err := IngestSession(root, id)
		if err != nil {
			continue
		}
		out = append(out, res)
	}
	return out
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
			if obs, ok := toolObservation(sessionID, sp); ok {
				id, inserted, err := s.storeEpisode(obs)
				if err != nil {
					return res, err
				}
				if id == 0 {
					res.Skipped++
				} else if inserted {
					res.Inserted++
				} else {
					res.Existing++
				}
				for _, f := range obs.Files {
					fileSet[f] = struct{}{}
				}
			} else {
				res.Skipped++
			}
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
		ep.Text = unwrapCapture(ep.Text)
		ep.Title = unwrapCapture(ep.Title)
		if packFingerprint(ep.Text) || packFingerprint(ep.Title) || noisyCapture(ep.Text) || noisyCapture(ep.Title) || blockedCapture(ep.Text) || tooShort(ep.Text+ep.Title) || undeclaredNonEnglish(ep.Text) {
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
	promptEvent := strings.Contains(name, "llm.turn") || strings.Contains(name, "completion") || strings.Contains(name, "user_prompt.submit")
	if !promptEvent {
		return Episode{}, false
	}
	thought := strings.TrimSpace(attrs["coding_agent.llm.thought.text"])
	prompt := firstAttr(attrs, "gen_ai.prompt", "gen_ai.content.prompt")
	if prompt == "" {
		prompt = messagesRoleText(attrs["gen_ai.input.messages"], true)
	}
	prompt = unwrapCapture(prompt)
	if prompt == "" && thought != "" {
		return Episode{}, false
	}
	text := strings.TrimSpace(prompt)
	if text == "" {
		return Episode{}, false
	}
	if packFingerprint(text) || strings.Contains(strings.ToLower(text), "<private>") || skipPrivate(text) || noisyCapture(text) || blockedCapture(text) {
		return Episode{}, false
	}
	text = clipCapture(text)
	if tooShort(text) {
		return Episode{}, false
	}
	title := firstLine(text, 80)
	if title == "" {
		return Episode{}, false
	}
	return Episode{
		UID:         contentHashUID(sessionID, KindPrompt, title, text),
		ContentHash: contentHashUID(sessionID, KindPrompt, title, text),
		SessionID:   sessionID,
		SpanID:      sp.SpanID,
		Kind:        KindPrompt,
		Source:      SourceSpan,
		Title:       title,
		Text:        text,
		Files:       files,
		CreatedAt:   created,
		Tier:        tierEpisodic,
		Topic:       heuristicTopic(title + " " + text),
		Tags:        entityTags(text),
	}, true
}

func isToolSpan(sp trace.Span) bool {
	name := strings.ToLower(sp.Name)
	if strings.Contains(name, "llm.turn") || strings.Contains(name, "completion") {
		return false
	}
	return strings.Contains(name, "tool") || strings.Contains(name, "shell") || strings.Contains(name, "edit") || strings.Contains(name, "read")
}

func toolObservation(sessionID string, sp trace.Span) (Episode, bool) {
	files := filesFromAttrs(sp.Attributes)
	if len(files) == 0 {
		return Episode{}, false
	}
	tool := toolNameFromSpan(sp)
	if tool == "" {
		tool = strings.TrimSpace(sp.Name)
	}
	path := files[0]
	state := repofile.State(tool, sp.Name)
	title := strings.TrimSpace(tool + " " + path)
	if title == "" {
		return Episode{}, false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "tool: %s\n", tool)
	if path != "" {
		fmt.Fprintf(&b, "path: %s\n", path)
	}
	fmt.Fprintf(&b, "state: %s\n", state)
	text := strings.TrimSpace(b.String())
	created := time.Unix(0, sp.StartTimeUnixN).UTC().Format(time.RFC3339Nano)
	if sp.StartTimeUnixN == 0 {
		created = nowRFC()
	}
	return Episode{
		UID:         contentHashUID(sessionID, KindObservation, tool, path, state),
		ContentHash: contentHashUID(sessionID, KindObservation, tool, path, state),
		SessionID:   sessionID,
		SpanID:      sp.SpanID,
		Kind:        KindObservation,
		Source:      SourceSpan,
		Title:       firstLine(title, 80),
		Text:        text,
		Files:       files,
		CreatedAt:   created,
		Tier:        tierEpisodic,
		Topic:       ObservationChange,
	}, true
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
		p = NormalizePath(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if attrs == nil {
		return nil
	}
	tool := firstAttr(attrs, "gen_ai.tool.name")
	cwd := firstAttr(attrs, "code.cwd")
	add(repofile.Accept(firstAttr(attrs, "coding_agent.file_path", "code.file.path"), tool, cwd))
	for _, key := range []string{"gen_ai.tool.call.arguments", "gen_ai.tool.arguments", "coding_agent.tool.arguments"} {
		raw := attrs[key]
		if raw == "" {
			continue
		}
		add(repofile.Accept(repofile.PathFromJSON(raw), tool, cwd))
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

func messagesRoleText(raw string, userOnly bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return ""
	}
	items := messageObjects(payload)
	var parts []string
	for _, item := range items {
		role := strings.ToLower(strings.TrimSpace(asString(item["role"])))
		if userOnly {
			if role != "" && role != "user" {
				continue
			}
		} else if role == "user" || role == "system" {
			continue
		}
		text := strings.TrimSpace(messageContent(item))
		if text == "" || isThoughtOnly(text, item) {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func messageObjects(payload any) []map[string]any {
	switch v := payload.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		if nested, ok := v["messages"]; ok {
			return messageObjects(nested)
		}
		return []map[string]any{v}
	default:
		return nil
	}
}

func messageContent(item map[string]any) string {
	if s := asString(item["content"]); s != "" {
		return s
	}
	if s := asString(item["text"]); s != "" {
		return s
	}
	return joinParts(item["parts"])
}

func joinParts(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			switch p := item.(type) {
			case string:
				if strings.TrimSpace(p) != "" {
					parts = append(parts, p)
				}
			case map[string]any:
				kind := strings.ToLower(asString(p["type"]) + " " + asString(p["kind"]))
				if strings.Contains(kind, "thought") || strings.Contains(kind, "reasoning") {
					continue
				}
				if s := firstNonEmpty(asString(p["content"]), asString(p["text"]), asString(p["value"])); s != "" {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func isThoughtOnly(text string, item map[string]any) bool {
	if strings.TrimSpace(text) == "" {
		return true
	}
	kind := strings.ToLower(asString(item["type"]) + " " + asString(item["kind"]))
	return strings.Contains(kind, "thought") || strings.Contains(kind, "reasoning")
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func episodeUID(sessionID, spanID, kind, text string) string {
	return contentHashUID(sessionID, spanID, kind, text)
}

func contentHashUID(parts ...string) string {
	sum := sha256.Sum256([]byte("idem:sha256|" + strings.Join(parts, "|")))
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
