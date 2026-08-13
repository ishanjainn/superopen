package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ishanjainn/superopen/internal/coding/pricing"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/redact"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

const (
	DefaultTurnTokens = 500
	DefaultFileTokens = 250
	DefaultTurnHits   = 4
)

type RetrievalQuery struct {
	Text       string
	Vendor     string
	Paths      []string
	Branch     string
	Worktree   string
	Seen       map[string]string // fingerprint -> content identity
	MaxTokens  int
	MaxResults int
	FileOnly   bool
}

type RetrievalHit struct {
	Fingerprint         string        `json:"fingerprint"`
	ContentID           string        `json:"content_id"`
	Vendor              string        `json:"vendor"`
	Scope               string        `json:"scope"`
	Status              string        `json:"status,omitempty"`
	Kind                string        `json:"kind"`
	Summary             string        `json:"summary"`
	Applicability       string        `json:"applicability,omitempty"`
	TargetPath          string        `json:"target_path,omitempty"`
	Score               float64       `json:"score"`
	Reasons             []string      `json:"reasons"`
	EstimatedTokens     int64         `json:"estimated_tokens"`
	Occurrences         int           `json:"occurrences"`
	Verified            int           `json:"verified_sessions"`
	Confidence          float64       `json:"confidence"`
	Evidence            []EvidenceRef `json:"evidence_refs,omitempty"`
	Stale               bool          `json:"stale,omitempty"`
	fileEvidenceQuality float64
}

type EvidenceWindow struct {
	Pattern RetrievalHit    `json:"pattern"`
	Events  []EvidenceEvent `json:"events"`
}

// EvidenceEvent is intentionally compact. Expansion never returns captured
// prompts, responses, reasoning, tool arguments, or complete tool results.
type EvidenceEvent struct {
	SpanID    string            `json:"span_id"`
	Name      string            `json:"name"`
	SessionID string            `json:"session_id"`
	Timestamp int64             `json:"start_time_unix_nano"`
	Status    string            `json:"status,omitempty"`
	Facts     map[string]string `json:"facts,omitempty"`
}

