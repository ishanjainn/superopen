package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/agentcli"
	"github.com/ishanjainn/superopen/internal/coding"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/githooks"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/inject"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/userpaths"
)

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Warn   bool   `json:"warn,omitempty"`
	Detail string `json:"detail"`
}

func Run(repoRoot string) []Check {
	paths := harness.Resolve(repoRoot)
	graph.SweepStaleGraphWork(paths)
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

	for name, path := range map[string]string{
		"graph_html":  paths.GraphHTML,
		"graph_state": paths.GraphState,
		"corpus":      paths.GraphCorpus,
	} {
		_, statErr := os.Stat(path)
		checks = append(checks, Check{Name: name, OK: statErr == nil, Detail: path})
	}

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
			Detail: "no API key (evals.backend=auto uses the next live agent, then sealed claude/codex/opencode/pi CLI; heuristics do not complete reviews)",
		})
	}

	// Coding-agent CLIs for evals.backend=auto|agent_cli.
	if found := agentcli.DetectAll(); len(found) > 0 {
		checks = append(checks, Check{
			Name:   "eval_agent_cli",
			OK:     true,
			Detail: strings.Join(found, ", ") + " (evals.backend=auto uses these on true SessionEnd / idle)",
		})
	} else {
		checks = append(checks, Check{
			Name:   "eval_agent_cli",
			OK:     true,
			Warn:   true,
			Detail: "claude/codex/opencode/pi not on PATH — pending reviews wait for the next live agent (so review-brief / so apply-review)",
		})
	}

	for name, ok := range coding.Status(repoRoot, cfg.Observability.Vendors) {
		checks = append(checks, Check{Name: "hooks:" + name, OK: ok, Detail: ""})
	}
	for name, ok := range inject.StatusFor(repoRoot, cfg.Vendors.Enabled, cfg.Vendors.SharedAgents) {
		checks = append(checks, Check{Name: "inject:" + name, OK: ok, Detail: ""})
	}

	// Memory pack
	mem := memory.NewStore(paths)
	memSt := mem.Status()
	if _, err := os.Stat(paths.Lessons); os.IsNotExist(err) {
		checks = append(checks, Check{Name: "memory", OK: false, Detail: "missing memory/state.json - run so sync"})
	} else if memSt.PrefsStub || memSt.ProjectsStub {
		checks = append(checks, Check{
			Name: "memory", OK: true, Warn: true,
			Detail: fmt.Sprintf("%s - stub prefs/projects (run so sync or edit Memory → Prefs)", paths.MemoryDir),
		})
	} else {
		detail := fmt.Sprintf("%s lessons=%d semantic=%d active=%dB", paths.MemoryDir, memSt.LessonCount, memSt.SemanticCount, memSt.ActiveBytes)
		checks = append(checks, Check{Name: "memory", OK: true, Detail: detail})
	}
	if _, err := os.Stat(paths.MemoryActive); os.IsNotExist(err) {
		checks = append(checks, Check{Name: "memory_active", OK: false, Detail: "missing memory/context.md - run so sync"})
	} else if memSt.ActiveBytes == 0 {
		checks = append(checks, Check{Name: "memory_active", OK: true, Warn: true, Detail: "context.md is empty - so memory refresh"})
	} else {
		checks = append(checks, Check{Name: "memory_active", OK: true, Detail: fmt.Sprintf("%s (%d bytes)", memSt.ActivePath, memSt.ActiveBytes)})
	}

	// Guardrails (advisory rules + enforcement in one file)
	gf := paths.GuardrailsFile
	if _, err := os.Stat(gf); err == nil {
		if !cfg.GuardrailsEnabled() {
			checks = append(checks, Check{Name: "guardrails", OK: true, Warn: true, Detail: "enforcement disabled - set guardrails.enabled: true or SUPEROPEN_GUARDRAILS=on (" + gf + ")"})
		} else {
			checks = append(checks, Check{Name: "guardrails", OK: true, Detail: gf})
		}
	} else {
		checks = append(checks, Check{Name: "guardrails", OK: false, Detail: "missing guardrails.yaml - run so sync"})
	}

	// Repository audit history is an eager, self-described stream.
	if _, err := os.Stat(paths.AuditEvents); err == nil {
		checks = append(checks, Check{Name: "audit", OK: true, Detail: paths.AuditEvents})
	} else {
		checks = append(checks, Check{Name: "audit", OK: false, Detail: "missing audit/events.jsonl - run so sync"})
	}

	// Rebuildable corpus index.
	idx := paths.GraphCorpus
	if _, err := os.Stat(idx); err == nil {
		checks = append(checks, Check{Name: "corpus", OK: true, Detail: idx})
	} else {
		checks = append(checks, Check{Name: "corpus", OK: false, Detail: "run so sync to build corpus index"})
	}

	// Port ledger is machine-local runtime state, not harness content.
	if runtimeDir, runtimeErr := userpaths.RuntimeDir(repoRoot); runtimeErr == nil {
		ledger := filepath.Join(runtimeDir, "port", "ledger.json")
		if _, err := os.Stat(ledger); err == nil {
			checks = append(checks, Check{Name: "port_ledger", OK: true, Detail: ledger})
		}
	}

	// Project registry
	if projs, err := projects.List(); err != nil {
		checks = append(checks, Check{Name: "projects_registry", OK: false, Detail: err.Error()})
	} else {
		checks = append(checks, Check{Name: "projects_registry", OK: true, Detail: fmt.Sprintf("%d project(s)", len(projs))})
	}

	// Git hooks: Superopen no longer installs commit/push hooks (they slowed
	// commits and hung pushes while syncing refs/so/sessions). Absence is OK.
	_ = githooks.TrailerSession
	hooksDetail := "disabled (no Superopen commit/push hooks)"
	if out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-path", "hooks").Output(); err == nil {
		dir := strings.TrimSpace(string(out))
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(repoRoot, dir)
		}
		for _, name := range []string{"pre-push", "post-commit", "prepare-commit-msg"} {
			p := filepath.Join(dir, name)
			if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), "githook") {
				hooksDetail = "leftover Superopen hook present: " + name + " (run so sync to remove)"
				checks = append(checks, Check{Name: "git_hooks", OK: false, Warn: true, Detail: hooksDetail})
				return checks
			}
		}
	}
	checks = append(checks, Check{Name: "git_hooks", OK: true, Detail: hooksDetail})

	return checks
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
