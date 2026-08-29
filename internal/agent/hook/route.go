package hook

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/sessionstate"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
)

const (
	routeCode   = "code"
	routeMemory = "memory"
	routeEmpty  = "empty"
)

// Generic personal/prior-work cues. No dataset name lists.
var memoryCue = regexp.MustCompile(`(?i)(?:last time|we decided|remember|what did we|who(?:'s|\s+is|\s+was|\s+were)\b|what\s+did\s+i\b|when\s+did\s+i\b|where\s+did\s+i\b|where\s+do\s+i\b|what\s+(?:is|was)\s+my\b|what\s+degree|i\s+graduate|graduated|\bmy\s+(?:degree|school|birthday|family|parents|job|hometown|commute|playlist|occupation)\b|how\s+long\b.{0,48}(?:commute|drive|trip|travel)|how\s+many\b.{0,40}(?:\bdo\s+i\b|\bdid\s+i\b|\bi\s+have\b|\bi've\b)|where\s+(?:did|do)\s+i\s+(?:buy|bought|get|got|shop)|what\s+did\s+i\s+(?:name|buy|get))`)

var codeCue = regexp.MustCompile(`(?i)(?:\.(?:go|py|ts|tsx|js|jsx|rs|java|rb|php|c|h|cc|cpp|cs|kt|swift)\b|\b(?:function|class|method|module|package|import|export|caller|callee|queryset|endpoint|handler|middleware|architecture|codebase|refactor|implement|stacktrace|traceback|orm|migration|migrations|admin|signal|signals|manage\.py|django-admin)\b|\b(?:src|pkg|internal|lib)/|\bwhere is\b|\bhow does\b|\bwho calls\b|\bcallers of\b)`)

var sourceExt = map[string]struct{}{
	".go": {}, ".py": {}, ".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {},
	".rs": {}, ".java": {}, ".rb": {}, ".php": {}, ".c": {}, ".h": {},
	".cc": {}, ".cpp": {}, ".cs": {}, ".kt": {}, ".swift": {}, ".scala": {},
}

var skipWalkDir = map[string]struct{}{
	".so": {}, ".git": {}, "node_modules": {}, "vendor": {}, "dist": {},
	"build": {}, "target": {}, ".venv": {}, "venv": {}, "__pycache__": {},
}

const sourceWalkCap = 4000

func classifyPrompt(prompt string) string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return ""
	}
	code := codeCue.MatchString(p)
	mem := memoryCue.MatchString(p)
	if code {
		return routeCode
	}
	if mem {
		return routeMemory
	}
	return ""
}

func workspaceHasSource(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	visited := 0
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return fs.SkipAll
		}
		name := d.Name()
		if d.IsDir() {
			if _, skip := skipWalkDir[strings.ToLower(name)]; skip && path != root {
				return fs.SkipDir
			}
			if strings.HasPrefix(name, ".") && path != root && name != "." {
				if name != ".so" && name != ".git" {
					return fs.SkipDir
				}
			}
			return nil
		}
		visited++
		if visited > sourceWalkCap {
			return fs.SkipAll
		}
		ext := strings.ToLower(filepath.Ext(name))
		if _, ok := sourceExt[ext]; ok {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func workspaceRoute(root string) string {
	if strings.TrimSpace(root) == "" {
		return routeEmpty
	}
	src := workspaceHasSource(root)
	n := memory.CountLiveMemories(root)
	if src {
		return routeCode
	}
	if n > 0 {
		return routeMemory
	}
	return routeEmpty
}

func rememberWorkspaceRoute(payload []byte, vendor, route string) {
	sessionID := steerSessionID(payload)
	if sessionID == "" || route == "" {
		return
	}
	state := sessionstate.Load(sessionID, vendor)
	state.WorkspaceRoute = route
	sessionstate.Save(sessionID, vendor, state)
}

func rememberPromptKind(payload []byte, vendor, kind string) {
	sessionID := steerSessionID(payload)
	if sessionID == "" || kind == "" {
		return
	}
	state := sessionstate.Load(sessionID, vendor)
	state.PromptKind = kind
	sessionstate.Save(sessionID, vendor, state)
}

func promptRoute(payload []byte, vendor string) string {
	sessionID := steerSessionID(payload)
	if sessionID != "" {
		state := sessionstate.Load(sessionID, vendor)
		if state.PromptKind == routeMemory || state.PromptKind == routeCode {
			return state.PromptKind
		}
		if state.WorkspaceRoute == routeMemory || state.WorkspaceRoute == routeCode || state.WorkspaceRoute == routeEmpty {
			return state.WorkspaceRoute
		}
	}
	return workspaceRoute(repoRoot(payload))
}

func repoRoot(payload []byte) string {
	start := strings.TrimSpace(peekContext(payload).CWD)
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		start = wd
	}
	root, err := paths.FindRoot(start)
	if err != nil || root == "" {
		return ""
	}
	if !paths.Managed(root) {
		return ""
	}
	return root
}

func bashCommandFromPayload(payload []byte) string {
	var probe map[string]any
	if json.Unmarshal(payload, &probe) != nil {
		return ""
	}
	input, _ := probe["tool_input"].(map[string]any)
	if input == nil {
		input, _ = probe["toolInput"].(map[string]any)
	}
	if input == nil {
		input, _ = probe["args"].(map[string]any)
	}
	if input == nil {
		input = probe
	}
	if cmd, ok := input["command"].(string); ok {
		return strings.TrimSpace(cmd)
	}
	return ""
}

var soVerb = map[string]struct{}{
	"graph": {}, "memory": {}, "sessions": {}, "init": {}, "harvest": {},
	"status": {}, "gc": {}, "install": {}, "uninstall": {}, "dev": {},
	"projects": {},
}

func commandLooksLikeSo(cmd string) bool {
	fields := strings.Fields(cmd)
	for i, f := range fields {
		base := strings.ToLower(filepath.Base(strings.Trim(f, `"'`)))
		if !paths.IsSoBinary(base) {
			continue
		}
		if i+1 < len(fields) {
			verb := strings.ToLower(strings.Trim(fields[i+1], `"'`))
			if _, ok := soVerb[verb]; ok {
				return true
			}
		}
		return true
	}
	return false
}

func isBashTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bash", "shell":
		return true
	default:
		return false
	}
}
