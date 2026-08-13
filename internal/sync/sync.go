package sync

import (
	"fmt"
	"os"
	"sort"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/coding"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/githooks"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/inject"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/retrieve"
	"github.com/ishanjainn/superopen/internal/session"
)

type Options struct {
	RepoRoot        string
	Semantic        bool
	SkipGraph       bool
	SkipInject      bool // set by git hooks so commits never rewrite tracked injectors
	Vendors         []string
	SharedAgents    bool
	SetSharedAgents bool
}

func Run(opts Options) error {
	root := opts.RepoRoot
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return fmt.Errorf("no .so/ harness found - run `so init` first")
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	if err := session.NewStore(paths).Ensure(); err != nil {
		return fmt.Errorf("sessions layout: %w", err)
	}
	if err := audit.Ensure(paths); err != nil {
		return fmt.Errorf("audit layout: %w", err)
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		cfg = config.Default()
	}
	if len(opts.Vendors) > 0 {
		seen := map[string]bool{}
		for _, vendor := range append(cfg.Vendors.Enabled, opts.Vendors...) {
			vendor = harness.NormalizeVendorKind(vendor)
			if vendor != "" && !seen[vendor] {
				seen[vendor] = true
			}
		}
		cfg.Vendors.Enabled = cfg.Vendors.Enabled[:0]
		for vendor := range seen {
			cfg.Vendors.Enabled = append(cfg.Vendors.Enabled, vendor)
		}
		sort.Strings(cfg.Vendors.Enabled)
		cfg.Observability.Vendors = append([]string(nil), cfg.Vendors.Enabled...)
	}
	if opts.SetSharedAgents {
		cfg.Vendors.SharedAgents = opts.SharedAgents
	}
	// Save on every sync so compact-schema migrations (for example removal of
	// the obsolete local receiver address) are reflected on disk.
	if err := config.Save(paths.Config, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if !opts.SkipInject {
		if err := inject.Apply(root); err != nil {
			return fmt.Errorf("inject: %w", err)
		}
	}
	if err := coding.Install(root, cfg.Observability.Vendors); err != nil {
		return fmt.Errorf("coding hooks: %w", err)
	}
	soBin, _ := os.Executable()
	if soBin == "" {
		soBin = "so"
	}
	_ = githooks.Install(root, soBin)
	_, _ = projects.Register(root, paths.Root, "")

	if !opts.SkipGraph {
		codeOnly := !cfg.Graph.Semantic && !opts.Semantic
		if opts.Semantic {
			codeOnly = false
		}
		if _, err := graph.Build(root, paths, codeOnly, cfg.Graph.SemanticBackend); err != nil {
			return fmt.Errorf("graph: %w", err)
		}
	}
	_ = guardrails.EnsureDefaults(paths)
	_ = memory.NewStore(paths).Ensure()
	_, _ = memory.NewStore(paths).RefreshActive("")
	if _, err := retrieve.Rebuild(root, paths); err != nil {
		return fmt.Errorf("retrieve index: %w", err)
	}
	return nil
}
