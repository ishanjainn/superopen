package eval

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

// Result mirrors process dimensions + optional task notes.
type Result struct {
	SessionID      string             `json:"session_id"`
	At             time.Time          `json:"at"`
	Dimensions     map[string]float64 `json:"dimensions"`
	Notes          []string           `json:"notes,omitempty"`
	Score          float64            `json:"score"`
	Badge          string             `json:"badge"`
	EvidenceStatus string             `json:"evidence_status,omitempty"` // sufficient | insufficient
	Backend        string             `json:"backend"`
	// HotAreas are repo-relative dirs that dominated file activity (for nested AGENTS.md recs).
	HotAreas []string                `json:"hot_areas,omitempty"`
	Verified bool                    `json:"verified,omitempty"`
	Findings []session.ReviewFinding `json:"-"`
	Drafts   []Draft                 `json:"-"`
	Memory   MemoryDelta             `json:"-"`
}

// Draft is recommendation content returned by the same reviewer invocation
// that scores the session. It is deliberately excluded from evaluation JSON;
// the recommendation record is the sole owner of proposed file bodies.
type Draft struct {
	Fingerprint string
	Type        string
	Title       string
	Rationale   string
	Path        string
	Body        string
	ChangeKind  string
	Evidence    []string
}

type MemoryDelta struct {
	Lessons     []string
	Preference  string
	ProjectNote string
}

type activitySignals struct {
	reads       int
	edits       int
	searches    int
	toolCalls   int
	harnessHits int
	files       map[string]bool
	failedTools int
	verified    bool
	toolNames   []string
}

func collectActivitySignals(spans []tracestore.Span) activitySignals {
	s := activitySignals{files: map[string]bool{}}
	for _, sp := range spans {
		name := strings.ToLower(sp.Name)
		attrs := sp.Attributes
		toolName := strings.ToLower(attrs["gen_ai.tool.name"] + " " + attrs["coding_agent.tool.name"])
		arguments := strings.ToLower(attrs["gen_ai.tool.call.arguments"] + " " + attrs["coding_agent.tool.arguments"] + " " + attrs["coding_agent.tool.command"])
		completedTool := strings.Contains(name, "tool.call") || strings.Contains(name, "tool.execute")
		if completedTool {
			s.toolCalls++
			if toolName != "" {
				s.toolNames = append(s.toolNames, strings.Fields(toolName)[0])
			}
			if attrs["coding_agent.tool.errored"] == "true" || strings.EqualFold(sp.Status, "error") || attrs["error.type"] != "" {
				s.failedTools++
			}
			if isVerificationCommand(arguments) && attrs["coding_agent.tool.errored"] != "true" && !strings.EqualFold(sp.Status, "error") && attrs["error.type"] == "" {
				s.verified = true
			}
		}

		readSignal := strings.Contains(name, "read") || containsAny(toolName, "read", "open", "view")
		editSignal := strings.Contains(name, "edit") || strings.Contains(name, "write") || containsAny(toolName, "edit", "write", "apply_patch")
		searchSignal := strings.Contains(name, "search") || strings.Contains(name, "grep") || strings.Contains(name, "glob") || containsAny(toolName, "search", "grep", "glob")
		if completedTool {
			readSignal = readSignal || containsAny(arguments, "sed -n", "cat ", "head ", "tail ", "jq ")
			editSignal = editSignal || containsAny(arguments, "apply_patch", "gofmt -w")
			searchSignal = searchSignal || containsAny(arguments, "rg ", "grep ", "find ")
		}
		if readSignal {
			s.reads++
		}
		if editSignal {
			s.edits++
		}
		if searchSignal {
			s.searches++
		}

		for _, key := range []string{"coding_agent.file_path", "code.file.path"} {
			if p := attrs[key]; p != "" {
				s.files[p] = true
				if isHarnessReference(strings.ToLower(p)) {
					s.harnessHits++
				}
			}
		}
		if completedTool && isHarnessReference(toolName+" "+arguments) {
			s.harnessHits++
		}
	}
	return s
}

