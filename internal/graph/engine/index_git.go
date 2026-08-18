package engine

import (
	"context"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Superopen pass_githistory.c thresholds. Commits touching more than
// gitMaxFilesPerCommit files are refactor/merge noise, a pair needs at least
// gitMinCoChanges commits together, and the coupling score is the share of the
// rarer file's commits that are shared.
const (
	gitMinCoChanges       = 3
	gitMaxFilesPerCommit  = 20
	gitMinCouplingScore   = 0.3
	gitMaxCouplings       = 8192
	gitMaxFileTemporal    = 16384
	gitHistoryMaxCommits  = 10000
	gitHistorySinceWindow = "1 year ago"
)

type gitCommit struct {
	files     []string
	timestamp int64
}

type gitCoupling struct {
	fileA, fileB  string
	coChangeCount int
	couplingScore float64
	lastCoChange  int64
}

type gitFileTemporal struct {
	path         string
	changeCount  int
	lastModified int64
}

func indexGitCochange(ctx context.Context, root, project string, graph *goGraph) {
	commits := readGitCommits(ctx, root)
	if len(commits) == 0 {
		return
	}
	couplings := computeChangeCoupling(commits)
	temporal := computeFileTemporal(commits)

	fileNodes := map[string]string{}
	for _, node := range graph.nodes {
		if node.Label == "File" {
			fileNodes[filepath.ToSlash(node.Location.File)] = node.QualifiedName
		}
	}
	for _, coupling := range couplings {
		left, okLeft := fileNodes[coupling.fileA]
		right, okRight := fileNodes[coupling.fileB]
		if !okLeft || !okRight || left == right {
			continue
		}
		graph.edges = append(graph.edges, pendingEdge{
			source: left, target: right, kind: "FILE_CHANGES_WITH",
			properties: api.Properties{
				"co_changes":     coupling.coChangeCount,
				"coupling_score": roundTwo(coupling.couplingScore),
				"last_co_change": coupling.lastCoChange,
			},
		})
	}
	applyFileTemporalProperties(temporal, fileNodes, graph)
	sortGraph(graph)
}

func readGitCommits(ctx context.Context, root string) []gitCommit {
	command := exec.CommandContext(ctx, "git", "-C", root, "log", "--name-only",
		"--pretty=format:COMMIT:%H:%ct", "--since="+gitHistorySinceWindow,
		"--max-count="+strconv.Itoa(gitHistoryMaxCommits))
	output, err := command.Output()
	if err != nil || len(output) == 0 {
		return nil
	}
	commits := make([]gitCommit, 0, 64)
	current := gitCommit{}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "COMMIT:"); ok {
			if len(current.files) > 0 {
				commits = append(commits, current)
			}
			current = gitCommit{}
			if index := strings.Index(rest, ":"); index >= 0 {
				current.timestamp, _ = strconv.ParseInt(rest[index+1:], 10, 64)
			}
			continue
		}
		if isTrackableGitFile(line) {
			current.files = append(current.files, line)
		}
	}
	if len(current.files) > 0 {
		commits = append(commits, current)
	}
	return commits
}

// isTrackableGitFile implements Superopen: generated, vendored, and
// binary paths are excluded from coupling analysis.
func isTrackableGitFile(path string) bool {
	if path == "" {
		return false
	}
	for _, prefix := range []string{".git/", "node_modules/", "vendor/", "__pycache__/", ".cache/"} {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	base := path
	if index := strings.LastIndex(base, "/"); index >= 0 {
		base = base[index+1:]
	}
	switch base {
	case "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "Cargo.lock",
		"poetry.lock", "composer.lock", "Gemfile.lock", "Pipfile.lock":
		return false
	}
	for _, suffix := range []string{".lock", ".sum", ".min.js", ".min.css", ".map",
		".wasm", ".png", ".jpg", ".gif", ".ico", ".svg"} {
		if strings.HasSuffix(path, suffix) {
			return false
		}
	}
	return true
}

func computeChangeCoupling(commits []gitCommit) []gitCoupling {
	fileCounts := map[string]int{}
	pairCounts := map[string]int{}
	pairTimestamps := map[string]int64{}
	for _, commit := range commits {
		if len(commit.files) > gitMaxFilesPerCommit {
			continue
		}
		for _, file := range commit.files {
			fileCounts[file]++
		}
		for i := 0; i < len(commit.files); i++ {
			for j := i + 1; j < len(commit.files); j++ {
				left, right := commit.files[i], commit.files[j]
				if left > right {
					left, right = right, left
				}
				key := left + "\x01" + right
				pairCounts[key]++
				if commit.timestamp > pairTimestamps[key] {
					pairTimestamps[key] = commit.timestamp
				}
			}
		}
	}
	keys := make([]string, 0, len(pairCounts))
	for key := range pairCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	couplings := make([]gitCoupling, 0, len(keys))
	for _, key := range keys {
		if len(couplings) >= gitMaxCouplings {
			break
		}
		coChanges := pairCounts[key]
		if coChanges < gitMinCoChanges {
			continue
		}
		separator := strings.Index(key, "\x01")
		if separator < 0 {
			continue
		}
		fileA, fileB := key[:separator], key[separator+1:]
		countA, okA := fileCounts[fileA]
		countB, okB := fileCounts[fileB]
		if !okA || !okB {
			continue
		}
		minTotal := countA
		if countB < minTotal {
			minTotal = countB
		}
		if minTotal == 0 {
			continue
		}
		score := float64(coChanges) / float64(minTotal)
		if score < gitMinCouplingScore {
			continue
		}
		couplings = append(couplings, gitCoupling{fileA: fileA, fileB: fileB,
			coChangeCount: coChanges, couplingScore: score, lastCoChange: pairTimestamps[key]})
	}
	return couplings
}

func computeFileTemporal(commits []gitCommit) []gitFileTemporal {
	index := map[string]int{}
	result := make([]gitFileTemporal, 0, gitMaxFileTemporal)
	for _, commit := range commits {
		if len(commit.files) > gitMaxFilesPerCommit {
			continue
		}
		for _, file := range commit.files {
			if position, ok := index[file]; ok {
				result[position].changeCount++
				if commit.timestamp > result[position].lastModified {
					result[position].lastModified = commit.timestamp
				}
				continue
			}
			if len(result) >= gitMaxFileTemporal {
				continue
			}
			index[file] = len(result)
			result = append(result, gitFileTemporal{path: file, changeCount: 1, lastModified: commit.timestamp})
		}
	}
	return result
}

func applyFileTemporalProperties(temporal []gitFileTemporal, fileNodes map[string]string, graph *goGraph) {
	byQN := map[string]*api.Node{}
	for index := range graph.nodes {
		if graph.nodes[index].Label == "File" {
			byQN[graph.nodes[index].QualifiedName] = &graph.nodes[index]
		}
	}
	for _, entry := range temporal {
		qn, ok := fileNodes[entry.path]
		if !ok {
			continue
		}
		node, ok := byQN[qn]
		if !ok {
			continue
		}
		if node.Properties == nil {
			node.Properties = api.Properties{}
		}
		node.Properties["extension"] = filepath.Ext(entry.path)
		node.Properties["last_modified"] = entry.lastModified
		node.Properties["change_count"] = entry.changeCount
	}
}
