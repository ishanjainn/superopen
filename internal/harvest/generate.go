package harvest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/headless"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/session"
)

const generateTimeout = 45 * time.Second

// MaybeGenerate is called from session finalize. It detaches a harvest scan so
// SessionEnd stays fast. Under go test it no-ops (re-exec would fork the test binary).
func MaybeGenerate(root, sessionID string) GenerateResult {
	sessionID = strings.TrimSpace(sessionID)
	res := GenerateResult{SessionID: sessionID}
	if sessionID == "" {
		res.Skipped = "no-session"
		return res
	}
	if testing.Testing() {
		res.Skipped = "test"
		return res
	}
	if os.Getenv("SUPEROPEN_HARVEST_SYNC") == "1" {
		return Generate(root, sessionID)
	}
	cli.SpawnSO(root, "--root", root, "harvest", "scan", sessionID)
	res.Skipped = "detached"
	return res
}

// Generate runs skip gates then at most one bounded headless call.
func Generate(root, sessionID string) GenerateResult {
	res := GenerateResult{SessionID: strings.TrimSpace(sessionID)}
	if res.SessionID == "" {
		res.Skipped = "no-session"
		return res
	}
	store, err := OpenRoot(root)
	if err != nil {
		res.Skipped = err.Error()
		return res
	}
	defer store.Close()
	if store.SuccessfulRun(res.SessionID) || store.HasOpenForSession(res.SessionID) {
		res.Skipped = "already-harvested"
		_, _ = store.InsertRun(res.SessionID, StatusSkipped, "", res.Skipped)
		return res
	}
	if !sessionActive(root, res.SessionID) {
		res.Skipped = "empty"
		_, _ = store.InsertRun(res.SessionID, StatusSkipped, "", res.Skipped)
		return res
	}
	provider, ok := headless.Available()
	if !ok {
		res.Skipped = "no-auth"
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, "", res.Skipped)
		return res
	}
	prompt, err := buildPrompt(root, res.SessionID, store)
	if err != nil {
		res.Skipped = err.Error()
		_, _ = store.InsertRun(res.SessionID, StatusFailed, provider.Name, res.Skipped)
		return res
	}
	ctx, cancel := context.WithTimeout(context.Background(), generateTimeout)
	defer cancel()
	out, err := headless.Run(ctx, provider, prompt)
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		_, _ = store.InsertRun(res.SessionID, StatusPending, provider.Name, res.Skipped)
		return res
	}
	inputs := pickProposals(parseProposals(out))
	inserted := 0
	for _, in := range inputs {
		in.SessionID = res.SessionID
		in.Provider = provider.Name
		if _, err := Propose(root, in); err != nil {
			continue
		}
		inserted++
	}
	status := StatusProposed
	if inserted == 0 {
		status = StatusSkipped
		res.Skipped = "no-proposals"
	}
	_, _ = store.InsertRun(res.SessionID, status, provider.Name, res.Skipped)
	res.Provider = provider.Name
	res.Inserted = inserted
	return res
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

func buildPrompt(root, sessionID string, store *Store) (string, error) {
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
	b.WriteString("You propose playbook patches for Superopen harvest. Output JSON only: an array of objects with keys kind,target,title,reason,issue,suggestion,diff,evidence.\n")
	b.WriteString("kind is improve|create|simplify|principle. Prefer simplify/delete of unused or duplicate rules. Max 3 proposals.\n")
	b.WriteString("Each proposal needs a concrete reason and evidence (session/memory/graph ids that exist). Do not restate graph-first, Superopen sentinel, or memory contracts.\n")
	b.WriteString("diff must be a unified diff against the named target. Do not edit files. Do not dump full playbook bodies.\n")
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
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if i := strings.Index(raw, "```"); i >= 0 {
		rest := raw[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			raw = rest[:j]
		}
	}
	if i := strings.Index(raw, "["); i >= 0 {
		if j := strings.LastIndex(raw, "]"); j > i {
			raw = raw[i : j+1]
		}
	}
	var many []ProposeInput
	if err := json.Unmarshal([]byte(raw), &many); err == nil {
		return many
	}
	var one ProposeInput
	if err := json.Unmarshal([]byte(raw), &one); err == nil && one.Title != "" {
		return []ProposeInput{one}
	}
	return nil
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