// StartupPatterns returns a small vendor-isolated pack of the strongest
// durable patterns without requiring prompt text. Unverified first sightings
// remain searchable but are never included here.
func (s *Store) StartupPatterns(vendor string, maxTokens int) ([]RetrievalHit, error) {
	st, err := s.readState()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	vendor = harness.NormalizeVendorKind(vendor)
	var hits []RetrievalHit
	for _, p := range st.Patterns {
		if p.Status == "dismissed" || p.Status == "superseded" || p.Status == "obsolete" || s.patternStale(p) {
			continue
		}
		if p.Scope != "shared" && harness.NormalizeVendorKind(p.Vendor) != vendor {
			continue
		}
		if !p.ExplicitWorkflow && len(p.VerifiedSessions) == 0 && (p.Occurrences < 2 || p.Confidence < .70) {
			continue
		}
		h := RetrievalHit{Fingerprint: p.Fingerprint, ContentID: patternContentID(p), Vendor: p.Vendor, Scope: p.Scope, Status: p.Status,
			Kind: p.Kind, Summary: p.Summary, Applicability: p.Applicability, TargetPath: p.TargetPath,
			Occurrences: p.Occurrences, Verified: len(p.VerifiedSessions), Confidence: p.Confidence, Evidence: p.EvidenceRefs}
		h.Score = .45*clamp(p.Confidence) + .30*boolScore(h.Verified > 0 || p.ExplicitWorkflow) + .25*clamp(float64(p.Occurrences)/3)
		h.Reasons = []string{"startup", "durable"}
		h.EstimatedTokens = pricing.EstimateTokens(formatRetrievalHit(h))
		hits = append(hits, h)
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if maxTokens <= 0 {
		maxTokens = 1500
	}
	var out []RetrievalHit
	var used int64
	for _, h := range hits {
		if used+h.EstimatedTokens > int64(maxTokens) {
			continue
		}
		out = append(out, h)
		used += h.EstimatedTokens
	}
	return out, nil
}

func boolScore(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// Retrieve performs bounded local ranking. It never invokes a model, graph
// builder, network client, or external executable.
func (s *Store) Retrieve(q RetrievalQuery) ([]RetrievalHit, error) {
	deadline := time.Now().Add(40 * time.Millisecond)
	st, err := s.readState()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	q.Vendor = harness.NormalizeVendorKind(q.Vendor)
	if q.MaxTokens <= 0 {
		q.MaxTokens = DefaultTurnTokens
	}
	if q.MaxResults <= 0 || q.MaxResults > DefaultTurnHits {
		q.MaxResults = DefaultTurnHits
	}
	queryTokens := tokenSet(q.Text + " " + q.Branch + " " + q.Worktree + " " + strings.Join(q.Paths, " "))
	normalPaths := normalizePaths(q.Paths)
	now := time.Now().UTC()
	var ranked []RetrievalHit
	for _, p := range st.Patterns {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("memory retrieval deadline exceeded")
		}
		if p.Status == "dismissed" || p.Status == "superseded" || p.Status == "obsolete" {
			continue
		}
		if p.Scope == "" {
			p.Scope = "vendor"
		}
		if p.Scope != "shared" && harness.NormalizeVendorKind(p.Vendor) != q.Vendor {
			continue
		}
		eligible := p.ExplicitWorkflow || len(p.VerifiedSessions) > 0 || (p.Occurrences >= 2 && p.Confidence >= 0.70)
		if !eligible {
			continue
		}
		contentID := patternContentID(p)
		if q.Seen != nil && q.Seen[p.Fingerprint] == contentID {
			continue
		}
		stale := s.patternStale(p)
		if stale {
			continue
		}
		patternText := strings.Join(append(append([]string{p.Summary, p.Applicability, p.TargetPath}, p.Keywords...), append(p.Symbols, p.ErrorSignatures...)...), " ")
		textOverlap := overlap(queryTokens, tokenSet(patternText))
		pathOverlap := pathScore(normalPaths, append(append([]string{p.TargetPath}, p.Paths...), p.Symbols...))
		if q.FileOnly && pathOverlap == 0 {
			continue
		}
		verified := 0.0
		if len(p.VerifiedSessions) > 0 {
			verified = 1
		}
		recurrence := float64(p.Occurrences) / 3
		if recurrence > 1 {
			recurrence = 1
		}
		recency := 0.0
		if !p.LastObservedAt.IsZero() {
			days := now.Sub(p.LastObservedAt).Hours() / 24
			if days < 0 {
				days = 0
			}
			recency = 1 - days/180
			if recency < 0 {
				recency = 0
			}
		}
		score := .35*textOverlap + .25*pathOverlap + .15*clamp(p.Confidence) + .10*verified + .10*recurrence + .05*recency
		if p.Contradictions > 0 {
			score -= .35
		}
		if score < .55 {
			continue
		}
		reasons := []string{}
		if textOverlap > 0 {
			reasons = append(reasons, "prompt")
		}
		if pathOverlap > 0 {
			reasons = append(reasons, "path")
		}
		if verified > 0 {
			reasons = append(reasons, "verified")
		}
		if p.Occurrences > 1 {
			reasons = append(reasons, "recurring")
		}
		h := RetrievalHit{Fingerprint: p.Fingerprint, ContentID: contentID, Vendor: p.Vendor, Scope: p.Scope, Status: p.Status,
			Kind: p.Kind, Summary: p.Summary, Applicability: p.Applicability, TargetPath: p.TargetPath,
			Score: score, Reasons: reasons, Occurrences: p.Occurrences, Verified: len(p.VerifiedSessions),
			Confidence: p.Confidence, Evidence: p.EvidenceRefs}
		h.fileEvidenceQuality = evidenceQuality(p.EvidenceRefs)
		if q.FileOnly && h.fileEvidenceQuality > 0 {
			h.Reasons = append(h.Reasons, "modified-evidence")
		}
		h.EstimatedTokens = pricing.EstimateTokens(formatRetrievalHit(h))
		ranked = append(ranked, h)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if q.FileOnly && ranked[i].fileEvidenceQuality != ranked[j].fileEvidenceQuality {
			return ranked[i].fileEvidenceQuality > ranked[j].fileEvidenceQuality
		}
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].Fingerprint < ranked[j].Fingerprint
		}
		return ranked[i].Score > ranked[j].Score
	})
	var out []RetrievalHit
	used := int64(0)
	for _, h := range ranked {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("memory retrieval deadline exceeded")
		}
		if len(out) >= q.MaxResults || used+h.EstimatedTokens > int64(q.MaxTokens) {
			continue
		}
		out = append(out, h)
		used += h.EstimatedTokens
	}
	return out, nil
}

