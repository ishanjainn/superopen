package sync

import (
	"fmt"
	"os"

	"github.com/ishanjainn/superopen/internal/coding"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/githooks"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/inject"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/retrieve"
	"github.com/ishanjainn/superopen/internal/viz"
)

type Options struct {
	RepoRoot  string
	Semantic  bool
	SkipGraph bool
}

func Run(opts Options) error {
	root := opts.RepoRoot
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return fmt.Errorf("no .so/ harness found - run `so init` first")
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		cfg = config.Default()
	}

	if err := inject.Apply(root); err != nil {
		return fmt.Errorf("inject: %w", err)
	}
	if err := coding.Install(root, cfg.Observability.Listen, cfg.Observability.Vendors, "plugins"); err != nil {
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
	if cfg.Observability.Viz.Citymap {
		if err := viz.BuildCitymap(root, paths); err != nil {
			return fmt.Errorf("citymap: %w", err)
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
