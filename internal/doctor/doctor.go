package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/coding"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/githooks"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/inject"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/projects"
)

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Warn   bool   `json:"warn,omitempty"`
	Detail string `json:"detail"`
}

func Run(repoRoot string) []Check {
	paths := harness.Resolve(repoRoot)
	var checks []Check

	checks = append(checks, Check{Name: "harness", OK: paths.Exists(), Detail: paths.Root})

	cfg, err := config.Load(paths.Config)
	if err != nil {
		checks = append(checks, Check{Name: "config", OK: false, Detail: err.Error()})
		cfg = config.Default()
	} else {
		checks = append(checks, Check{Name: "config", OK: true, Detail: paths.Config})
	}

	_, err = os.Stat(paths.GraphJSON)
	checks = append(checks, Check{Name: "graph", OK: err == nil, Detail: paths.GraphJSON})

	_, err = os.Stat(paths.Citymap)
	checks = append(checks, Check{Name: "citymap", OK: err == nil, Detail: paths.Citymap})

	if _, err := exec.LookPath("graphify"); err == nil {
		checks = append(checks, Check{Name: "graphify", OK: true, Detail: "on PATH"})
	} else {
		checks = append(checks, Check{Name: "graphify", OK: false, Detail: "not found (stub graph used)"})
	}

	r := cfg.ResolveLLM()
	if cfg.HasLLM() {
		detail := r.Provider + " model=" + r.Model
		if r.Source != "" {
			detail += " key=" + r.Source
		}
		if r.BaseURL != "" {
			detail += " base=" + r.BaseURL
		}
		checks = append(checks, Check{Name: "llm", OK: true, Detail: detail})
	} else {
		checks = append(checks, Check{
			Name:   "llm",
			OK:     true,
			Detail: "no API key (evals/memory prefer coding-agent CLI; keys only for headless llm_api)",
		})
	}

	// Coding-agent CLIs reused for sealed backend evals/recommendations.
	if found := detectAgentCLIs(); len(found) > 0 {
		checks = append(checks, Check{
			Name:   "eval_agent_cli",
			OK:     true,
			Detail: strings.Join(found, ", ") + " (evals.backend=auto prefers these)",
		})
	} else {
		checks = append(checks, Check{
			Name:   "eval_agent_cli",
			OK:     false,
			Detail: "claude/codex not on PATH - evals fall back to API key or heuristics",
		})
	}

	for name, ok := range coding.Status(repoRoot, cfg.Observability.Vendors) {
		checks = append(checks, Check{Name: "hooks:" + name, OK: ok, Detail: ""})
	}
	for name, ok := range inject.Status(repoRoot) {
		checks = append(checks, Check{Name: "inject:" + name, OK: ok, Detail: ""})
	}

	// Memory pack
	mem := memory.NewStore(paths)
	_ = mem.Ensure()
	memSt := mem.Status()
	if memSt.PrefsStub || memSt.ProjectsStub {
		checks = append(checks, Check{
			Name: "memory", OK: true, Warn: true,
			Detail: fmt.Sprintf("%s - stub prefs/projects (run so sync or edit Memory → Prefs)", paths.MemoryDir),
		})
	} else {
		detail := fmt.Sprintf("%s lessons=%d semantic=%d active=%dB", paths.MemoryDir, memSt.LessonCount, memSt.SemanticCount, memSt.ActiveBytes)
		checks = append(checks, Check{Name: "memory", OK: true, Detail: detail})
	}
	if memSt.ActiveBytes == 0 {
		checks = append(checks, Check{Name: "memory_active", OK: true, Warn: true, Detail: "active-context.md empty - so memory refresh"})
	} else {
		checks = append(checks, Check{Name: "memory_active", OK: true, Detail: fmt.Sprintf("%s (%d bytes)", memSt.ActivePath, memSt.ActiveBytes)})
	}

	// Guardrails (advisory rules + enforcement in one file)
	gf := filepath.Join(paths.GuardrailsDir, "guardrails.yaml")
	if _, err := os.Stat(gf); err == nil {
		if !cfg.GuardrailsEnabled() {
			checks = append(checks, Check{Name: "guardrails", OK: true, Warn: true, Detail: "enforcement disabled - set guardrails.enabled: true or SUPEROPEN_GUARDRAILS=on (" + gf + ")"})
		} else {
			checks = append(checks, Check{Name: "guardrails", OK: true, Detail: gf})
		}
	} else {
		checks = append(checks, Check{Name: "guardrails", OK: false, Detail: "missing guardrails.yaml - run so sync"})
	}

	// Audit log dir
	if _, err := os.Stat(paths.AuditDir); err == nil {
		checks = append(checks, Check{Name: "audit", OK: true, Detail: paths.AuditDir})
	} else {
		checks = append(checks, Check{Name: "audit", OK: false, Detail: "missing .so/audit"})
	}

	// Retrieve index
	idx := filepath.Join(paths.GraphDir, "retrieve_index.json")
	if _, err := os.Stat(idx); err == nil {
		checks = append(checks, Check{Name: "retrieve_index", OK: true, Detail: idx})
	} else {
		checks = append(checks, Check{Name: "retrieve_index", OK: false, Detail: "run so sync to build corpus index"})
	}

	// Port ledger (only report when present; empty is normal)
	ledger := filepath.Join(paths.Root, "port", "ledger.json")
	if _, err := os.Stat(ledger); err == nil {
		checks = append(checks, Check{Name: "port_ledger", OK: true, Detail: ledger})
	}

	// Project registry
	if projs, err := projects.List(); err != nil {
		checks = append(checks, Check{Name: "projects_registry", OK: false, Detail: err.Error()})
	} else {
		checks = append(checks, Check{Name: "projects_registry", OK: true, Detail: fmt.Sprintf("%d project(s)", len(projs))})
	}

	// Git hooks
	hooksOK := false
	if out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-path", "hooks").Output(); err == nil {
		prep := filepath.Join(strings.TrimSpace(string(out)), "prepare-commit-msg")
		if !filepath.IsAbs(prep) {
			prep = filepath.Join(repoRoot, prep)
		}
		if data, err := os.ReadFile(prep); err == nil && strings.Contains(string(data), "githook") {
			hooksOK = true
		}
	}
	_ = githooks.TrailerSession
	checks = append(checks, Check{Name: "git_hooks", OK: hooksOK, Detail: "prepare-commit-msg SO-Session"})

	return checks
}

func detectAgentCLIs() []string {
	var found []string
	for _, name := range []string{"claude", "codex"} {
		if _, err := exec.LookPath(name); err == nil {
			found = append(found, name)
		}
	}
	return found
}

func Format(checks []Check) string {
	var b strings.Builder
	for _, c := range checks {
		mark := "FAIL"
		if c.Warn {
			mark = "WARN"
		} else if c.OK {
			mark = "OK  "
		}
		fmt.Fprintf(&b, "[%s] %s", mark, c.Name)
		if c.Detail != "" {
			fmt.Fprintf(&b, " - %s", c.Detail)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