func isVerificationCommand(value string) bool {
	value = strings.ToLower(value)
	return containsAny(value, "go test", "npm test", "npm run test", "pnpm test", "yarn test", "pytest", "cargo test", "go vet", "npm run typecheck", "npm run lint")
}

var durableCorrectionRE = regexp.MustCompile(`(?i)\b(always|never|from now on|our standard|we prefer|do not|don't|instead|that is wrong|that's wrong|remember to)\b`)

func reviewText(spans []tracestore.Span) string {
	var parts []string
	for _, sp := range spans {
		for _, key := range []string{"gen_ai.prompt", "coding_agent.prompt", "coding_agent.user_prompt", "gen_ai.input.messages"} {
			if value := strings.TrimSpace(sp.Attributes[key]); value != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func isHarnessReference(value string) bool {
	return containsAny(value,
		".so/", "agents.md", "/agents.md", "claude.md",
		".claude/rules", ".cursor/rules", ".agents/rules", ".codex/rules",
		".claude/skills", ".cursor/skills", ".agents/skills", ".pi/skills",
		"so graph", "so query", "so knowledge", "so rules", "so guard", "so skill", "/so",
	)
}

// hotAreasFromFiles returns up to 2 concentrated package dirs from touched files.
func hotAreasFromFiles(files map[string]bool) []string {
	counts := map[string]int{}
	for p := range files {
		area := guidanceArea(p)
		if area == "" {
			continue
		}
		counts[area]++
	}
	type kv struct {
		k string
		v int
	}
	var ranked []kv
	for k, v := range counts {
		ranked = append(ranked, kv{k, v})
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].v > ranked[i].v {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	var out []string
	total := 0
	for _, r := range ranked {
		total += r.v
	}
	for i := 0; i < len(ranked) && i < 2; i++ {
		// Require the area to account for a meaningful share of file touches.
		if total > 0 && float64(ranked[i].v)/float64(total) < 0.35 && ranked[i].v < 3 {
			continue
		}
		out = append(out, ranked[i].k)
	}
	return out
}

func guidanceArea(path string) string {
	p := filepath.ToSlash(path)
	p = strings.TrimPrefix(p, "./")
	// Drop absolute prefix noise by taking from known roots.
	for _, root := range []string{"internal/", "cmd/", "web/", "sdk/", "plugins/", "templates/", "docs/"} {
		if i := strings.Index(p, root); i >= 0 {
			p = p[i:]
			break
		}
	}
	parts := strings.Split(p, "/")
	if len(parts) == 0 || parts[0] == "" || strings.HasPrefix(parts[0], ".") {
		return ""
	}
	switch parts[0] {
	case "internal", "cmd", "web", "sdk", "plugins":
		if len(parts) >= 2 && parts[1] != "" && !strings.Contains(parts[1], ".") {
			return parts[0] + "/" + parts[1]
		}
		return parts[0]
	default:
		return ""
	}
}

func deterministicFindings(paths harness.Paths, vendor, sessionID string, spans []tracestore.Span, signals activitySignals) []session.ReviewFinding {
	vendor = harness.NormalizeVendorKind(vendor)
	if vendor == "" {
		return nil
	}
	var out []session.ReviewFinding
	text := reviewText(spans)
	if match := durableCorrectionRE.FindString(text); match != "" {
		summary := "The user supplied a durable correction or workflow preference during this session."
		finding := newFinding(paths, vendor, "correction", "update", "memory", "", summary, 0.85, signals.verified, true,
			[]string{"durable user wording detected", "event evidence retained in session events"}, eventIDs(spans, 4))
		// The correction text is used only to separate unrelated patterns; it
		// is not copied into session review or durable memory.
		sum := sha256.Sum256([]byte(vendor + "|correction|" + normalizeSummary(text)))
		finding.Fingerprint = fmt.Sprintf("pattern_%x", sum[:10])
		out = append(out, finding)
	}
	if signals.failedTools > 0 {
		summary := fmt.Sprintf("The session encountered %d failed tool operation(s) before completing the work.", signals.failedTools)
		out = append(out, newFinding(paths, vendor, "failure", "update", "rules", harness.RulesDirForVendor(paths.RepoRoot, vendor), summary, 0.65, signals.verified, false,
			[]string{fmt.Sprintf("failed_tools=%d", signals.failedTools)}, eventIDs(spans, 4)))
	}
	if repeated := repeatedWorkflow(signals.toolNames); repeated != "" {
		summary := "A repeatable tool workflow appeared more than once: " + repeated + "."
		finding := newFinding(paths, vendor, "workflow", "create", "skill", harness.SkillsDirForVendor(paths.RepoRoot, vendor), summary, 0.7, signals.verified, false,
			[]string{"repeated workflow=" + repeated}, eventIDs(spans, 6))
		sum := sha256.Sum256([]byte(vendor + "|workflow|" + repeated))
		finding.Fingerprint = fmt.Sprintf("pattern_%x", sum[:10])
		out = append(out, finding)
	}
	if signals.verified && signals.edits > 0 {
		summary := "Repository edits were followed by a successful verification command."
		out = append(out, newFinding(paths, vendor, "success", "update", "memory", "", summary, 0.7, true, false,
			[]string{"edited files were verified"}, eventIDs(spans, 4)))
	}
	return out
}

func repeatedWorkflow(names []string) string {
	if len(names) < 6 {
		return ""
	}
	for width := 4; width >= 3; width-- {
		seen := map[string]bool{}
		for i := 0; i+width <= len(names); i++ {
			seq := strings.Join(names[i:i+width], " → ")
			if seen[seq] {
				return seq
			}
			seen[seq] = true
		}
	}
	return ""
}

func eventIDs(spans []tracestore.Span, limit int) []string {
	var ids []string
	for _, sp := range spans {
		if sp.SpanID == "" {
			continue
		}
		ids = append(ids, sp.SpanID)
		if len(ids) == limit {
			break
		}
	}
	return ids
}

func newFinding(paths harness.Paths, vendor, kind, changeKind, targetType, targetPath, summary string, confidence float64, verified, explicit bool, evidence, ids []string) session.ReviewFinding {
	if rel, err := filepath.Rel(paths.RepoRoot, targetPath); err == nil && targetPath != "" && !strings.HasPrefix(rel, "..") {
		targetPath = filepath.ToSlash(rel)
	}
	// Fingerprints are based on normalized ownership and target identity, never
	// reviewer prose, so wording changes do not split one durable pattern.
	key := strings.Join([]string{vendor, kind, changeKind, targetType, filepath.ToSlash(targetPath)}, "|")
	sum := sha256.Sum256([]byte(key))
	return session.ReviewFinding{
		Fingerprint: fmt.Sprintf("pattern_%x", sum[:10]), Kind: kind, ChangeKind: changeKind,
		Summary: summary, Vendor: vendor, TargetType: targetType, TargetPath: targetPath,
		Confidence: confidence, Verified: verified, ExplicitWorkflow: explicit,
		Evidence: evidence, EventIDs: ids,
	}
}

func normalizeSummary(value string) string {
	value = strings.ToLower(value)
	value = regexp.MustCompile(`\d+`).ReplaceAllString(value, "#")
	return strings.Join(strings.Fields(value), " ")
}

func sessionVendor(paths harness.Paths, sessionID string, spans []tracestore.Span) string {
	if meta, err := session.NewStore(paths).Get(sessionID); err == nil && meta.Vendor != "" {
		return meta.Vendor
	}
	return session.VendorFromSpans(spans)
}

type reviewerFinding struct {
	Kind             string   `json:"kind"`
	ChangeKind       string   `json:"change_kind"`
	Summary          string   `json:"summary"`
	TargetType       string   `json:"target_type"`
	TargetPath       string   `json:"target_path"`
	Confidence       float64  `json:"confidence"`
	Verified         bool     `json:"verified"`
	ExplicitWorkflow bool     `json:"explicit_workflow"`
	Evidence         []string `json:"evidence"`
	EventIDs         []string `json:"event_ids"`
	Keywords         []string `json:"keywords"`
	Paths            []string `json:"paths"`
	Symbols          []string `json:"symbols"`
	ErrorSignatures  []string `json:"error_signatures"`
	Applicability    string   `json:"applicability"`
	WorkflowShape    string   `json:"workflow_shape"`
	Title            string   `json:"title"`
	Rationale        string   `json:"rationale"`
	ProposedBody     string   `json:"proposed_body"`
}

type reviewerResult struct {
	Exploration  *float64          `json:"exploration"`
	Scope        *float64          `json:"scope"`
	Wandering    *float64          `json:"wandering"`
	Verification *float64          `json:"verification"`
	Note         string            `json:"note"`
	Findings     []reviewerFinding `json:"findings"`
	Memory       MemoryDelta       `json:"memory"`
}

func (r reviewerResult) dimension(key string) float64 {
	var value *float64
	switch key {
	case "exploration":
		value = r.Exploration
	case "scope":
		value = r.Scope
	case "wandering":
		value = r.Wandering
	case "verification":
		value = r.Verification
	}
	if value == nil {
		return -1
	}
	return clamp01(*value)
}

func (r reviewerResult) toFindings(paths harness.Paths, vendor string, sessionVerified bool) ([]session.ReviewFinding, []Draft) {
	vendor = harness.NormalizeVendorKind(vendor)
	var findings []session.ReviewFinding
	var drafts []Draft
	for _, item := range r.Findings {
		item.Kind = strings.ToLower(strings.TrimSpace(item.Kind))
		item.ChangeKind = strings.ToLower(strings.TrimSpace(item.ChangeKind))
		item.TargetType = strings.ToLower(strings.TrimSpace(item.TargetType))
		item.Summary = truncateText(item.Summary, 320)
		if item.Summary == "" || vendor == "" || !allowedFindingKind(item.Kind) || !allowedChangeKind(item.ChangeKind) || !allowedTargetType(item.TargetType) {
			continue
		}
		path, ok := resolveFindingPath(paths, vendor, item.TargetType, item.TargetPath)
		if !ok {
			continue
		}
		finding := newFinding(paths, vendor, item.Kind, item.ChangeKind, item.TargetType, path, item.Summary,
			clamp01(item.Confidence), item.Verified || sessionVerified, item.ExplicitWorkflow,
			compactEvidence(item.Evidence), compactEvidence(item.EventIDs))
		finding.Keywords = compactEvidence(item.Keywords)
		finding.Paths = compactEvidence(item.Paths)
		finding.Symbols = compactEvidence(item.Symbols)
		finding.ErrorSignatures = compactEvidence(item.ErrorSignatures)
		finding.Applicability = truncateText(item.Applicability, 240)
		if item.WorkflowShape != "" {
			finding.Fingerprint = findingFingerprint(vendor, item.Kind, item.ChangeKind, item.TargetType, path,
				item.WorkflowShape, finding.Paths, finding.Symbols, finding.ErrorSignatures)
		}
		findings = append(findings, finding)
		if strings.TrimSpace(item.ProposedBody) == "" || item.ChangeKind == "remove" || item.ChangeKind == "restructure" || item.TargetType == "memory" || item.Kind == "product_gap" {
			continue
		}
		drafts = append(drafts, Draft{
			Fingerprint: finding.Fingerprint, Type: item.TargetType, Title: truncateText(item.Title, 140),
			Rationale: truncateText(item.Rationale, 400), Path: path, Body: strings.TrimSpace(item.ProposedBody) + "\n",
			ChangeKind: item.ChangeKind, Evidence: finding.Evidence,
		})
	}
	return findings, drafts
}

func findingFingerprint(vendor, kind, changeKind, targetType, targetPath, workflow string, paths, symbols, errors []string) string {
	normalize := func(values []string) string {
		for i := range values {
			values[i] = normalizeSummary(filepath.ToSlash(values[i]))
		}
		sort.Strings(values)
		return strings.Join(values, ",")
	}
	key := strings.Join([]string{vendor, kind, changeKind, targetType, filepath.ToSlash(targetPath), normalizeSummary(workflow), normalize(append([]string(nil), paths...)), normalize(append([]string(nil), symbols...)), normalize(append([]string(nil), errors...))}, "|")
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("pattern_%x", sum[:10])
}

func allowedFindingKind(value string) bool {
	switch value {
	case "correction", "workflow", "failure", "success", "guidance_gap", "simplification", "product_gap":
		return true
	}
	return false
}

func allowedChangeKind(value string) bool {
	switch value {
	case "create", "update", "remove", "restructure":
		return true
	}
	return false
}

func allowedTargetType(value string) bool {
	switch value {
	case "skill", "rules", "docs", "memory", "guardrail", "eval":
		return true
	}
	return false
}

func resolveFindingPath(paths harness.Paths, vendor, targetType, proposed string) (string, bool) {
	proposed = filepath.Clean(filepath.FromSlash(strings.TrimSpace(proposed)))
	if proposed == "." {
		proposed = ""
	}
	var allowedRoot string
	switch targetType {
	case "skill":
		allowedRoot = harness.SkillsDirForVendor(paths.RepoRoot, vendor)
	case "rules":
		allowedRoot = harness.RulesDirForVendor(paths.RepoRoot, vendor)
	case "docs":
		allowedRoot = paths.RepoRoot
	case "guardrail":
		return paths.GuardrailsFile, true
	case "eval":
		return paths.EvalsConfig, true
	case "memory":
		return "", true
	}
	if allowedRoot == "" {
		return "", false
	}
	if proposed == "" {
		return allowedRoot, false
	}
	abs := proposed
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(paths.RepoRoot, proposed)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", false
	}
	root, _ := filepath.Abs(allowedRoot)
	if targetType == "docs" {
		if filepath.Base(abs) != "AGENTS.md" || (abs != filepath.Join(paths.RepoRoot, "AGENTS.md") && !strings.HasPrefix(abs, root+string(os.PathSeparator))) {
			return "", false
		}
	} else if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", false
	}
	base := strings.ToLower(filepath.Base(abs))
	if targetType == "skill" && (base != "skill.md" || strings.Contains(strings.ToLower(filepath.ToSlash(abs)), "/skills/so/") || strings.Contains(strings.ToLower(filepath.ToSlash(abs)), "/skills/superopen/")) {
		return "", false
	}
	return abs, true
}

func currentGuidance(paths harness.Paths, vendor string, limit int) string {
	files := []string{paths.AgentsMD, paths.GuardrailsFile, paths.EvalsConfig}
	for _, dir := range []string{harness.RulesDirForVendor(paths.RepoRoot, vendor), harness.SkillsDirForVendor(paths.RepoRoot, vendor)} {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (strings.HasSuffix(strings.ToLower(path), ".md") || strings.HasSuffix(strings.ToLower(path), ".mdc")) {
				files = append(files, path)
			}
			return nil
		})
	}
	sort.Strings(files)
	var b strings.Builder
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(paths.RepoRoot, path)
		b.WriteString("### " + filepath.ToSlash(rel) + "\n")
		b.WriteString(truncateText(string(data), 1200) + "\n")
		if b.Len() >= limit {
			break
		}
	}
	return truncateText(b.String(), limit)
}

