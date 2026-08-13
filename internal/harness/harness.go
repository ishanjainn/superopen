package harness

import (
	"fmt"
	"os"
	"path/filepath"
)

const DirName = ".so"

// Paths holds absolute paths into a project's Superopen layout.
// Guidance (AGENTS.md + vendor rules/skills) lives at the repo root.
// Runtime state lives under .so/.
type Paths struct {
	RepoRoot string

	Root        string
	Config      string
	AgentBrief  string
	GraphDir    string
	GraphJSON   string
	GraphReport string
	GraphCorpus string
	GraphHTML   string
	GraphState  string

	// Native developer guidance (not under .so/).
	AgentsMD  string // AGENTS.md
	RulesDir  string // discovered vendor rules dir
	SkillsDir string // discovered vendor skills dir

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
	MemoryDir       string
	Lessons         string
	LessonsJSONL    string
	MemoryActive    string
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
// RulesDir / SkillsDir follow existing vendor trees when present.
func Resolve(repoRoot string) Paths {
	root := filepath.Join(repoRoot, DirName)
	rulesDir, skillsDir := discoverNativeRoots(repoRoot)
	return Paths{
		RepoRoot:        repoRoot,
		Root:            root,
		Config:          filepath.Join(root, "config.yaml"),
		AgentBrief:      "",
		GraphDir:        filepath.Join(root, "graph"),
		GraphJSON:       filepath.Join(root, "graph", "graph.json"),
		GraphReport:     "",
		GraphCorpus:     filepath.Join(root, "graph", "corpus.json"),
		GraphHTML:       filepath.Join(root, "graph", "graph.html"),
		GraphState:      filepath.Join(root, "graph", "state.json"),
		AgentsMD:        filepath.Join(repoRoot, "AGENTS.md"),
		RulesDir:        rulesDir,
		SkillsDir:       skillsDir,
		GuardrailsDir:   root,
		GuardrailsFile:  filepath.Join(root, "guardrails.yaml"),
		EvalsDir:        root,
		EvalsConfig:     filepath.Join(root, "evals.yaml"),
		EvalsHistory:    filepath.Join(root, "sessions", "index.json"),
		TracesDir:       filepath.Join(root, "sessions"),
		SessionsDir:     filepath.Join(root, "sessions"),
		SessionsIndex:   filepath.Join(root, "sessions", "index.json"),
		Recommendations: filepath.Join(root, "sessions"),
		PendingRecs:     filepath.Join(root, "sessions", "index.json"),
		RecsHistory:     filepath.Join(root, "sessions", "index.json"),
		VizDir:          filepath.Join(root, "graph"),
		MemoryDir:       filepath.Join(root, "memory"),
		Lessons:         filepath.Join(root, "memory", "state.json"),
		LessonsJSONL:    filepath.Join(root, "memory", "state.json"),
		MemoryActive:    filepath.Join(root, "memory", "context.md"),
		AuditDir:        filepath.Join(root, "audit"),
		AuditEvents:     filepath.Join(root, "audit", "events.jsonl"),
	}
}

// Exists reports whether the harness has been initialized.
func (p Paths) Exists() bool {
	info, err := os.Stat(p.Root)
	return err == nil && info.IsDir()
}

// EnsureDirs creates the complete stable v2 directory skeleton. Per-session
// directories and checkpoints are still created only when a session exists.
func (p Paths) EnsureDirs() error {
	dirs := []string{
		p.Root, p.GraphDir, p.SessionsDir, p.MemoryDir, p.AuditDir,
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

// SkillSKILL returns <SkillsDir>/<name>/SKILL.md
func (p Paths) SkillSKILL(name string) string {
	return filepath.Join(p.SkillsDir, name, "SKILL.md")
}

// AgentsPaths is the ordered multi-path registry of AGENTS.md files
// (repo root first, then nested dir/AGENTS.md discovered on disk).
func (p Paths) AgentsPaths() []string {
	found := ListAgentsFiles(p.RepoRoot)
	if len(found) > 0 {
		return found
	}
	return []string{p.AgentsMD}
}

// SessionDir returns the path for a session id.
func (p Paths) SessionDir(id string) string {
	return filepath.Join(p.SessionsDir, id)
}