func evidenceQuality(refs []EvidenceRef) float64 {
	best := 0.0
	for _, ref := range refs {
		quality := 0.0
		if ref.Modified {
			quality += 1
		}
		if ref.SessionFileCount > 0 {
			quality += 1 / float64(ref.SessionFileCount)
		}
		if quality > best {
			best = quality
		}
	}
	return best
}

func FormatRetrieval(hits []RetrievalHit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Relevant project memory\n\nAdvisory historical evidence; current instructions, guardrails, and live guidance take precedence.\n")
	for _, h := range hits {
		b.WriteString(formatRetrievalHit(h))
	}
	return strings.TrimSpace(b.String())
}

func formatRetrievalHit(h RetrievalHit) string {
	if h.Status == "applied" && h.TargetPath != "" {
		return fmt.Sprintf("\n- [%s] Follow current live guidance at `%s` (content fingerprint verified).\n", h.Fingerprint, filepath.ToSlash(h.TargetPath))
	}
	line := fmt.Sprintf("\n- [%s] %s", h.Fingerprint, strings.TrimSpace(h.Summary))
	if h.Applicability != "" {
		line += " Applies when: " + h.Applicability
	}
	if h.TargetPath != "" {
		line += " Target: `" + filepath.ToSlash(h.TargetPath) + "`."
	}
	line += fmt.Sprintf(" Evidence: %d session(s), %d verified; confidence %.0f%%.", h.Occurrences, h.Verified, h.Confidence*100)
	return line + "\n"
}

func (s *Store) patternStale(p Pattern) bool {
	wantSHA := p.SourceSHA256
	if p.Status == "applied" && p.GuidanceSHA256 != "" {
		wantSHA = p.GuidanceSHA256
	}
	if wantSHA == "" {
		return false
	}
	path := p.TargetPath
	if path == "" && len(p.Paths) > 0 {
		path = p.Paths[0]
	}
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.Paths.RepoRoot, filepath.FromSlash(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) != wantSHA
}

func patternContentID(p Pattern) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{p.Fingerprint, p.Summary, p.Applicability, p.TargetPath, p.SourceSHA256, p.GuidanceSHA256}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func tokenSet(s string) map[string]bool {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte(' ')
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteByte(' ')
		}
	}
	out := map[string]bool{}
	for _, tok := range strings.Fields(b.String()) {
		if len(tok) > 1 {
			out[tok] = true
		}
	}
	return out
}

func overlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	den := len(a)
	if len(b) < den {
		den = len(b)
	}
	if den == 0 {
		return 0
	}
	v := float64(n) / float64(den)
	if v > 1 {
		return 1
	}
	return v
}

func normalizePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = normalizePathKey(p)
		if p != "." && p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizePathKey(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	unc := strings.HasPrefix(value, "//")
	value = path.Clean(value)
	if unc {
		value = "//" + strings.TrimLeft(value, "/")
	}
	// Windows paths are case-insensitive for retrieval identity. Lowercasing
	// all lookup keys is deterministic on every platform and never changes the
	// repository-relative path persisted in memory.
	return strings.ToLower(value)
}

