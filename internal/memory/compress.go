package memory

// isCompressible reports whether observer/distill may summarize an episode.
// KindPrompt is write-once verbatim (default-deny compression for prompts).
// Pins are never compressed.
func isCompressible(ep Episode) bool {
	if ep.Pinned || ep.NeverDecay {
		return false
	}
	switch ep.Kind {
	case KindPrompt, KindWorking, KindTeaching, KindPin:
		return false
	case KindObservation, KindSession:
		return true
	default:
		return false
	}
}
