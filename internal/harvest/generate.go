package harvest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/headless"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/scope"
	"github.com/ishanjainn/superopen/internal/session"
)

const generateTimeout = 45 * time.Second

// MaybeGenerate is called from session finalize. Cursor/Copilot/Gemini mark
// pending for the next live SessionStart. One-shot vendors spawn their own CLI.
func MaybeGenerate(root, sessionID string) GenerateResult {
	return maybeGenerate(root, sessionID, false)
}

// MaybeGenerateResumed evaluates new work added after a completed session was
// reopened. Earlier runs and proposals remain available for review.
func MaybeGenerateResumed(root, sessionID string) GenerateResult {
	return maybeGenerate(root, sessionID, true)
}

func maybeGenerate(root, sessionID string, resumed bool) GenerateResult {
	sessionID = strings.TrimSpace(sessionID)
	res := GenerateResult{SessionID: sessionID}
	if sessionID == "" {
		res.Skipped = "no-session"
		return res
	}
	skipped, pending, spawn := headless.SessionEndPolicy(root, sessionID)
	store, storeErr := OpenRoot(root)
	if storeErr == nil && !resumed && store.HasAttempt(sessionID) {
		_ = store.Close()
		res.Skipped = "already-queued"
		res.Pending = pending
		return res
	}
	if !spawn {
		res.Skipped = skipped
		res.Pending = pending
		if pending && storeErr == nil {
			_, _ = store.InsertRun(sessionID, StatusPending, "", skipped)
		}
		if storeErr == nil {
			_ = store.Close()
		}
		return res
	}
	if storeErr == nil {
		_ = store.Close()
	}
	if testing.Testing() {
		res.Skipped = "test"
		return res
	}
	if resumed || os.Getenv("SUPEROPEN_HARVEST_SYNC") == "1" {
		return generate(root, sessionID, resumed)
	}
	if store, err := OpenRoot(root); err == nil {
		_, _ = store.InsertRun(sessionID, StatusPending, "", "detached")
		_ = store.Close()
	}
	if strings.TrimSpace(os.Getenv("SUPEROPEN_TENANT")) == "" {
		_ = os.Setenv("SUPEROPEN_TENANT", scope.DefaultTenant)
	}
	cli.SpawnSO(root, "--root", root, "harvest", "scan", sessionID)
	res.Skipped = "detached"
	res.Pending = true
	return res
}

// Generate runs skip gates then at most one bounded call on the session's own CLI.
func Generate(root, sessionID string) GenerateResult {
	return generate(root, sessionID, false)
}

func generate(root, sessionID string, resumed bool) GenerateResult {
	res := GenerateResult{SessionID: strings.TrimSpace(sessionID)}
	if res.SessionID == "" {
		res.Skipped = "no-session"
		return res
	}
	if reason, ok := headless.SkipWorker(root, res.SessionID); ok {
		res.Skipped = reason
		return res
	}
	store, err := OpenRoot(root)
	if err != nil {
		res.Skipped = err.Error()
		return res
	}
	defer store.Close()
	if !resumed && (store.SuccessfulRun(res.SessionID) || store.HasOpenForSession(res.SessionID)) {
		res.Skipped = "already-harvested"
		_ = store.ResolvePending(res.SessionID, StatusSkipped, res.Skipped)
		return res
	}
	if !sessionActive(root, res.SessionID) {
		res.Skipped = "empty"
		_ = store.ResolvePending(res.SessionID, StatusSkipped, res.Skipped)
		return res
	}
	unlock, lockErr := headless.TryLock(headless.LockPath(root, "harvest"))
	if lockErr != nil {
		res.Skipped = "busy"
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, "", res.Skipped)
		return res
	}
	defer unlock()

	vendor := ""
	if meta, err := headless.LoadMeta(root, res.SessionID); err == nil {
		vendor = meta.Vendor
	}
	provider, ok := headless.AvailableLive(vendor)
	if !ok {
		res.Skipped = "no-auth"
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, "", res.Skipped)
		return res
	}
	if store.ProviderFailCount(res.SessionID, provider.Name) >= headless.MaxProviderFails() {
		res.Skipped = "provider-unhealthy"
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, provider.Name, res.Skipped)
		return res
	}
	prompt, err := buildPrompt(root, res.SessionID, store, false)
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, provider.Name, res.Skipped)
		return res
	}
	ctx, cancel := context.WithTimeout(context.Background(), generateTimeout)
	defer cancel()
	out, err := headless.Run(ctx, provider, prompt)
	if !resumed && store.HasOpenForSession(res.SessionID) {
		_ = store.ResolvePending(res.SessionID, StatusProposed, "")
		res.Provider = provider.Name
		return res
	}
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, provider.Name, res.Skipped)
		return res
	}
	inputs := pickProposals(parseProposals(out))
	if len(inputs) == 0 && !emptyProposalOutput(out) {
		res.Skipped = "invalid-proposals"
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, provider.Name, res.Skipped)
		return res
	}
	inserted := 0
	var proposeErr error
	for _, in := range inputs {
		in.SessionID = res.SessionID
		in.Provider = provider.Name
		if _, err := Propose(root, in); err != nil {
			proposeErr = err
			continue
		}
		inserted++
	}
	status := StatusProposed
	if inserted == 0 {
		if proposeErr != nil {
			res.Skipped = "invalid-proposal: " + proposeErr.Error()
			res.Pending = true
			_, _ = store.InsertRun(res.SessionID, StatusPending, provider.Name, res.Skipped)
			return res
		}
		status = StatusSkipped
		res.Skipped = "no-proposals"
	}
	_ = store.ResolvePending(res.SessionID, status, res.Skipped)
	res.Provider = provider.Name
	res.Inserted = inserted
	return res
}

