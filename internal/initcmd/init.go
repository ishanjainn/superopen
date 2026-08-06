package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/superopen/so/internal/coding"
	"github.com/superopen/so/internal/config"
	"github.com/superopen/so/internal/discover"
	"github.com/superopen/so/internal/seed"
	"github.com/superopen/so/internal/githooks"
	"github.com/superopen/so/internal/graph"
	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/inject"
	"github.com/superopen/so/internal/llm"
	"github.com/superopen/so/internal/memory"
	"github.com/superopen/so/internal/projects"
	"github.com/superopen/so/internal/viz"
)

type Options struct {
	RepoRoot     string
	CodeOnly     bool
	Force        bool // reseed discovery-driven guardrails/evals/docs
	UseLLM       bool // force LLM upgrade (error if no key)
	NoLLM        bool // skip LLM even if key present
	TemplateRoot string
	PluginRoot   string
	SkipHooks    bool
	SkipInject   bool
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
	fmt.Println("→ seeding docs, guardrails, evals, and memory…")
	if err := seed.Seed(paths, seed.SeedOptions{
		TemplateRoot: opts.TemplateRoot,
		Profile:      profile,
		Force:        opts.Force,
	}); err != nil {
		return Report{}, err
	}
	mem := memory.NewStore(paths)
	if err := mem.Ensure(); err != nil {
		return Report{}, fmt.Errorf("memory: %w", err)
	}
	if _, err := mem.RefreshActive(""); err != nil {
		fmt.Printf("  warning: active-context refresh: %v\n", err)
	} else {
		fmt.Println("  memory: preferences/projects seeded, active-context.md ready")
	}

	// 4) LLM upgrade when key/gateway present (or --llm).
	var up seed.UpgradeResult
	wantLLM := !opts.NoLLM && (opts.UseLLM || cfg.HasLLM())
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
			_ = seed.WriteUpgradeBrief(paths, profile)
		}
	} else {
		up = seed.UpgradeResult{Used: false, Reason: "assistant or API key"}
		_ = seed.WriteUpgradeBrief(paths, profile)
		if opts.NoLLM {
			fmt.Println("→ heuristic harness ready (--no-llm); assistant should apply upgrade via .so/upgrade-brief.md")
		} else {
			fmt.Println("→ heuristic harness ready (no API key)")
			fmt.Println("  In Cursor/Claude/Codex: /so init upgrades docs with the assistant model (see .so/upgrade-brief.md)")
			fmt.Println("  Headless/CI: set an API key and run `so init --llm`, or `so apply-upgrade` with JSON")
		}
	}

	if cfg.Observability.Viz.Citymap {
		if err := viz.BuildCitymap(root, paths); err != nil {
			return Report{}, fmt.Errorf("citymap: %w", err)
		}
	}

	if !opts.SkipHooks {
		pluginRoot := opts.PluginRoot
		if pluginRoot == "" {
			pluginRoot = findPlugins()
		}
		if err := coding.Install(root, cfg.Observability.Listen, cfg.Observability.Vendors, pluginRoot); err != nil {
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

	_, _ = projects.Register(root, paths.Root, "")

	return Report{
		Paths: paths, Graph: gr, ConfigPath: paths.Config,
		Agents: len(profile.Agents), Rules: len(profile.DerivedRules),
		LLM: up,
	}, nil
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
		// Legacy template layout
		if info, err := os.Stat(filepath.Join(c, "docs")); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return "templates"
}

func findPlugins() string {
	wd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(wd, "plugins"),
		filepath.Join(wd, "superopen", "plugins"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return "plugins"
}

func orEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
