package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
)

// AgentSource is an existing instruction file found in the repo.
type AgentSource struct {
	Path     string   `json:"path"`
	Kind     string   `json:"kind"` // agents | claude | contributing | <vendor>-rule | <vendor>-skill | …
	Headings []string `json:"headings,omitempty"`
	Rules    []string `json:"rules,omitempty"`
	Excerpt  string   `json:"excerpt,omitempty"`
}

// GraphSummary is a lightweight read of .so/graph/graph.json.
type GraphSummary struct {
	NodeCount   int            `json:"node_count"`
	EdgeCount   int            `json:"edge_count"`
	Languages   map[string]int `json:"languages,omitempty"`
	TopDirs     []string       `json:"top_dirs,omitempty"`
	SampleFiles []string       `json:"sample_files,omitempty"`
}

// Profile is what init uses to seed docs, guardrails, and evals.
type Profile struct {
	Stack       string        `json:"stack"`
	Structure   string        `json:"structure"`
	Graph       GraphSummary  `json:"graph"`
	Agents      []AgentSource `json:"agents"`
	DerivedRules []string     `json:"derived_rules"`
	Themes      []string      `json:"themes"`
}

var (
	headingRe = regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
	bulletRe  = regexp.MustCompile(`(?m)^(?:\s*[-*]|\s*\d+\.)\s+(.+)$`)
	ruleCueRe = regexp.MustCompile(`(?i)\b(never|always|must|must not|do not|don't|avoid|prefer|required|critical|forbidden|ensure)\b`)
	themeCueRe = regexp.MustCompile(`(?i)\b(concurren|race|secret|security|sql|rate.?limit|test|lint|format|pr title|github|credential|mutex|goroutine|migration)\b`)
)

// CollectAgentFiles reads AGENTS.md, CLAUDE.md, vendor rules/skills, etc.
func CollectAgentFiles(repoRoot string) []AgentSource {
	var out []AgentSource
	candidates := []struct {
		rel  string
		kind string
	}{
		{"AGENTS.md", "agents"},
		{"CLAUDE.md", "claude"},
		{"GEMINI.md", "gemini"},
		{"CONTRIBUTING.md", "contributing"},
		{".cursorrules", "cursor-rule"},
		{".github/copilot-instructions.md", "copilot"},
	}
	for _, c := range candidates {
		path := filepath.Join(repoRoot, filepath.FromSlash(c.rel))
		if src, ok := readAgentFile(path, c.kind); ok {
			out = append(out, src)
		}
	}
	for _, dir := range harness.RulesCandidates(repoRoot) {
		kind := harness.KindForRulesDir(dir) + "-rule"
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == "superopen.mdc" || name == "superopen.md" {
				return nil
			}
			lower := strings.ToLower(name)
			if !strings.HasSuffix(lower, ".mdc") && !strings.HasSuffix(lower, ".md") {
				return nil
			}
			if src, ok := readAgentFile(path, kind); ok {
				out = append(out, src)
			}
			return nil
		})
	}
	for _, dir := range harness.SkillsCandidates(repoRoot) {
		kind := harness.KindForSkillsDir(dir) + "-skill"
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() || e.Name() == "so" || e.Name() == "superopen" {
				continue
			}
			path := filepath.Join(dir, e.Name(), "SKILL.md")
			if src, ok := readAgentFile(path, kind); ok {
				out = append(out, src)
			}
		}
	}
	return out
}

