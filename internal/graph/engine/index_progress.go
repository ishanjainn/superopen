package engine

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	progressMu     sync.Mutex
	progressWriter = os.Stderr
)

func reportIndexProgress(format string, args ...any) {
	progressMu.Lock()
	defer progressMu.Unlock()
	fmt.Fprintf(progressWriter, format+"\n", args...)
	_ = progressWriter.Sync()
}

func indexElapsed(started time.Time) string {
	return time.Since(started).Truncate(time.Millisecond).String()
}
