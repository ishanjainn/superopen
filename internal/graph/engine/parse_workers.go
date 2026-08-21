package engine

import goruntime "runtime"

const maxParseWorkers = 32

func parseWorkerCount() int {
	workers := goruntime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > maxParseWorkers {
		return maxParseWorkers
	}
	return workers
}