func readAgentFile(path, kind string) (AgentSource, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentSource{}, false
	}
	text := stripSuperopenBlock(string(data))
	text = stripFrontmatter(text)
	text = stripCodeFences(text)
	if strings.TrimSpace(text) == "" {
		return AgentSource{}, false
	}
	src := AgentSource{
		Path:    path,
		Kind:    kind,
		Excerpt: truncate(strings.TrimSpace(text), 2000),
	}
	for _, m := range headingRe.FindAllStringSubmatch(text, -1) {
		h := cleanMD(m[1])
		if h != "" && !strings.EqualFold(h, "Superopen") && !strings.EqualFold(h, "Superopen harness") && !strings.EqualFold(h, "SuperOpen harness") {
			src.Headings = append(src.Headings, h)
		}
	}

	negative := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		if strings.HasPrefix(line, "#") {
			negative = isNegativeHeading(lower)
			continue
		}
		if line == "" || strings.HasPrefix(line, "|") || strings.HasPrefix(line, ">") {
			continue
		}
		// Bold section labels like **❌ DON'T:** / **✅ DO:**
		if isSectionMarker(lower) {
			negative = isNegativeHeading(lower)
			continue
		}
		if isNegativeBullet(lower) {
			continue
		}
		if negative {
			continue
		}

		bullet := ""
		if m := bulletRe.FindStringSubmatch(line); len(m) == 2 {
			bullet = m[1]
		} else if ruleCueRe.MatchString(line) {
			bullet = line
		} else {
			continue
		}
		rule := cleanMD(bullet)
		if !usableRule(rule) {
			continue
		}
		if ruleCueRe.MatchString(rule) || themeCueRe.MatchString(rule) {
			src.Rules = append(src.Rules, rule)
		}
	}
	src.Rules = uniqueLimited(src.Rules, 40)
	src.Headings = uniqueLimited(src.Headings, 30)
	return src, true
}

func stripCodeFences(s string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func isSectionMarker(lower string) bool {
	trimmed := strings.Trim(lower, "*_ \t")
	return strings.HasPrefix(trimmed, "✅") ||
		strings.HasPrefix(trimmed, "❌") ||
		strings.HasPrefix(trimmed, "do:") ||
		strings.HasPrefix(trimmed, "don't") ||
		strings.HasPrefix(trimmed, "do not") ||
		strings.HasPrefix(trimmed, "correct") ||
		strings.HasPrefix(trimmed, "incorrect") ||
		strings.HasPrefix(trimmed, "dangerous") ||
		strings.HasPrefix(trimmed, "requirements") ||
		strings.HasPrefix(trimmed, "forbidden")
}

func isNegativeHeading(lower string) bool {
	cues := []string{"don't", "do not", "dangerous", "forbidden", "bad pattern", "never:", "❌", "incorrect", "anti-pattern"}
	for _, c := range cues {
		if strings.Contains(lower, c) {
			return true
		}
	}
	return false
}

func isNegativeBullet(lower string) bool {
	return strings.Contains(lower, "❌") ||
		strings.HasPrefix(lower, "- ❌") ||
		strings.Contains(lower, "**don't**") ||
		strings.Contains(lower, "**never:**")
}

func cleanMD(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "-*• ")
	s = strings.TrimSpace(s)
	// Drop leading checklist markers
	if strings.HasPrefix(s, "[ ]") || strings.HasPrefix(s, "[x]") || strings.HasPrefix(s, "[X]") {
		s = strings.TrimSpace(s[3:])
	}
	return strings.Join(strings.Fields(s), " ")
}

func usableRule(s string) bool {
	if len(s) < 18 || len(s) > 220 {
		return false
	}
	lower := strings.ToLower(s)
	// Skip path-ish leftovers and table/checklist noise
	if strings.HasPrefix(s, "pkg/") || strings.HasPrefix(lower, "files to") {
		return false
	}
	if strings.HasPrefix(lower, "background:") || strings.HasPrefix(lower, "lesson:") {
		return false
	}
	if strings.HasSuffix(s, ":") && len(s) < 48 {
		return false // section labels like "REQUIRED VALIDATIONS:"
	}
	if strings.Count(s, "/") >= 3 && !ruleCueRe.MatchString(s) {
		return false
	}
	if strings.HasPrefix(lower, "pattern ") || strings.HasPrefix(lower, "example") {
		return false
	}
	// Prefer imperative / constraint language over glossary lines
	if !ruleCueRe.MatchString(s) {
		return false
	}
	return true
}

