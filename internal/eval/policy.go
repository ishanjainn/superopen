package eval

import (
	"fmt"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
)

// SkipDecision explains why an automatic (non-force) eval should not run.
type SkipDecision struct {
	Skip   bool
	Reason string // ended_final | active_cooldown | ""
	Prior  Result
	Scope  string // complete | snapshot
}

// DecideSkip enforces eval frequency:
//   - ended chats: at most one final whole-chat eval (unless force)
//   - active chats: at most one snapshot per ActiveCooldownHours (default 6)
func DecideSkip(paths harness.Paths, cfg config.Config, meta session.Meta, force bool) SkipDecision {
	if force || meta.ID == "" {
		return SkipDecision{}
	}
	prior, ok := LatestResult(paths, meta.ID)
	if !ok {
		return SkipDecision{}
	}

	if meta.Status == session.StatusEnded {
		scope := "complete"
		if meta.EndedAt != nil && !prior.At.Before(*meta.EndedAt) {
			return SkipDecision{
				Skip:   true,
				Reason: "ended_final",
				Prior:  prior,
				Scope:  scope,
			}
		}
		return SkipDecision{Scope: scope}
	}

	hours := cfg.EvalsActiveCooldownHours()
	if hours <= 0 {
		hours = 6
	}
	cooldown := time.Duration(hours) * time.Hour
	age := time.Since(prior.At)
	if age < cooldown {
		return SkipDecision{
			Skip:   true,
			Reason: "active_cooldown",
			Prior:  prior,
			Scope:  "snapshot",
		}
	}
	return SkipDecision{Scope: "snapshot"}
}

// SkipMessage is a human-readable line for CLI / API reused responses.
func SkipMessage(id string, d SkipDecision) string {
	switch d.Reason {
	case "ended_final":
		return fmt.Sprintf(
			"Session %s already has a final whole-chat evaluation from %s",
			id, d.Prior.At.UTC().Format(time.RFC3339),
		)
	case "active_cooldown":
		return fmt.Sprintf(
			"Session %s was evaluated %s ago (active cooldown); pass --force to re-run",
			id, formatAge(time.Since(d.Prior.At)),
		)
	default:
		return ""
	}
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", m)
	}
	h := int(d.Hours())
	if h == 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", h)
}