// Brief gives a live agent the session evidence and proposal instructions.
func Brief(root, sessionID string) (string, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer store.Close()
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = store.PendingSession()
	}
	if sessionID == "" {
		return "", fmt.Errorf("no pending harvest session")
	}
	return buildPrompt(root, sessionID, store, true)
}

// Skip closes a pending harvest with nothing to propose.
func Skip(root, sessionID, reason string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id required")
	}
	store, err := OpenRoot(root)
	if err != nil {
		return err
	}
	defer store.Close()
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "live-skip"
	}
	return store.ResolvePending(sessionID, StatusSkipped, reason)
}

func pickProposals(in []ProposeInput) []ProposeInput {
	sort.SliceStable(in, func(i, j int) bool {
		return kindRank(in[i].Kind) < kindRank(in[j].Kind)
	})
	if len(in) > MaxProposals {
		in = in[:MaxProposals]
	}
	return in
}

func kindRank(k string) int {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case KindSimplify:
		return 0
	case KindImprove:
		return 1
	case KindPrinciple:
		return 2
	default:
		return 3
	}
}

func sessionActive(root, id string) bool {
	return session.HasActivity(root, id)
}

func buildPrompt(root, sessionID string, store *Store, liveAgent bool) (string, error) {
	files, _ := Inventory(root)
	var inv []string
	open, _ := store.List(StatusOpen)
	for _, f := range files {
		flag := ""
		if f.Protected {
			flag = " protected"
		}
		inv = append(inv, fmt.Sprintf("- %s %s %dB%s", f.Path, shortHash(f.Hash), f.Bytes, flag))
		if len(inv) >= 40 {
			break
		}
	}
	var openTitles []string
	for _, p := range open {
		openTitles = append(openTitles, p.Target+": "+p.Title)
		if len(openTitles) >= 12 {
			break
		}
	}
	digest := session.Digest(root, sessionID)
	var b strings.Builder
	b.WriteString("You propose playbook patches for Superopen harvest. Output JSON only: an array of objects with keys kind,target,title,reason,issue,suggestion,diff,evidence,memory_title,memory_text.\n")
	b.WriteString("kind,target,title,reason,issue,suggestion,diff,memory_title,memory_text must be JSON strings; evidence must be an array of objects.\n")
	if liveAgent {
		b.WriteString("The current user message is a note for this turn (memory capture). It is not evidence for a playbook diff on this session. If this session has nothing to change, run harvest skip. Pipe the JSON array to so harvest propose on stdin. Do not write a file. Do not shell-redirect into a path.\n")
	} else {
		b.WriteString("Return the JSON array as your final answer. Do not call so harvest propose or skip, and do not write harvest or memory state. Before writing a diff, read the named playbook file in the repository root below and match its current lines exactly. Never use a similarly named file from another repository. Return [] if there is nothing to propose.\n")
	}
	b.WriteString("Repository root: ")
	b.WriteString(root)
	b.WriteString("\n")
	b.WriteString("kind is improve|create|simplify|principle|memory. Prefer simplify/delete of unused or duplicate rules. Max 3 proposals.\n")
	b.WriteString("Each proposal needs a concrete reason and evidence as JSON objects, for example: \"evidence\":[{\"kind\":\"session\",\"id\":\"SESSION_ID\"}]. Use session, memory, or graph ids that exist; never use evidence strings. Do not restate graph-first, Superopen sentinel, or memory contracts.\n")
	b.WriteString("diff must be a unified diff against the named target. Do not edit files. Do not dump full playbook bodies.\n")
	b.WriteString("If this session contains a correction or debugging pattern future agents should reuse, put it in memory_title and memory_text on one proposal. That text is stored only when the proposal is applied. Use kind memory and omit target when there is no playbook patch.\n")
	b.WriteString("Session: ")
	b.WriteString(sessionID)
	b.WriteString("\n")
	b.WriteString(digest)
	b.WriteString("\nPlaybook inventory (path hash size):\n")
	if len(inv) == 0 {
		b.WriteString("(none)\n")
	} else {
		b.WriteString(strings.Join(inv, "\n"))
		b.WriteString("\n")
	}
	if len(openTitles) > 0 {
		b.WriteString("OPEN proposals (do not duplicate):\n")
		b.WriteString(strings.Join(openTitles, "\n"))
		b.WriteString("\n")
	}
	return b.String(), nil
}

