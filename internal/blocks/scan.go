package blocks

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Hit is one retyped shell command already on disk. The text is not uploaded.
type Hit struct {
	Command string
	Count   int
}

// Scan ranks shell commands that show up more than once in local session events.
func Scan(root string) ([]Hit, error) {
	dir := filepath.Join(root, ".so", "sessions")
	counts := map[string]int{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "events.jsonl" {
			return nil
		}
		return countCommands(path, counts)
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var hits []Hit
	for cmd, n := range counts {
		if n < 2 || mentionsSO(strings.ToLower(cmd)) {
			continue
		}
		hits = append(hits, Hit{Command: cmd, Count: n})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Count != hits[j].Count {
			return hits[i].Count > hits[j].Count
		}
		return hits[i].Command < hits[j].Command
	})
	return hits, nil
}

func countCommands(path string, counts map[string]int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 2<<20)
	for sc.Scan() {
		var row struct {
			Attributes map[string]string `json:"attributes"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil || row.Attributes == nil {
			continue
		}
		cmd := strings.Join(strings.Fields(row.Attributes["coding_agent.tool.command"]), " ")
		if cmd == "" || len(cmd) < 24 {
			continue
		}
		if !scriptLike(cmd) {
			continue
		}
		counts[cmd]++
	}
	return sc.Err()
}

func scriptLike(cmd string) bool {
	low := strings.ToLower(cmd)
	return strings.Contains(low, "python") || strings.Contains(low, "node ") ||
		strings.Contains(low, "jq ") || strings.Contains(low, "<<") ||
		strings.Contains(low, "perl ") || strings.Contains(cmd, "|")
}

// Init writes the staged index and does not touch AGENTS.md.
func Init(root string) (string, error) {
	path := ProposalPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(IndexText()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Apply writes the staged index into AGENTS.md. Init must have run first.
func Apply(root string) error {
	body, err := os.ReadFile(ProposalPath(root))
	if err != nil {
		return err
	}
	target := filepath.Join(root, "AGENTS.md")
	live, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	next := mergeIndex(string(live), string(body))
	return os.WriteFile(target, []byte(next), 0o644)
}

// Decline removes a staged index. It does not edit AGENTS.md.
func Decline(root string) error {
	err := os.Remove(ProposalPath(root))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func mergeIndex(live, proposal string) string {
	proposal = strings.TrimSpace(proposal) + "\n"
	start := strings.Index(live, markerBegin)
	end := strings.Index(live, markerEnd)
	if start >= 0 && end > start {
		end += len(markerEnd)
		for end < len(live) && (live[end] == '\n' || live[end] == '\r') {
			end++
		}
		return live[:start] + proposal + live[end:]
	}
	if strings.TrimSpace(live) == "" {
		return proposal
	}
	return strings.TrimRight(live, "\n") + "\n\n" + proposal
}
