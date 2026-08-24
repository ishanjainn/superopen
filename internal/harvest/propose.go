package harvest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Propose(root string, in ProposeInput) (Proposal, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	in.Title = strings.TrimSpace(in.Title)
	in.Target = strings.TrimSpace(in.Target)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Reason == "" {
		return Proposal{}, fmt.Errorf("reason is required")
	}
	if in.Title == "" {
		return Proposal{}, fmt.Errorf("title is required")
	}
	if in.Target == "" {
		return Proposal{}, fmt.Errorf("target is required")
	}
	if in.Kind == "" {
		in.Kind = KindImprove
	}
	switch in.Kind {
	case KindImprove, KindCreate, KindSimplify, KindPrinciple:
	default:
		return Proposal{}, fmt.Errorf("unknown kind %q", in.Kind)
	}
	if strings.TrimSpace(in.SessionID) != "" && len(in.Evidence) == 0 {
		return Proposal{}, fmt.Errorf("evidence is required when session_id is set")
	}
	if ProtectedPath(in.Target) {
		return Proposal{}, fmt.Errorf("target is protected")
	}
	store, err := OpenRoot(root)
	if err != nil {
		return Proposal{}, err
	}
	defer store.Close()
	if store.FindOpenDup(in.Target, in.Title) {
		return Proposal{}, fmt.Errorf("duplicate open proposal")
	}
	abs := filepath.Join(root, filepath.FromSlash(in.Target))
	live, liveErr := os.ReadFile(abs)
	liveStr := string(live)
	if liveErr == nil && strings.TrimSpace(in.Diff) != "" && AlreadyContains(liveStr, in.Diff) {
		plus, minus := DiffStats(in.Diff)
		p := Proposal{
			SessionID:  in.SessionID,
			Status:     StatusNoop,
			Kind:       in.Kind,
			Target:     in.Target,
			Title:      in.Title,
			Reason:     in.Reason,
			Issue:      in.Issue,
			Suggestion: in.Suggestion,
			Diff:       in.Diff,
			Evidence:   in.Evidence,
			Plus:       plus,
			Minus:      minus,
		}
		if h, err := HashFile(root, in.Target); err == nil {
			p.BaseHash = h
		}
		return store.InsertProposal(p)
	}
	plus, minus := DiffStats(in.Diff)
	p := Proposal{
		SessionID:  in.SessionID,
		Status:     StatusOpen,
		Kind:       in.Kind,
		Target:     in.Target,
		Title:      in.Title,
		Reason:     in.Reason,
		Issue:      in.Issue,
		Suggestion: in.Suggestion,
		Diff:       in.Diff,
		Evidence:   in.Evidence,
		Plus:       plus,
		Minus:      minus,
	}
	if liveErr == nil {
		if h, err := HashFile(root, in.Target); err == nil {
			p.BaseHash = h
		}
		if st, err := os.Stat(abs); err == nil {
			p.BaseMtime = st.ModTime().UTC().Format(time.RFC3339)
		}
	}
	out, err := store.InsertProposal(p)
	if err != nil {
		return out, err
	}
	if in.SessionID != "" && in.Provider != "" {
		_, _ = store.InsertRun(in.SessionID, StatusProposed, in.Provider, "")
	}
	return out, nil
}

func ProposeJSON(root string, raw []byte) ([]Proposal, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty propose payload")
	}
	var many []ProposeInput
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &many); err != nil {
			return nil, err
		}
	} else {
		var one ProposeInput
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, err
		}
		many = []ProposeInput{one}
	}
	var out []Proposal
	for _, in := range many {
		p, err := Propose(root, in)
		if err != nil {
			return out, err
		}
		out = append(out, p)
	}
	return out, nil
}
