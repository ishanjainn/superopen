package helpers

import "sync"

var (
	captureMessageContent     = true // default: capture content (matches Python SDK)
	captureMessageContentMu   sync.RWMutex
)

// SetCaptureMessageContent configures whether helpers that still consult this
// flag would record prompt/completion text. Coding hooks set false and manage
// capture in adapters instead.
func SetCaptureMessageContent(capture bool) {
	captureMessageContentMu.Lock()
	captureMessageContent = capture
	captureMessageContentMu.Unlock()
}

// GetCaptureMessageContent returns whether message content should be captured.
func GetCaptureMessageContent() bool {
	captureMessageContentMu.RLock()
	defer captureMessageContentMu.RUnlock()
	return captureMessageContent
}
