package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/coding"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/githooks"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/inject"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/retrieve"
	"github.com/ishanjainn/superopen/internal/seed"
	"github.com/ishanjainn/superopen/internal/session"
)

type Options struct {
	RepoRoot     string
	CodeOnly     bool
	Force        bool // reseed discovery-driven guardrails/evals/docs
	UseLLM       bool // force LLM upgrade (error if no key)
	NoLLM        bool // skip LLM even if key present
	TemplateRoot string
	SkipHooks    bool
	SkipInject   bool
	Vendors      []string
	SharedAgents bool
}

type Report struct {
	Paths      harness.Paths
	Graph      graph.Result
	ConfigPath string
	Agents     int
	Rules      int
	LLM        seed.UpgradeResult
}

func Run(opts Options) (Report, error) {
	root := opts.RepoRoot
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Report{}, err
		}
		root, err = harness.FindRoot(wd)
		if err != nil {
			return Report{}, err
		}
	}
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		return Report{}, err
	}
	if err := session.NewStore(paths).Ensure(); err != nil {
		return Report{}, fmt.Errorf("sessions layout: %w", err)
	}
	mem := memory.NewStore(paths)
	if err := mem.Ensure(); err != nil {
		return Report{}, fmt.Errorf("memory layout: %w", err)
	}
	if _, err := mem.RefreshActive(""); err != nil {
		return Report{}, fmt.Errorf("memory context: %w", err)
	}
	if err := audit.Ensure(paths); err != nil {
		return Report{}, fmt.Errorf("audit layout: %w", err)
	}

	cfg, err := config.Load(paths.Config)
	if err != nil {
		cfg = config.Default()
		if err := config.Save(paths.Config, cfg); err != nil {
			return Report{}, err
		}
	}
	if opts.CodeOnly {
		cfg.Graph.Semantic = false
	}
	enabled := append([]string{}, inject.DetectVendors(root)...)
	enabled = append(enabled, opts.Vendors...)
	cfg.Vendors.Enabled = uniqueStrings(enabled)
	cfg.Vendors.SharedAgents = opts.SharedAgents || cfg.Vendors.SharedAgents
	cfg.Observability.Vendors = append([]string{}, cfg.Vendors.Enabled...)
	if err := config.Save(paths.Config, cfg); err != nil {
		return Report{}, err
	}

	// 1) Graph first - install Graphify if needed, then build (JSON + HTML for UI).
	fmt.Println("→ ensuring Graphify…")
	if err := graph.EnsureTool(); err != nil {
		fmt.Printf("  warning: %v\n", err)
	}
	codeOnly := opts.CodeOnly || !cfg.Graph.Semantic
	fmt.Println("→ building repository graph…")
	gr, err := graph.Build(root, paths, codeOnly, cfg.Graph.SemanticBackend)
	if err != nil {
		return Report{}, fmt.Errorf("graph: %w", err)
	}
	htmlNote := "no graph.html"
	if gr.HasHTML {
		htmlNote = "graph.html ready"
	}
	fmt.Printf("  graph: %d nodes, %d edges (%s, %s)\n", gr.NodeCount, gr.EdgeCount, gr.Source, htmlNote)

	// 2) Read existing agent instruction files (AGENTS.md, CLAUDE.md, Cursor rules, …).
	fmt.Println("→ reading existing agent instruction files…")
	structure := detectStructure(root)
	stack := detectStack(root)
	profile := discover.BuildProfile(root, paths, stack, structure)
	fmt.Printf("  found %d agent source(s), %d derived rules\n", len(profile.Agents), len(profile.DerivedRules))

	// 3) Heuristic seed (always) - works offline / without API key.
	if opts.TemplateRoot == "" {
		opts.TemplateRoot = findTemplates()
	}
	fmt.Println("→ seeding shared docs, guardrails, and evals…")
	if err := seed.Seed(paths, seed.SeedOptions{
		TemplateRoot: opts.TemplateRoot,
		Profile:      profile,
		Force:        opts.Force,
	}); err != nil {
		return Report{}, err
	}
	// Seeding changes documentation only, so rebuild the corpus without
	// repeating the source graph build.
	if _, err := retrieve.Rebuild(root, paths); err != nil {
		return Report{}, fmt.Errorf("corpus: %w", err)
	}
	// 4) LLM upgrade when key/gateway present (or --llm).
	var up seed.UpgradeResult
	// Ambient API keys do not opt a repository into model calls. Headless/API
	// upgrade requires --llm or an explicit project llm: configuration.
	wantLLM := !opts.NoLLM && (opts.UseLLM || (cfg.HasExplicitLLM() && cfg.HasLLM()))
	if wantLLM {
		resolved := cfg.ResolveLLM()
		fmt.Printf("→ upgrading harness with LLM (%s via %s)…\n", resolved.Provider, orEmpty(resolved.Source, resolved.BaseURL))
		client := llm.NewFromConfig(cfg)
		var err error
		up, err = seed.UpgradeWithLLM(paths, profile, client, opts.UseLLM)
		if err != nil {
			return Report{}, err
		}
		if up.Used {
			fmt.Printf("  llm: wrote %d guardrails, %d eval checks\n", up.Rules, up.Checks)
		} else {
			fmt.Printf("  llm skipped: %s\n", up.Reason)
		}
	} else {
		up = seed.UpgradeResult{Used: false, Reason: "assistant or API key"}
		if opts.NoLLM {
			fmt.Println("→ heuristic harness ready (--no-llm); run `so upgrade-brief` to print the assistant prompt")
		} else {
			fmt.Println("→ heuristic harness ready (no API key)")
			fmt.Println("  In Cursor/Claude/Codex: use `so upgrade-brief`, then pipe the JSON to `so apply-upgrade`")
			fmt.Println("  Headless/CI: set an API key and run `so init --llm`, or `so apply-upgrade` with JSON")
		}
	}

	if !opts.SkipHooks {
		if err := coding.Install(root, cfg.Observability.Vendors); err != nil {
			return Report{}, fmt.Errorf("coding hooks: %w", err)
		}
		soBin, _ := os.Executable()
		if soBin == "" {
			soBin = "so"
		}
		_ = githooks.Install(root, soBin)
	}
	if !opts.SkipInject {
		if err := inject.Apply(root); err != nil {
			return Report{}, fmt.Errorf("inject: %w", err)
		}
	}
	// Refresh once more after vendor skills and shared guidance exist so the
	// eager context pack reflects the completed initialization.
	_, _ = mem.RefreshActive("")

	_, _ = projects.Register(root, paths.Root, "")

	return Report{
		Paths: paths, Graph: gr, ConfigPath: paths.Config,
		Agents: len(profile.Agents), Rules: len(profile.DerivedRules),
		LLM: up,
	}, nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func detectStructure(root string) string {
	var lines []string
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		kind := "file"
		if e.IsDir() {
			kind = "dir"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s)", e.Name(), kind))
		if len(lines) >= 40 {
			break
		}
	}
	if len(lines) == 0 {
		return "- (empty)"
	}
	return strings.Join(lines, "\n")
}

func detectStack(root string) string {
	checks := []struct{ file, label string }{
		{"go.mod", "Go"},
		{"package.json", "Node/TypeScript"},
		{"pyproject.toml", "Python"},
		{"Cargo.toml", "Rust"},
		{"pom.xml", "Java"},
	}
	var found []string
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(root, c.file)); err == nil {
			found = append(found, c.label)
		}
	}
	if len(found) == 0 {
		return "Unknown (detect from repo)"
	}
	return strings.Join(found, ", ")
}

func findTemplates() string {
	candidates := []string{
		"templates",
		filepath.Join("superopen", "templates"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "templates")}, candidates...)
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "templates"))
	}
	wd, _ := os.Getwd()
	candidates = append(candidates, filepath.Join(wd, "templates"))
	dir := wd
	for i := 0; i < 6; i++ {
		candidates = append(candidates, filepath.Join(dir, "superopen", "templates"), filepath.Join(dir, "templates"))
		dir = filepath.Dir(dir)
	}
	for _, c := range candidates {
		if info, err := os.Stat(filepath.Join(c, "knowledge")); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return "templates"
}

func orEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