func parseProposals(raw string) []ProposeInput {
	raw = proposalPayload(raw)
	if raw == "" {
		return nil
	}
	var many []ProposeInput
	if err := json.Unmarshal([]byte(raw), &many); err == nil {
		return many
	}
	var one ProposeInput
	if json.Unmarshal([]byte(raw), &one) == nil && one.Title != "" {
		return []ProposeInput{one}
	}
	// One-shot agents sometimes render evidence ids as strings despite the
	// requested object shape. Preserve recognizable ids instead of dropping
	// an otherwise usable proposal.
	var records []map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &records) != nil {
		var record map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &record) != nil {
			return nil
		}
		records = []map[string]json.RawMessage{record}
	}
	var proposals []ProposeInput
	for _, record := range records {
		var reason string
		if json.Unmarshal(record["reason"], &reason) != nil {
			var fields map[string]string
			if json.Unmarshal(record["reason"], &fields) == nil {
				keys := make([]string, 0, len(fields))
				for key := range fields {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				var parts []string
				for _, key := range keys {
					if value := strings.TrimSpace(fields[key]); value != "" {
						parts = append(parts, value)
					}
				}
				record["reason"], _ = json.Marshal(strings.Join(parts, " "))
			}
		}
		var items []json.RawMessage
		if json.Unmarshal(record["evidence"], &items) != nil {
			continue
		}
		var evidence []Evidence
		for _, item := range items {
			var e Evidence
			if json.Unmarshal(item, &e) == nil && e.Kind != "" {
				evidence = append(evidence, e)
				continue
			}
			var label string
			if json.Unmarshal(item, &label) != nil {
				continue
			}
			kind, id, ok := strings.Cut(label, ":")
			if ok && id != "" && (kind == "session" || kind == "memory" || kind == "graph") {
				evidence = append(evidence, Evidence{Kind: kind, ID: id})
			}
		}
		if len(evidence) == 0 {
			continue
		}
		record["evidence"], _ = json.Marshal(evidence)
		encoded, _ := json.Marshal(record)
		var in ProposeInput
		if json.Unmarshal(encoded, &in) == nil && in.Title != "" {
			proposals = append(proposals, in)
		}
	}
	return proposals
}

func emptyProposalOutput(raw string) bool {
	return proposalPayload(raw) == "[]"
}

var terminalControl = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")

func proposalPayload(raw string) string {
	raw = strings.TrimSpace(terminalControl.ReplaceAllString(raw, ""))
	if i := strings.Index(raw, "```"); i >= 0 {
		rest := raw[i+3:]
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, "json"), "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			raw = rest[:j]
		}
	}
	arrayStart := strings.Index(raw, "[")
	objectStart := strings.Index(raw, "{")
	if objectStart >= 0 && (arrayStart < 0 || objectStart < arrayStart) {
		if end := strings.LastIndex(raw, "}"); end > objectStart {
			raw = raw[objectStart : end+1]
		}
	} else if arrayStart >= 0 {
		if end := strings.LastIndex(raw, "]"); end > arrayStart {
			raw = raw[arrayStart : end+1]
		}
	}
	return strings.TrimSpace(raw)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func Review(root string) (string, []Proposal, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return "", nil, err
	}
	defer store.Close()
	items, err := store.List(StatusOpen)
	if err != nil {
		return "", nil, err
	}
	if len(items) == 0 {
		return "0 open harvest proposals", items, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d open harvest proposals\n", len(items))
	for _, p := range items {
		fmt.Fprintf(&b, "#%d %s %s\n  %s\n  %s\n", p.ID, p.Kind, p.Target, p.Title, truncate(p.Reason, 160))
	}
	b.WriteString("Apply with so harvest apply <id> (simplify/create need --force). Decline with so harvest decline <id>.")
	return strings.TrimSpace(b.String()), items, nil
}
