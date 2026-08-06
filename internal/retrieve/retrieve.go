// Package retrieve indexes harness corpus files for offline keyword search.
package retrieve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/superopen/so/internal/harness"
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
	return filepath.Join(paths.GraphDir, "retrieve_index.json")
}

// Rebuild walks harness corpus into a simple keyword index.
func Rebuild(repoRoot string, paths harness.Paths) (int, error) {
	var docs []IndexDoc
	addFile := func(path, kind string) {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			return
		}
		rel, _ := filepath.Rel(repoRoot, path)
		if rel == "" || strings.HasPrefix(rel, "..") {
			rel = path
		}
		text := string(data)
		if len(text) > 100_000 {
			text = text[:100_000]
		}
		docs = append(docs, IndexDoc{Path: rel, Kind: kind, Content: text})
	}
	walk := func(dir, kind string) {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".md", ".yaml", ".yml", ".json", ".jsonl", ".txt":
				addFile(path, kind)
			}
			return nil
		})
	}
	walk(paths.KnowledgeDir, "knowledge")
	walk(paths.RulesDir, "rules")
	walk(paths.SkillsDir, "skills")
	walk(paths.GuardrailsDir, "guardrails")
	walk(paths.MemoryDir, "memory")
	if data, err := os.ReadFile(paths.GraphJSON); err == nil {
		docs = append(docs, IndexDoc{Path: ".so/graph/graph.json", Kind: "graph", Content: string(data)})
	}
	if data, err := os.ReadFile(paths.GraphReport); err == nil {
		docs = append(docs, IndexDoc{Path: ".so/graph/GRAPH_REPORT.md", Kind: "graph", Content: string(data)})
	}
	// recent session metas
	_ = filepath.WalkDir(paths.SessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "meta.json" || d.Name() == "footprint.json" {
			addFile(path, "session")
		}
		return nil
	})
	if err := os.MkdirAll(paths.GraphDir, 0o755); err != nil {
		return 0, err
	}
	raw, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(indexPath(paths), raw, 0o644); err != nil {
		return 0, err
	}
	return len(docs), nil
}

func Search(paths harness.Paths, q string, limit int) ([]Hit, error) {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return nil, nil
	}
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
	var docs []IndexDoc
	if err := json.Unmarshal(data, &docs); err != nil {
		return nil, err
	}
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