func mergeFindings(existing, incoming []session.ReviewFinding) []session.ReviewFinding {
	byKey := map[string]session.ReviewFinding{}
	for _, finding := range append(existing, incoming...) {
		if prev, ok := byKey[finding.Fingerprint]; ok {
			if finding.Confidence > prev.Confidence {
				prev = finding
			}
			prev.Verified = prev.Verified || finding.Verified
			prev.ExplicitWorkflow = prev.ExplicitWorkflow || finding.ExplicitWorkflow
			prev.Evidence = compactEvidence(append(prev.Evidence, finding.Evidence...))
			byKey[finding.Fingerprint] = prev
			continue
		}
		byKey[finding.Fingerprint] = finding
	}
	out := make([]session.ReviewFinding, 0, len(byKey))
	for _, finding := range byKey {
		out = append(out, finding)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out
}

func compactEvidence(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = truncateText(value, 240)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) == 6 {
			break
		}
	}
	return out
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func Run(paths harness.Paths, cfg config.Config, sessionID string, spans []tracestore.Span, completer llm.Completer) (Result, error) {
	res := Result{
		SessionID: sessionID,
		At:        time.Now().UTC(),
		Dimensions: map[string]float64{
			"exploration":  0.7,
			"scope":        0.7,
			"wandering":    0.3,
			"verification": 0.5,
			"harness_use":  0.5,
		},
	}

	// Deterministic signals from normalized span names, tool names, and tool
	// arguments. Codex uses generic coding_agent.tool.call span names, so the
	// operation must not be inferred from the span name alone.
	signals := collectActivitySignals(spans)
	if signals.toolCalls == 0 && signals.reads == 0 && signals.edits == 0 && signals.searches == 0 && len(signals.files) == 0 {
		res.Dimensions = map[string]float64{}
		res.Score = 0
		res.Badge = "unknown"
		res.EvidenceStatus = "insufficient"
		res.Notes = append(res.Notes, "Insufficient activity telemetry to score this session.")
		return persistResult(paths, res)
	}
	res.EvidenceStatus = "sufficient"
	res.Verified = signals.verified
	res.HotAreas = hotAreasFromFiles(signals.files)
	vendor := sessionVendor(paths, sessionID, spans)
	res.Findings = deterministicFindings(paths, vendor, sessionID, spans, signals)
	if signals.searches > 15 {
		res.Dimensions["wandering"] = 0.8
		res.Dimensions["exploration"] = 0.4
		note := fmt.Sprintf("Problem: %d search tools ran with little reuse of existing guidance. Impact: next similar task will re-discover the same paths and burn tokens.", signals.searches)
		if len(res.HotAreas) > 0 {
			note += " Hot area: " + strings.Join(res.HotAreas, ", ") + "."
		}
		res.Notes = append(res.Notes, note)
	}
	if signals.harnessHits > 0 {
		res.Dimensions["harness_use"] = 0.9
		res.Notes = append(res.Notes, "Harness guidance was consulted (AGENTS.md / rules / skills / so graph) — good reuse for the next session.")
	} else if len(spans) > 5 {
		res.Dimensions["harness_use"] = 0.2
		res.Notes = append(res.Notes, "Problem: session never opened AGENTS.md, project rules/skills, or so graph. Impact: agents rediscover structure instead of following durable guidance.")
	}
	if signals.edits > 0 && signals.reads == 0 {
		res.Dimensions["scope"] = 0.4
		res.Notes = append(res.Notes, "Problem: edits landed without prior reads. Impact: higher risk of unrelated churn and harder review.")
	}

	backend := strings.ToLower(strings.TrimSpace(cfg.Evals.Backend))
	if backend == "" {
		backend = "auto"
	}
	res.Backend = "heuristics"
	wantModel := backend != "heuristics" && backend != "none" && backend != "off"
	if wantModel && completer != nil && completer.Available() {
		summary := fmt.Sprintf("session=%s vendor=%s reads=%d edits=%d searches=%d tool_calls=%d failed_tools=%d verified=%v harness_hits=%d files=%d notes=%v\n\nREDACTED SESSION TEXT:\n%s\n\nCURRENT GUIDANCE:\n%s",
			sessionID, harness.NormalizeVendorKind(vendor), signals.reads, signals.edits, signals.searches, signals.toolCalls, signals.failedTools, signals.verified, signals.harnessHits, len(signals.files), res.Notes,
			truncateText(reviewText(spans), 5000), currentGuidance(paths, vendor, 5000))
		out, err := completer.Complete(
			`Review one coding-agent session and return JSON only:
{"exploration":0,"scope":0,"wandering":0,"verification":0,"note":"","findings":[{"kind":"correction|workflow|failure|success|guidance_gap|simplification|product_gap","change_kind":"create|update|remove|restructure","summary":"","target_type":"skill|rules|docs|memory|guardrail|eval","target_path":"repository-relative path","confidence":0,"verified":false,"explicit_workflow":false,"evidence":["short redacted fact"],"event_ids":["span id"],"keywords":["normalized term"],"paths":["repository-relative path"],"symbols":["symbol"],"error_signatures":["stable error signature"],"applicability":"when this applies","workflow_shape":"stable action sequence without generated prose","title":"","rationale":"","proposed_body":""}],"memory":{"lessons":[],"preference":"","project_note":""}}
Use existing guidance before proposing anything. Prefer no finding over weak advice. A removal or restructure may be recommended but must never be auto-applied. Never target another vendor, .agents, or a managed so/superopen skill. Proposed bodies must be complete and concise. Do not include prompts or tool output verbatim in evidence.`,
			summary,
		)
		if err == nil {
			res.Backend = completer.Backend()
			res.Notes = append(res.Notes, "model:"+completer.Backend())
			var parsed reviewerResult
			if json.Unmarshal([]byte(extractJSON(out)), &parsed) == nil {
				for _, k := range []string{"exploration", "scope", "wandering", "verification"} {
					if v := parsed.dimension(k); v >= 0 {
						res.Dimensions[k] = v
					}
				}
				if parsed.Note != "" {
					res.Notes = append(res.Notes, parsed.Note)
				}
				modelFindings, drafts := parsed.toFindings(paths, vendor, signals.verified)
				for i := range modelFindings {
					modelFindings[i].EventIDs = eventIDs(spans, 6)
				}
				res.Findings = mergeFindings(res.Findings, modelFindings)
				res.Drafts = drafts
				res.Memory = parsed.Memory
			}
		} else {
			res.Notes = append(res.Notes, "model enrichment skipped: "+err.Error())
		}
	}

	sum := 0.0
	for k, v := range res.Dimensions {
		if k == "wandering" {
			sum += 1 - v
		} else {
			sum += v
		}
	}
	res.Score = sum / float64(len(res.Dimensions))
	switch {
	case res.Score >= 0.75:
		res.Badge = "good"
	case res.Score >= 0.45:
		res.Badge = "ok"
	default:
		res.Badge = "poor"
	}

	return persistResult(paths, res)
}

