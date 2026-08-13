package retrieve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
)

type Hit struct {
	Path    string  `json:"path"`
	Kind    string  `json:"kind"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type IndexDoc struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

func indexPath(paths harness.Paths) string {
	return paths.GraphCorpus
}

// Rebuild walks harness corpus into a simple keyword index.
// Indexes every AGENTS.md plus all vendor rules/skills trees (not only the
// preferred write target), so UI search, evals, and recommendations see the
// full guidance surface.
func Rebuild(repoRoot string, paths harness.Paths) (int, error) {
	var docs []IndexDoc
	seen := map[string]bool{}
	addFile := func(path, kind string) {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			return
		}
		rel, _ := filepath.Rel(repoRoot, path)
		if rel == "" || strings.HasPrefix(rel, "..") {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if seen[rel] {
			return
		}
		seen[rel] = true
		text := string(data)
		if len(text) > 100_000 {
			text = text[:100_000]
		}
		docs = append(docs, IndexDoc{Path: rel, Kind: kind, Content: text})
	}
	addSynthetic := func(path, kind, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		docs = append(docs, IndexDoc{Path: filepath.ToSlash(path), Kind: kind, Content: content})
	}
	walkRules := func(dir string) {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == "superopen.mdc" || name == "superopen.md" {
				return nil
			}
			if !isIndexableRule(name) {
				return nil
			}
			addFile(path, "rules")
			return nil
		})
	}
	walkSkills := func(dir string) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "so" || name == "superopen" {
				continue
			}
			addFile(filepath.Join(dir, name, "SKILL.md"), "skills")
		}
	}

	for _, dir := range harness.RulesCandidates(repoRoot) {
		walkRules(dir)
	}
	// Also index the preferred write target even if empty-of-user at rebuild time
	// (seeded coding.md may land there between scans).
	if paths.RulesDir != "" {
		walkRules(paths.RulesDir)
	}

	for _, dir := range harness.SkillsCandidates(repoRoot) {
		walkSkills(dir)
	}
	if paths.SkillsDir != "" {
		walkSkills(paths.SkillsDir)
	}

	for _, agents := range paths.AgentsPaths() {
		addFile(agents, "knowledge")
	}
	addFile(paths.GuardrailsFile, "guardrails")
	if data, err := os.ReadFile(filepath.Join(paths.MemoryDir, "state.json")); err == nil {
		var state struct {
			Preferences string                      `json:"preferences"`
			Projects    string                      `json:"projects"`
			Lessons     []struct{ ID, Text string } `json:"lessons"`
			Patterns    []struct {
				Fingerprint     string   `json:"fingerprint"`
				Summary         string   `json:"summary"`
				Applicability   string   `json:"applicability"`
				TargetPath      string   `json:"target_path"`
				Status          string   `json:"status"`
				Keywords        []string `json:"keywords"`
				Paths           []string `json:"paths"`
				Symbols         []string `json:"symbols"`
				ErrorSignatures []string `json:"error_signatures"`
			} `json:"patterns"`
		}
		if json.Unmarshal(data, &state) == nil {
			addSynthetic(".so/memory/preferences", "memory", state.Preferences)
			addSynthetic(".so/memory/projects", "memory", state.Projects)
			for _, lesson := range state.Lessons {
				addSynthetic(".so/memory/lesson/"+lesson.ID, "memory", lesson.Text)
			}
			for _, p := range state.Patterns {
				if p.Status == "dismissed" || p.Status == "superseded" {
					continue
				}
				content := strings.Join([]string{p.Summary, p.Applicability, p.TargetPath, strings.Join(p.Keywords, " "), strings.Join(p.Paths, " "), strings.Join(p.Symbols, " "), strings.Join(p.ErrorSignatures, " ")}, "\n")
				addSynthetic(".so/memory/pattern/"+p.Fingerprint, "memory", content)
			}
		}
	}
	if data, err := os.ReadFile(paths.GraphJSON); err == nil {
		docs = append(docs, IndexDoc{Path: ".so/graph/graph.json", Kind: "graph", Content: string(data)})
	}
	// Recent sessions contribute compact summaries and finding conclusions only.
	// Full prompts, tool results, transcripts, and recommendation bodies never
	// enter the corpus.
	_ = filepath.WalkDir(paths.SessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "session.json" {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			var doc struct {
				Summary string `json:"summary"`
				Review  struct {
					Findings []struct {
						Kind            string   `json:"kind"`
						Summary         string   `json:"summary"`
						TargetPath      string   `json:"target_path"`
						Applicability   string   `json:"applicability"`
						Keywords        []string `json:"keywords"`
						Paths           []string `json:"paths"`
						Symbols         []string `json:"symbols"`
						ErrorSignatures []string `json:"error_signatures"`
					} `json:"findings"`
				} `json:"review"`
			}
			if json.Unmarshal(data, &doc) != nil {
				return nil
			}
			var lines []string
			if doc.Summary != "" {
				lines = append(lines, doc.Summary)
			}
			for _, f := range doc.Review.Findings {
				lines = append(lines, strings.Join([]string{f.Kind, f.Summary, f.TargetPath, f.Applicability, strings.Join(f.Keywords, " "), strings.Join(f.Paths, " "), strings.Join(f.Symbols, " "), strings.Join(f.ErrorSignatures, " ")}, " "))
			}
			rel, _ := filepath.Rel(repoRoot, path)
			addSynthetic(rel, "session", strings.Join(lines, "\n"))
		}
		return nil
	})
	if err := os.MkdirAll(paths.GraphDir, 0o755); err != nil {
		return 0, err
	}
	raw, err := json.MarshalIndent(map[string]any{
		"_about": map[string]string{
			"purpose":    "Search index for AGENTS.md, active-vendor rules and skills, guardrails, memory context, and recent session summaries.",
			"authority":  "derived and rebuildable",
			"updated_by": "so init, so sync, graph refresh, and documentation updates",
		},
		"documents": docs,
	}, "", "  ")
	if err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(paths.GraphDir, "corpus.json.*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, indexPath(paths)); err != nil {
		if removeErr := os.Remove(indexPath(paths)); removeErr != nil && !os.IsNotExist(removeErr) {
			return 0, err
		}
		if err := os.Rename(tmpName, indexPath(paths)); err != nil {
			return 0, err
		}
	}
	return len(docs), nil
}

func isIndexableRule(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".mdc") ||
		strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".instructions.md")
}

func Search(paths harness.Paths, q string, limit int) ([]Hit, error) {
	return SearchWith(paths, q, SearchOptions{Limit: limit})
}

// SearchOptions tunes corpus ranking for a coding session.
type SearchOptions struct {
	Limit  int
	Vendor string // session meta.Vendor — boosts matching vendor trees; AGENTS.md stays shared/high
}

// SearchWith ranks harness corpus hits, optionally weighting by session vendor.
func SearchWith(paths harness.Paths, q string, opts SearchOptions) ([]Hit, error) {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return nil, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	data, err := os.ReadFile(indexPath(paths))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var corpus struct {
		Documents []IndexDoc `json:"documents"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, err
	}
	docs := corpus.Documents
	var hits []Hit
	for _, d := range docs {
		c := strings.ToLower(d.Content)
		score := 0.0
		if strings.Contains(c, q) {
			score = 3 + float64(strings.Count(c, q))
		} else {
			for _, tok := range strings.Fields(q) {
				if len(tok) > 2 && strings.Contains(c, tok) {
					score++
				}
			}
		}
		if score <= 0 {
			continue
		}
		score *= harness.VendorWeight(d.Path, opts.Vendor)
		snippet := snippetAround(d.Content, q, 200)
		hits = append(hits, Hit{Path: d.Path, Kind: d.Kind, Snippet: snippet, Score: score})
	}
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Score > hits[i].Score {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func snippetAround(content, q string, n int) string {
	lower := strings.ToLower(content)
	idx := strings.Index(lower, strings.ToLower(q))
	if idx < 0 {
		if len(content) > n {
			return content[:n] + "…"
		}
		return content
	}
	start := idx - n/3
	if start < 0 {
		start = 0
	}
	end := start + n
	if end > len(content) {
		end = len(content)
	}
	out := content[start:end]
	if start > 0 {
		out = "…" + out
	}
	if end < len(content) {
		out += "…"
	}
	return out
}
