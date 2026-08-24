package memory

import "strings"

type ObserveResult struct {
	SessionID string `json:"session_id"`
	Inserted  int    `json:"inserted"`
	Provider  string `json:"provider,omitempty"`
	Skipped   string `json:"skipped,omitempty"`
}

// ObserveSession is a no-op. Typed knowledge comes from live capture JSON
// or post-session distill — not regex on prompts.
func ObserveSession(root, sessionID string) (ObserveResult, error) {
	return ObserveResult{SessionID: sessionID, Skipped: "disabled"}, nil
}

func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)
	i := strings.Index(s, "[")
	j := strings.LastIndex(s, "]")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

func canonicalObservationType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ObservationDecision, ObservationBugfix, ObservationFeature, ObservationRefactor, ObservationDiscovery, ObservationChange:
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ObservationChange
	}
}
