package engine

import (
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	ramBytesPerGB     = 1024 * 1024 * 1024
	reclaimEveryFiles = 64
	reclaimNap        = 10 * time.Millisecond
)

var memoryBudgetBytes atomic.Uint64
var parsedSinceReclaim atomic.Int64

func applyMemoryBudget() {
	total := systemRAMBytes()
	budget := resolveMemoryBudget(total, os.Getenv("SUPEROPEN_MEM_BUDGET_MB"))
	if budget == 0 {
		return
	}
	memoryBudgetBytes.Store(budget)
	debug.SetMemoryLimit(int64(budget))
}

func resolveMemoryBudget(totalRAM uint64, budgetMB string) uint64 {
	if trimmed := trimBudget(budgetMB); trimmed != "" {
		mb, err := strconv.ParseUint(trimmed, 10, 64)
		if err == nil && mb > 0 {
			return mb * 1024 * 1024
		}
	}
	if totalRAM == 0 {
		return 0
	}
	fraction := 0.5
	switch {
	case totalRAM <= 16*ramBytesPerGB:
		fraction = 0.25
	case totalRAM <= 32*ramBytesPerGB:
		fraction = 0.35
	}
	return uint64(float64(totalRAM) * fraction)
}

func trimBudget(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t') {
		end--
	}
	return value[start:end]
}

func reclaimAfterParse() {
	if parsedSinceReclaim.Add(1)%reclaimEveryFiles != 0 {
		return
	}
	reclaimIfNeeded()
}

func reclaimIfNeeded() {
	budget := memoryBudgetBytes.Load()
	if budget == 0 {
		runtime.GC()
		debug.FreeOSMemory()
		return
	}
	rss := processRSSBytes()
	if rss == 0 {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		rss = stats.Sys
	}
	if rss < budget {
		return
	}
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(reclaimNap)
}

func dropExtractedOccurrences(files []ParsedSyntaxFile) {
	for i := range files {
		files[i].Extraction.Usages = nil
		files[i].Extraction.Writes = nil
		files[i].Extraction.Bindings = nil
		files[i].Extraction.Calls = nil
		files[i].Extraction.Throws = nil
		files[i].Body = nil
	}
}
