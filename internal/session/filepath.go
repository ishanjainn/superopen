package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/redact"
	"github.com/ishanjainn/superopen/internal/repofile"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func FilePathFromAttrs(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	tool := strings.TrimSpace(attrs["gen_ai.tool.name"])
	cwd := strings.TrimSpace(attrs["code.cwd"])
	for _, key := range []string{"coding_agent.file_path", "code.file.path"} {
		if p := repofile.Accept(attrs[key], tool, cwd); p != "" {
			return p
		}
	}
	for _, key := range []string{"gen_ai.tool.call.arguments", "gen_ai.tool.arguments", "coding_agent.tool.arguments"} {
		if p := repofile.Accept(repofile.PathFromJSON(attrs[key]), tool, cwd); p != "" {
			return p
		}
	}
	return ""
}

func keepUnredactedAttr(key string) bool {
	return redact.UnredactedAttr(key)
}

func footprintState(sp trace.Span) string {
	tool := ""
	if sp.Attributes != nil {
		tool = sp.Attributes["gen_ai.tool.name"]
	}
	return repofile.State(tool, sp.Name)
}

func addFootprintFile(foot map[string]*FootprintFile, path, state string) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return
	}
	if existing, ok := foot[path]; ok {
		existing.Count++
		if rank(state) > rank(existing.State) {
			existing.State = state
		}
		return
	}
	foot[path] = &FootprintFile{Path: path, State: state, Count: 1}
}

func countJSONLEvents(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		if _, ok := raw["_about"]; ok {
			continue
		}
		if name, _ := raw["name"].(string); name != "" {
			n++
		}
	}
	return n
}

func loadJSONLSpans(path string) []trace.Span {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []trace.Span
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var sp trace.Span
		if json.Unmarshal(sc.Bytes(), &sp) != nil || sp.Name == "" {
			continue
		}
		out = append(out, sp)
	}
	return out
}
