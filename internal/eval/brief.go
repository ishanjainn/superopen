package eval

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

// ReviewerSystemPrompt is the live-agent / sealed-CLI judge instruction.
const ReviewerSystemPrompt = `Review one coding-agent session and return JSON only:
{"exploration":null,"scope":null,"wandering":null,"verification":null,"note":"","findings":[{"kind":"correction|workflow|failure|success|guidance_gap|simplification|product_gap","change_kind":"create|update|remove|restructure","summary":"","target_type":"skill|rules|docs|memory|guardrail|eval","target_path":"repository-relative path","confidence":0,"verified":false,"explicit_workflow":false,"evidence":["short redacted fact"],"event_ids":["span id"],"keywords":["normalized term"],"paths":["repository-relative path"],"symbols":["symbol"],"error_signatures":["stable error signature"],"applicability":"when this applies","workflow_shape":"stable action sequence without generated prose","title":"","rationale":"","proposed_body":""}],"memory":{"lessons":[],"preference":"","project_note":""}}
Use null for dimensions that cannot be judged from evidence. Scope and verification are null when no repository edits occurred. This is a snapshot unless scope=complete in the summary; snapshots must return no findings or memory changes. Use existing guidance before proposing anything. Prefer no finding over weak advice. Never infer a reusable workflow from repeated generic shell invocations. A removal or restructure may be recommended but must never be auto-applied. Never target another vendor, .agents, or a managed so/superopen skill. Proposed bodies must be complete and concise. Do not include prompts or tool output verbatim in evidence.`

// Brief is the system+user prompt for so review-brief.
type Brief struct {
	SessionID string
	Vendor    string
	Status    string
	System    string
	User      string
}

func durableReview(backend string, cfg config.Config) bool {
	b := strings.ToLower(strings.TrimSpace(backend))
	if strings.HasPrefix(b, "live_agent:") || strings.HasPrefix(b, "agent_cli:") || strings.HasPrefix(b, "llm_api:") {
		return true
	}
	return b == "heuristics" && cfg.ExplicitHeuristics()
}

func reviewerUserPrompt(sessionID, vendor, scope string, signals activitySignals, notes []string, spans []tracestore.Span, paths harness.Paths) string {
	return fmt.Sprintf("session=%s vendor=%s scope=%s reads=%d edits=%d searches=%d tool_calls=%d failed_tools=%d verified=%v harness_hits=%d files=%d notes=%v\n\nREDACTED SESSION TEXT:\n%s\n\nCURRENT GUIDANCE:\n%s",
		sessionID, harness.NormalizeVendorKind(vendor), scope, signals.reads, signals.edits, signals.searches, signals.toolCalls, signals.failedTools, signals.verified, signals.harnessHits, len(signals.files), notes,
		truncateText(reviewText(spans), 5000), currentGuidance(paths, vendor, 5000))
}

func applyReviewerParse(res *Result, parsed reviewerResult, paths harness.Paths, vendor string, signals activitySignals, spans []tracestore.Span, final bool) {
	for _, k := range []string{"exploration", "scope", "wandering", "verification"} {
		if _, applicable := res.Dimensions[k]; !applicable {
			continue
		}
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
	if final {
		res.Findings = mergeFindings(res.Findings, modelFindings)
		res.Drafts = drafts
		res.Memory = parsed.Memory
	}
}

// BuildBrief prints the live-agent review prompt for a session.
func BuildBrief(paths harness.Paths, sessionID, vendorHint string, spans []tracestore.Span) (Brief, error) {
	sess := session.NewStore(paths)
	doc, err := sess.ReadDocument(sessionID)
	status := ""
	if err == nil {
		status = doc.Review.Status
	}
	meta, _ := sess.Get(sessionID)
	vendor := strings.TrimSpace(vendorHint)
	if vendor == "" {
		vendor = meta.Vendor
	}
	if vendor == "" {
		vendor = session.VendorFromSpans(spans)
	}
	signals := collectActivitySignals(spans)
	scope := "snapshot"
	if meta.Status == session.StatusEnded {
		scope = "complete"
	}
	notes := []string{}
	if status == "complete" || status == "running" {
		notes = append(notes, "review.status="+status+" — skip apply-review")
	}
	return Brief{
		SessionID: sessionID,
		Vendor:    harness.NormalizeVendorKind(vendor),
		Status:    status,
		System:    ReviewerSystemPrompt,
		User:      reviewerUserPrompt(sessionID, vendor, scope, signals, notes, spans, paths),
	}, nil
}

// ApplyJSON merges live-agent reviewer JSON onto deterministic signals and persists a complete review.
func ApplyJSON(paths harness.Paths, cfg config.Config, sessionID, backend string, raw []byte, spans []tracestore.Span) (Result, error) {
	res, err := Run(paths, cfg, sessionID, spans, nil, RunOptions{Final: true})
	if err != nil {
		return res, err
	}
	var parsed reviewerResult
	if json.Unmarshal([]byte(extractJSON(string(raw))), &parsed) != nil {
		return res, fmt.Errorf("apply-review: JSON must include the reviewer object")
	}
	vendor := sessionVendor(paths, sessionID, spans)
	signals := collectActivitySignals(spans)
	applyReviewerParse(&res, parsed, paths, vendor, signals, spans, true)
	if strings.TrimSpace(backend) != "" {
		res.Backend = backend
	}
	res.EvaluationScope = "complete"
	res.CompleteReview = durableReview(res.Backend, cfg)
	sum := 0.0
	for k, v := range res.Dimensions {
		if k == "wandering" {
			sum += 1 - v
		} else {
			sum += v
		}
	}
	if len(res.Dimensions) > 0 {
		res.Score = sum / float64(len(res.Dimensions))
		if signals.edits == 0 {
			res.Badge = "unknown"
		} else if res.Score >= 0.75 {
			res.Badge = "good"
		} else if res.Score >= 0.45 {
			res.Badge = "ok"
		} else {
			res.Badge = "poor"
		}
	}
	return persistResult(paths, res)
}

// ConsiderableWork reports whether mid-session review thresholds are met.
func ConsiderableWork(spans []tracestore.Span, minEdits, minTools int) bool {
	s := collectActivitySignals(spans)
	if minEdits <= 0 {
		minEdits = 3
	}
	if minTools <= 0 {
		minTools = 10
	}
	return s.edits >= minEdits || s.toolCalls >= minTools
}

// PendingReviewInstruction is the short SessionStart inject (not the full brief).
func PendingReviewInstruction(vendor, sessionID, status string) string {
	kind := harness.NormalizeVendorKind(vendor)
	switch status {
	case "complete":
		return ""
	case "running":
		return fmt.Sprintf("## Previous %s session review\n\nReview for `%s` is running; skip `so apply-review` and continue the user's task.", kind, sessionID)
	default:
		return fmt.Sprintf("## Previous %s session review\n\nSession `%s` is pending review. Run `so review-brief %s`, produce the JSON with your model, then `so apply-review %s`. Then continue the user's task. Skip if status is complete or running.", kind, sessionID, sessionID, sessionID)
	}
}
