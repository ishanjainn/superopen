package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/superopen/so/internal/llm"
)

// DisplayName returns the best human label for a session.
// Prefer AI/vendor title, then first-prompt preview, then id.
func DisplayName(m Meta) string {
	if t := strings.TrimSpace(m.Title); t != "" {
		return humanizePromptPreview(t)
	}
	if t := strings.TrimSpace(m.PromptPreview); t != "" {
		return humanizePromptPreview(t)
	}
	return m.ID
}

// EnsureTitle fills meta.Title when empty: vendor AI name first, then optional LLM.
func EnsureTitle(meta *Meta, client *llm.Client) {
	if meta == nil || strings.TrimSpace(meta.Title) != "" {
		return
	}
	if t := lookupVendorTitle(meta.ID, meta.Vendor); t != "" {
		meta.Title = t
		return
	}
	prompt := strings.TrimSpace(meta.PromptPreview)
	if prompt == "" || client == nil || !client.Available() {
		return
	}
	if t := generateTitle(client, prompt); t != "" {
		meta.Title = t
	}
}

type titleLookup struct {
	match func(vendor string) bool
	fn    func(sessionID string) string
}

func vendorTitleLookups() []titleLookup {
	return []titleLookup{
		{func(v string) bool { return strings.Contains(v, "claude") }, lookupClaudeAITitle},
		{func(v string) bool { return strings.Contains(v, "codex") }, lookupCodexThreadName},
		{func(v string) bool { return strings.Contains(v, "pi") }, lookupPiSessionName},
		{func(v string) bool {
			return strings.Contains(v, "opencode") || strings.Contains(v, "open-code")
		}, lookupOpenCodeTitle},
	}
}

func lookupVendorTitle(id, vendor string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	v := strings.ToLower(vendor)
	lookups := vendorTitleLookups()
	// Prefer the store that matches the session vendor.
	for _, l := range lookups {
		if l.match(v) {
			if t := l.fn(id); t != "" {
				return t
			}
		}
	}
	// Cursor / unknown / miss: try every vendor store.
	for _, l := range lookups {
		if t := l.fn(id); t != "" {
			return t
		}
	}
	return ""
}

func lookupClaudeAITitle(sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	needle := sessionID + ".jsonl"
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), needle)
		if t := readClaudeAITitle(path); t != "" {
			return t
		}
	}
	return ""
}

func readClaudeAITitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var title string
	for sc.Scan() {
		var row struct {
			Type    string `json:"type"`
			AITitle string `json:"aiTitle"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if row.Type == "ai-title" && strings.TrimSpace(row.AITitle) != "" {
			title = strings.TrimSpace(row.AITitle)
		}
	}
	return title
}

var codexTitleCache struct {
	mu      sync.Mutex
	modTime time.Time
	size    int64
	titles  map[string]string
}

func lookupCodexThreadName(sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".codex", "session_index.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	codexTitleCache.mu.Lock()
	defer codexTitleCache.mu.Unlock()
	if codexTitleCache.titles == nil ||
		codexTitleCache.size != info.Size() ||
		!codexTitleCache.modTime.Equal(info.ModTime()) {
		codexTitleCache.titles = loadCodexTitles(path)
		codexTitleCache.size = info.Size()
		codexTitleCache.modTime = info.ModTime()
	}
	return codexTitleCache.titles[sessionID]
}

func loadCodexTitles(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var row struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if row.ID != "" && strings.TrimSpace(row.ThreadName) != "" {
			out[row.ID] = strings.TrimSpace(row.ThreadName)
		}
	}
	return out
}

func piSessionsRoot() string {
	if v := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); v != "" {
		return filepath.Join(v, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

// lookupPiSessionName reads Pi's session_info.name.
// Returns empty when no session_info name exists so callers can fall back to prompt preview.
func lookupPiSessionName(sessionID string) string {
	root := piSessionsRoot()
	if root == "" {
		return ""
	}
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if found != "" || err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		// Filenames are typically "<timestamp>_<sessionId>.jsonl".
		if !strings.Contains(info.Name(), sessionID) {
			return nil
		}
		if t, ok := readPiSessionInfoName(path); ok {
			found = t
			return errStopWalk
		}
		return nil
	})
	return found
}

var errStopWalk = fmt.Errorf("stop walk")

// readPiSessionInfoName returns (name, true) when a session_info entry exists.
// An explicit empty name clears the title (ok=true, name="").
func readPiSessionInfoName(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var (
		title string
		saw   bool
	)
	for sc.Scan() {
		var row struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if row.Type != "session_info" {
			continue
		}
		saw = true
		title = strings.TrimSpace(row.Name)
		// Keep scanning: latest session_info wins.
	}
	return title, saw
}

func opencodeDBPath() string {
	if v := strings.TrimSpace(os.Getenv("OPENCODE_DB")); v != "" {
		return v
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "opencode", "opencode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

func opencodeSessionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".opencode", "sessions")
}

// lookupOpenCodeTitle reads info.title from exported JSON or the opencode.db session row.
func lookupOpenCodeTitle(sessionID string) string {
	if t := readOpenCodeJSONTitle(sessionID); t != "" {
		return t
	}
	return readOpenCodeSQLiteTitle(sessionID)
}

func readOpenCodeJSONTitle(sessionID string) string {
	dir := opencodeSessionsDir()
	if dir == "" {
		return ""
	}
	for _, name := range []string{sessionID + ".json", sessionID} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc struct {
			Info struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"info"`
		}
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		title := strings.TrimSpace(doc.Info.Title)
		if title == "" || title == sessionID || title == doc.Info.ID {
			continue // stub export used id as title
		}
		return title
	}
	return ""
}

