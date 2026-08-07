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
	Citymap         string
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
		AgentBrief:      filepath.Join(root, "AGENT.md"),
		GraphDir:        filepath.Join(root, "graph"),
		GraphJSON:       filepath.Join(root, "graph", "graph.json"),
		GraphReport:     filepath.Join(root, "graph", "GRAPH_REPORT.md"),
		AgentsMD:        filepath.Join(repoRoot, "AGENTS.md"),
		RulesDir:        rulesDir,
		SkillsDir:       skillsDir,
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

// EnsureDirs creates .so/ runtime dirs and the discovered rules/skills dirs.
func (p Paths) EnsureDirs() error {
	dirs := []string{
		p.Root, p.GraphDir, p.GuardrailsDir,
		p.EvalsDir, p.TracesDir, p.SessionsDir, p.Recommendations,
		p.VizDir, p.MemoryDir, filepath.Join(p.MemoryDir, "history"), p.AuditDir,
		p.RulesDir, p.SkillsDir,
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
