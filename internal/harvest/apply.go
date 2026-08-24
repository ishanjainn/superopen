package harvest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Apply(root string, id int64, force bool) (Proposal, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return Proposal{}, err
	}
	defer store.Close()
	p, err := store.GetProposal(id)
	if err != nil {
		return p, fmt.Errorf("proposal %d: %w", id, err)
	}
	if p.Status == StatusStale {
		return p, fmt.Errorf("proposal is stale")
	}
	if p.Status != StatusOpen {
		return p, fmt.Errorf("proposal %d is %s", id, p.Status)
	}
	if ProtectedPath(p.Target) {
		return p, fmt.Errorf("target is protected")
	}
	abs := filepath.Join(root, filepath.FromSlash(p.Target))
	live, err := os.ReadFile(abs)
	if err != nil && !os.IsNotExist(err) {
		return p, err
	}
	if os.IsNotExist(err) && p.Kind != KindCreate {
		return p, fmt.Errorf("target missing")
	}
	liveStr := string(live)
	if p.BaseHash != "" && !os.IsNotExist(err) {
		h, _ := HashFile(root, p.Target)
		if h != p.BaseHash {
			_ = store.SetStatus(id, StatusStale)
			p.Status = StatusStale
			return p, fmt.Errorf("stale: file changed since propose")
		}
	}
	if strings.TrimSpace(p.Diff) != "" && AlreadyContains(liveStr, p.Diff) {
		_ = store.SetStatus(id, StatusNoop)
		p.Status = StatusNoop
		return p, nil
	}
	patch, _ := ParseUnified(p.Diff)
	if !patch.Additive() && p.Kind != KindSimplify && !force {
		return p, fmt.Errorf("non-additive patch needs --force")
	}
	if p.Kind == KindSimplify && !force {
		return p, fmt.Errorf("simplify (delete/shrink) needs --force")
	}
	if p.Kind == KindCreate && !force {
		return p, fmt.Errorf("create needs --force")
	}
	next := liveStr
	if strings.TrimSpace(p.Diff) != "" {
		next, err = ApplyUnified(liveStr, p.Diff)
		if err != nil {
			return p, err
		}
	} else if strings.TrimSpace(p.Suggestion) != "" && p.Kind == KindCreate {
		next = p.Suggestion
		if !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
	} else {
		return p, fmt.Errorf("proposal has no diff")
	}
	if SentinelsChanged(liveStr, next) {
		return p, fmt.Errorf("apply would change a protected Superopen region")
	}
	if ProtectedContent(next) && ProtectedContent(liveStr) && next != liveStr {
		// Editing a protected skill/hook file is never allowed.
		if ProtectedPath(p.Target) || strings.Contains(liveStr, skillTripwire) {
			return p, fmt.Errorf("apply would change a protected Superopen file")
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return p, err
	}
	if err := os.WriteFile(abs, []byte(next), 0o644); err != nil {
		return p, err
	}
	if err := store.SetStatus(id, StatusApplied); err != nil {
		return p, err
	}
	p.Status = StatusApplied
	return p, nil
}

func Decline(root string, id int64) (Proposal, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return Proposal{}, err
	}
	defer store.Close()
	p, err := store.GetProposal(id)
	if err != nil {
		return p, err
	}
	if err := store.SetStatus(id, StatusDeclined); err != nil {
		return p, err
	}
	p.Status = StatusDeclined
	return p, nil
}