func pathScore(query, candidates []string) float64 {
	best := 0.0
	for _, q := range query {
		for _, c := range candidates {
			c = normalizePathKey(c)
			if q == c || strings.HasSuffix(q, "/"+c) || strings.HasSuffix(c, "/"+q) {
				return 1
			}
			if filepath.Base(q) == filepath.Base(c) && filepath.Base(q) != "." {
				best = .75
			}
			if strings.Contains(q, c) || strings.Contains(c, q) {
				if best < .5 {
					best = .5
				}
			}
		}
	}
	return best
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (s *Store) PatternEvidence(fingerprint, vendor string) (EvidenceWindow, error) {
	patterns, err := s.ListPatterns()
	if err != nil {
		return EvidenceWindow{}, err
	}
	var p *Pattern
	for i := range patterns {
		if patterns[i].Fingerprint == fingerprint && (vendor == "" || harness.NormalizeVendorKind(patterns[i].Vendor) == harness.NormalizeVendorKind(vendor)) {
			p = &patterns[i]
			break
		}
	}
	if p == nil {
		return EvidenceWindow{}, fmt.Errorf("pattern not found: %s", fingerprint)
	}
	h := RetrievalHit{Fingerprint: p.Fingerprint, ContentID: patternContentID(*p), Vendor: p.Vendor, Scope: p.Scope, Status: p.Status, Kind: p.Kind, Summary: p.Summary, Applicability: p.Applicability, TargetPath: p.TargetPath, Occurrences: p.Occurrences, Verified: len(p.VerifiedSessions), Confidence: p.Confidence, Evidence: p.EvidenceRefs, Stale: s.patternStale(*p)}
	if len(p.EvidenceRefs) == 0 {
		return EvidenceWindow{Pattern: h}, nil
	}
	ref := p.EvidenceRefs[0]
	spans, err := tracestore.NewLocalJSONL(s.Paths.SessionsDir).Query(tracestore.QueryFilter{SessionID: ref.SessionID})
	if err != nil {
		return EvidenceWindow{}, err
	}
	wanted := map[string]bool{}
	for _, id := range ref.EventIDs {
		wanted[id] = true
	}
	anchor := -1
	for i, sp := range spans {
		if wanted[sp.SpanID] {
			anchor = i
			break
		}
	}
	if anchor >= 0 {
		start := anchor - 2
		if start < 0 {
			start = 0
		}
		end := anchor + 3
		if end > len(spans) {
			end = len(spans)
		}
		spans = spans[start:end]
	} else {
		spans = nil
	}
	events := make([]EvidenceEvent, 0, len(spans))
	for _, sp := range spans {
		facts := map[string]string{}
		for _, key := range []string{"gen_ai.tool.name", "code.file.path", "coding_agent.file_path", "coding_agent.tool.errored", "error.type", "vcs.branch"} {
			if v := strings.TrimSpace(sp.Attributes[key]); v != "" {
				facts[key] = compactEvidenceText(redact.StringFull(v), 240)
			}
		}
		events = append(events, EvidenceEvent{SpanID: sp.SpanID, Name: sp.Name, SessionID: sp.SessionID,
			Timestamp: sp.StartTimeUnixN, Status: sp.Status, Facts: facts})
	}
	return EvidenceWindow{Pattern: h, Events: events}, nil
}

func compactEvidenceText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// ConsolidateRetrievals applies per-session retrieval outcomes once, during
// finalization. Prompt hooks remain read-only with respect to durable memory.
func (s *Store) ConsolidateRetrievals(sessionID, vendor string, patternIDs []string, editedPaths []string, verified bool) error {
	vendor = harness.NormalizeVendorKind(vendor)
	paths := normalizePaths(editedPaths)
	seen := map[string]bool{}
	return s.mutateState(func(st *stateFile) error {
		now := time.Now().UTC()
		for i := range st.Patterns {
			p := &st.Patterns[i]
			if seen[p.Fingerprint] || !containsString(patternIDs, p.Fingerprint) || (p.Scope != "shared" && harness.NormalizeVendorKind(p.Vendor) != vendor) {
				continue
			}
			seen[p.Fingerprint] = true
			if containsString(p.RetrievalSessions, sessionID) {
				continue
			}
			p.RetrievalSessions = append(p.RetrievalSessions, sessionID)
			p.RetrievalCount++
			p.LastRetrievedAt = &now
			matchedWork := pathScore(paths, append([]string{p.TargetPath}, p.Paths...)) > 0
			if verified && matchedWork {
				p.Confidence = p.Confidence + (1-p.Confidence)*.05
				p.LastVerifiedAt = &now
				p.Verification = append(p.Verification, PatternVerification{SessionID: sessionID, Outcome: "verified_reuse", At: now})
			}
		}
		return nil
	})
}

func (s *Store) RecordContradiction(sessionID, vendor, targetPath string) error {
	vendor = harness.NormalizeVendorKind(vendor)
	targetPath = strings.ToLower(filepath.ToSlash(filepath.Clean(targetPath)))
	if targetPath == "" || targetPath == "." {
		return nil
	}
	return s.mutateState(func(st *stateFile) error {
		now := time.Now().UTC()
		for i := range st.Patterns {
			p := &st.Patterns[i]
			if harness.NormalizeVendorKind(p.Vendor) != vendor || strings.ToLower(filepath.ToSlash(filepath.Clean(p.TargetPath))) != targetPath {
				continue
			}
			for _, v := range p.Verification {
				if v.SessionID == sessionID && v.Outcome == "contradicted" {
					return nil
				}
			}
			p.Contradictions++
			p.Confidence *= .80
			p.Verification = append(p.Verification, PatternVerification{SessionID: sessionID, Outcome: "contradicted", At: now})
		}
		return nil
	})
}

func (s *Store) RecordPatternRevert(fingerprint, vendor, reason string) error {
	vendor = harness.NormalizeVendorKind(vendor)
	return s.mutateState(func(st *stateFile) error {
		for i := range st.Patterns {
			p := &st.Patterns[i]
			if p.Fingerprint != fingerprint || harness.NormalizeVendorKind(p.Vendor) != vendor {
				continue
			}
			p.Contradictions++
			p.Confidence *= .80
			p.Status = "pending"
			p.StatusReason = nonEmptyReason(redact.StringFull(reason), "applied guidance was reverted")
			return nil
		}
		return nil
	})
}

func (s *Store) FeedbackPattern(fingerprint, vendor, feedback, reason string) (Pattern, error) {
	vendor = harness.NormalizeVendorKind(vendor)
	feedback = strings.ToLower(strings.TrimSpace(feedback))
	reason = redact.StringFull(strings.TrimSpace(reason))
	var result Pattern
	err := s.mutateState(func(st *stateFile) error {
		for i := range st.Patterns {
			p := &st.Patterns[i]
			if p.Fingerprint != fingerprint || harness.NormalizeVendorKind(p.Vendor) != vendor {
				continue
			}
			switch feedback {
			case "helpful":
				p.HelpfulCount++
				p.Confidence = p.Confidence + (1-p.Confidence)*.10
				now := time.Now().UTC()
				p.LastVerifiedAt = &now
			case "incorrect":
				p.IncorrectCount++
				p.Confidence = 0
				p.Status = "dismissed"
				p.StatusReason = nonEmptyReason(reason, "marked incorrect")
			case "obsolete":
				p.Status = "obsolete"
				p.StatusReason = nonEmptyReason(reason, "marked obsolete")
			default:
				return fmt.Errorf("feedback must be helpful, incorrect, or obsolete")
			}
			result = *p
			return nil
		}
		return fmt.Errorf("pattern not found: %s", fingerprint)
	})
	return result, err
}

func nonEmptyReason(reason, fallback string) string {
	if strings.TrimSpace(reason) != "" {
		return reason
	}
	return fallback
}