// SummarizeGraph reads .so/graph/graph.json into a compact summary.
func SummarizeGraph(paths harness.Paths) GraphSummary {
	sum := GraphSummary{Languages: map[string]int{}}
	data, err := os.ReadFile(paths.GraphJSON)
	if err != nil {
		return sum
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return sum
	}
	var nodes []map[string]any
	var edges []any
	_ = json.Unmarshal(raw["nodes"], &nodes)
	if err := json.Unmarshal(raw["edges"], &edges); err != nil || len(edges) == 0 {
		_ = json.Unmarshal(raw["links"], &edges) // Graphify / NetworkX
	}
	sum.NodeCount = len(nodes)
	sum.EdgeCount = len(edges)

	dirCount := map[string]int{}
	for _, n := range nodes {
		sf, _ := n["source_file"].(string)
		id, _ := n["id"].(string)
		path := sf
		if path == "" {
			path = id
		}
		if path == "" || path == "." {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			sum.Languages["go"]++
		case ".ts", ".tsx":
			sum.Languages["typescript"]++
		case ".js", ".jsx":
			sum.Languages["javascript"]++
		case ".py":
			sum.Languages["python"]++
		case ".rs":
			sum.Languages["rust"]++
		case ".java":
			sum.Languages["java"]++
		case ".md":
			sum.Languages["markdown"]++
		}
		top := strings.Split(filepath.ToSlash(path), "/")[0]
		if top != "" && !strings.HasPrefix(top, ".") {
			dirCount[top]++
		}
		if len(sum.SampleFiles) < 12 && ext != "" && !strings.HasSuffix(path, "/") {
			sum.SampleFiles = append(sum.SampleFiles, path)
		}
	}
	type kv struct {
		k string
		v int
	}
	var dirs []kv
	for k, v := range dirCount {
		dirs = append(dirs, kv{k, v})
	}
	for i := 0; i < len(dirs); i++ {
		for j := i + 1; j < len(dirs); j++ {
			if dirs[j].v > dirs[i].v {
				dirs[i], dirs[j] = dirs[j], dirs[i]
			}
		}
	}
	for i := 0; i < len(dirs) && i < 10; i++ {
		sum.TopDirs = append(sum.TopDirs, dirs[i].k)
	}
	return sum
}

// BuildProfile combines stack/structure, graph, and agent instruction files.
func BuildProfile(repoRoot string, paths harness.Paths, stack, structure string) Profile {
	agents := CollectAgentFiles(repoRoot)
	g := SummarizeGraph(paths)
	p := Profile{
		Stack:     stack,
		Structure: structure,
		Graph:     g,
		Agents:    agents,
	}
	seen := map[string]bool{}
	for _, a := range agents {
		for _, r := range a.Rules {
			k := strings.ToLower(r)
			if seen[k] {
				continue
			}
			seen[k] = true
			p.DerivedRules = append(p.DerivedRules, r)
		}
		for _, h := range a.Headings {
			if themeCueRe.MatchString(h) {
				p.Themes = append(p.Themes, h)
			}
		}
	}
	p.DerivedRules = uniqueLimited(p.DerivedRules, 25)
	p.Themes = uniqueLimited(p.Themes, 15)

	// Infer themes from graph languages / stack
	if g.Languages["go"] > 0 || strings.Contains(strings.ToLower(stack), "go") {
		p.Themes = append(p.Themes, "Go toolchain")
	}
	p.Themes = uniqueLimited(p.Themes, 15)
	return p
}

func stripSuperopenBlock(s string) string {
	start := strings.Index(s, "<!-- superopen:start -->")
	end := strings.Index(s, "<!-- superopen:end -->")
	if start >= 0 && end > start {
		end += len("<!-- superopen:end -->")
		return s[:start] + s[end:]
	}
	return s
}

func stripFrontmatter(s string) string {
	if !strings.HasPrefix(strings.TrimSpace(s), "---") {
		return s
	}
	parts := strings.SplitN(strings.TrimSpace(s), "---", 3)
	if len(parts) >= 3 {
		return parts[2]
	}
	return s
}

func uniqueLimited(in []string, n int) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if s == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
		if len(out) >= n {
			break
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