var openCodeTitleCache struct {
	mu      sync.Mutex
	dbPath  string
	modTime time.Time
	size    int64
	titles  map[string]string
}

func readOpenCodeSQLiteTitle(sessionID string) string {
	db := opencodeDBPath()
	if db == "" {
		return ""
	}
	info, err := os.Stat(db)
	if err != nil {
		return ""
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return ""
	}
	openCodeTitleCache.mu.Lock()
	defer openCodeTitleCache.mu.Unlock()
	if openCodeTitleCache.titles == nil ||
		openCodeTitleCache.dbPath != db ||
		openCodeTitleCache.size != info.Size() ||
		!openCodeTitleCache.modTime.Equal(info.ModTime()) {
		openCodeTitleCache.titles = loadOpenCodeTitles(db)
		openCodeTitleCache.dbPath = db
		openCodeTitleCache.size = info.Size()
		openCodeTitleCache.modTime = info.ModTime()
	}
	return openCodeTitleCache.titles[sessionID]
}

func loadOpenCodeTitles(db string) map[string]string {
	out := map[string]string{}
	// Best-effort: no CGO sqlite driver; use sqlite3 CLI like the port adapter.
	cmd := exec.Command("sqlite3", "-json", db, "SELECT id, title FROM session;")
	data, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("sqlite3", "-json", db, "SELECT id, title FROM sessions;")
		data, err = cmd.Output()
		if err != nil {
			return out
		}
	}
	var rows []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if json.Unmarshal(data, &rows) != nil {
		return out
	}
	for _, row := range rows {
		title := strings.TrimSpace(row.Title)
		if row.ID == "" || title == "" || title == row.ID {
			continue
		}
		out[row.ID] = title
	}
	return out
}

func generateTitle(client *llm.Client, prompt string) string {
	system := `You name coding-agent chat sessions.
Return ONLY a short title (3-8 words), Title Case, no quotes, no trailing punctuation.
Mirror Claude Code / Codex naming: imperative and specific (e.g. "Add health check endpoint", "Remove GCP secret manager dependency").
Do not repeat the whole prompt.`
	user := "First user message:\n" + truncate(prompt, 400)
	out, err := client.CompleteOpts(system, user, llm.Options{MaxTokens: 24, Timeout: 8 * time.Second})
	if err != nil {
		return ""
	}
	t := strings.TrimSpace(out)
	t = strings.Trim(t, `"'`)
	t = strings.Split(t, "\n")[0]
	t = strings.TrimSpace(t)
	if len(t) > 80 {
		t = t[:80]
	}
	// Reject if model echoed the whole prompt.
	if len(t) < 3 || strings.EqualFold(t, prompt) {
		return ""
	}
	return t
}
