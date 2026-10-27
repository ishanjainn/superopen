package engine

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	progressMu     sync.Mutex
	progressWriter = os.Stderr
	progressOnce   sync.Once
	progressOn     bool
)

// progressEnabled shows index progress on a terminal only. Agent shells
// (Cursor, Claude Code) hand stderr to the model, which then reads
// "parse 0/5957" ahead of the answer. SUPEROPEN_PROGRESS=1 forces it on.
func progressEnabled() bool {
	progressOnce.Do(func() {
		switch strings.ToLower(strings.TrimSpace(os.Getenv("SUPEROPEN_PROGRESS"))) {
		case "1", "true", "yes", "on":
			progressOn = true
			return
		case "0", "false", "no", "off":
			return
		}
		st, err := progressWriter.Stat()
		progressOn = err == nil && st.Mode()&os.ModeCharDevice != 0
	})
	return progressOn
}

func reportIndexProgress(format string, args ...any) {
	if !progressEnabled() {
		return
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	fmt.Fprintf(progressWriter, format+"\n", args...)
	_ = progressWriter.Sync()
}

func indexElapsed(started time.Time) string {
	return time.Since(started).Truncate(time.Millisecond).String()
}
