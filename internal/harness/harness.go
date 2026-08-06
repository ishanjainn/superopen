package harness

import (
	"fmt"
	"os"
	"path/filepath"
)

const DirName = ".so"

// Paths holds absolute paths into a project's .so/ harness.
type Paths struct {
	Root            string
	Config          string
	AgentBrief      string
	GraphDir        string
	GraphJSON       string
	GraphReport     string
	KnowledgeDir    string // .so/knowledge (team feedforward docs)
	RulesDir        string // .so/rules
	SkillsDir       string
	GuardrailsDir   string
	GuardrailsFile  string
	EvalsDir        string
	EvalsConfig     string
	EvalsHistory    string
	TracesDir       string
	SessionsDir     string
	SessionsIndex   string
	Recommendations string
	PendingRecs     string
	RecsHistory     string
	VizDir          string
	Citymap         string
	MemoryDir       string
	Lessons         string
	LessonsJSONL    string
	MemoryActive    string // .so/memory/active-context.md
	AuditDir        string
	AuditEvents     string
}

// FindRoot walks up from start looking for .so/ or a .git directory (repo root).
func FindRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	dir := abs
	for {
		if info, err := os.Stat(filepath.Join(dir, DirName)); err == nil && info.IsDir() {
			return dir, nil
		}
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs, nil
		}
		dir = parent
	}
}

// Resolve returns harness paths for a repo root.
func Resolve(repoRoot string) Paths {
	root := filepath.Join(repoRoot, DirName)
	return Paths{
		Root:            root,
		Config:          filepath.Join(root, "config.yaml"),
		AgentBrief:      filepath.Join(root, "AGENT.md"),
		GraphDir:        filepath.Join(root, "graph"),
		GraphJSON:       filepath.Join(root, "graph", "graph.json"),
		GraphReport:     filepath.Join(root, "graph", "GRAPH_REPORT.md"),
		KnowledgeDir:    filepath.Join(root, "knowledge"),
		RulesDir:        filepath.Join(root, "rules"),
		SkillsDir:       filepath.Join(root, "skills"),
		GuardrailsDir:   filepath.Join(root, "guardrails"),
		GuardrailsFile:  filepath.Join(root, "guardrails", "guardrails.yaml"),
		EvalsDir:        filepath.Join(root, "evals"),
		EvalsConfig:     filepath.Join(root, "evals", "configs.yaml"),
		EvalsHistory:    filepath.Join(root, "evals", "history.json"),
		TracesDir:       filepath.Join(root, "traces"),
		SessionsDir:     filepath.Join(root, "sessions"),
		SessionsIndex:   filepath.Join(root, "sessions", "index.json"),
		Recommendations: filepath.Join(root, "recommendations"),
		PendingRecs:     filepath.Join(root, "recommendations", "pending.json"),
		RecsHistory:     filepath.Join(root, "recommendations", "history.json"),
		VizDir:          filepath.Join(root, "viz"),
		Citymap:         filepath.Join(root, "viz", "citymap.json"),
		MemoryDir:       filepath.Join(root, "memory"),
		Lessons:         filepath.Join(root, "memory", "lessons.md"),
		LessonsJSONL:    filepath.Join(root, "memory", "lessons.jsonl"),
		MemoryActive:    filepath.Join(root, "memory", "active-context.md"),
		AuditDir:        filepath.Join(root, "audit"),
		AuditEvents:     filepath.Join(root, "audit", "events.jsonl"),
	}
}

// Exists reports whether the harness has been initialized.
func (p Paths) Exists() bool {
	info, err := os.Stat(p.Root)
	return err == nil && info.IsDir()
}

// EnsureDirs creates the standard .so/ directory tree and migrates old names once.
func (p Paths) EnsureDirs() error {
	// One-shot migrations (no ongoing aliases in APIs).
	migrateDir(filepath.Join(p.Root, "docs"), p.KnowledgeDir)
	migrateDir(filepath.Join(p.Root, "context"), p.KnowledgeDir)
	migrateFile(filepath.Join(p.MemoryDir, "ACTIVE.md"), p.MemoryActive)

	dirs := []string{
		p.Root, p.GraphDir, p.KnowledgeDir, p.RulesDir, p.SkillsDir, p.GuardrailsDir,
		p.EvalsDir, p.TracesDir, p.SessionsDir, p.Recommendations,
		p.VizDir, p.MemoryDir, filepath.Join(p.MemoryDir, "history"), p.AuditDir,
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func migrateDir(from, to string) {
	info, err := os.Stat(from)
	if err != nil || !info.IsDir() {
		return
	}
	if _, err := os.Stat(to); err == nil {
		return
	}
	_ = os.Rename(from, to)
}

func migrateFile(from, to string) {
	if _, err := os.Stat(from); err != nil {
		return
	}
	if _, err := os.Stat(to); err == nil {
		_ = os.Remove(from)
		return
	}
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	_ = os.Rename(from, to)
}

// SessionDir returns the path for a session id.
func (p Paths) SessionDir(id string) string {
	return filepath.Join(p.SessionsDir, id)
}