func persistResult(paths harness.Paths, res Result) (Result, error) {
	ss := session.NewStore(paths)
	data, _ := json.Marshal(res)
	now := time.Now().UTC()
	if err := ss.WriteDocument(res.SessionID, func(d *session.Document) {
		d.Evaluation = data
		d.EvalBadge = res.Badge
		d.Review.Findings = res.Findings
		d.Review.Status = "complete"
		d.Review.Backend = res.Backend
		d.Review.CompletedAt = &now
		d.Review.Error = ""
	}); err != nil {
		return res, err
	}
	if meta, err := ss.Get(res.SessionID); err == nil {
		_ = ss.UpdateMeta(meta)
	}
	return res, nil
}

// LatestResult returns the materialized latest evaluation for a session.
// Callers use it to avoid re-evaluating a closed chat whose final result is
// already newer than the session's ended_at timestamp.
func LatestResult(paths harness.Paths, sessionID string) (Result, bool) {
	doc, err := session.NewStore(paths).ReadDocument(sessionID)
	if err != nil || len(doc.Evaluation) == 0 {
		return Result{}, false
	}
	var res Result
	if json.Unmarshal(doc.Evaluation, &res) != nil || res.SessionID == "" || res.At.IsZero() {
		return Result{}, false
	}
	return res, true
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
