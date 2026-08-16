package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/axi"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/harness"
	projectmcp "github.com/ishanjainn/superopen/internal/mcp"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/projects"
)

func newGraphCommand() *cobra.Command {
	root := &cobra.Command{
		Use: "graph", Short: "Graphify-backed repository graph platform",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runGraphBuild(cmd, nil) },
	}
	extract := &cobra.Command{
		Use: "extract [path]", Short: "Build the complete graph extraction pipeline", Args: cobra.MaximumNArgs(1),
		RunE: runGraphBuild,
	}
	rebuild := &cobra.Command{
		Use: "rebuild", Short: "Compatibility alias for extract .", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runGraphBuild(cmd, []string{"."}) },
	}
	for _, cmd := range []*cobra.Command{root, extract, rebuild} {
		cmd.Flags().Bool("code-only", false, "Build a real AST graph without semantic extraction")
		cmd.Flags().String("backend", "", "Configured semantic backend")
		cmd.Flags().String("model", "", "Semantic backend model")
		cmd.Flags().String("mode", "", "Extraction mode")
		cmd.Flags().Bool("force", false, "Ignore incremental cache")
		cmd.Flags().Int("max-workers", 0, "AST extraction workers")
		cmd.Flags().Int("token-budget", 0, "Semantic chunk token budget")
		cmd.Flags().Int("max-concurrency", 0, "Semantic request concurrency")
		cmd.Flags().Float64("api-timeout", 0, "Semantic API timeout in seconds")
		cmd.Flags().Bool("directed", false, "Preserve directed graph edges")
		cmd.Flags().Bool("no-cluster", false, "Skip clustering")
		cmd.Flags().StringSlice("exclude", nil, "Exclude a path pattern (repeatable)")
		cmd.Flags().Float64("resolution", 0, "Community clustering resolution")
		cmd.Flags().Float64("exclude-hubs", 0, "Hub exclusion percentile")
		cmd.Flags().Bool("google-workspace", false, "Resolve supported Google Workspace shortcuts")
		cmd.Flags().String("postgres", "", "PostgreSQL DSN for schema extraction")
		cmd.Flags().Bool("cargo", false, "Extract Cargo workspace metadata")
		cmd.Flags().Bool("allow-partial", false, "Keep Graphify partial extraction diagnostics")
		cmd.Flags().Bool("timing", false, "Print Graphify stage timings")
		cmd.Flags().Bool("no-gitignore", false, "Do not apply .gitignore")
		cmd.Flags().Bool("dedup-llm", false, "Use Graphify semantic deduplication")
		cmd.Flags().String("whisper-model", "base", "Whisper model for audio/video transcription")
	}
	root.AddCommand(extract, rebuild, graphQueryCommand())

	root.AddCommand(graphUpdateCommand())
	root.AddCommand(graphNative("watch [path]", "Watch files and update the graph", "watch", pathDefaultArgs))
	root.AddCommand(graphNative("check-update [path]", "Check whether graph sources changed", "check-update", pathDefaultArgs))
	root.AddCommand(graphRead("path <A> <B>", "Find paths between two nodes", "path", cobra.ExactArgs(2)))
	root.AddCommand(graphRead("explain <node>", "Explain a node and its neighborhood", "explain", cobra.ExactArgs(1)))
	root.AddCommand(graphRead("affected <node>", "Show downstream impact", "affected", cobra.ExactArgs(1)))
	root.AddCommand(graphRead("god-nodes", "Show high-connectivity nodes", "god-nodes", cobra.NoArgs))
	root.AddCommand(graphStatsCommand())

	diagnose := &cobra.Command{Use: "diagnose", Short: "Graph diagnostics"}
	diagnose.AddCommand(graphRead("multigraph", "Diagnose duplicate edges and multigraph shape", "diagnose", cobra.NoArgs, "multigraph"))
	root.AddCommand(diagnose)
	root.AddCommand(graphNative("cluster [path]", "Cluster graph communities", "cluster-only", pathDefaultArgs))
	root.AddCommand(graphNative("label [path]", "Generate community labels", "label", pathDefaultArgs))
	root.AddCommand(graphLabelsCommand())
	root.AddCommand(graphPublishCommand())
	root.AddCommand(graphExportCommand())
	root.AddCommand(graphServeCommand())
	root.AddCommand(graphNative("add <url>", "Add a supported remote source", "add", requireURLArgs))
	root.AddCommand(graphNative("clone <github-url>", "Clone and graph a GitHub repository", "clone", requireURLArgs))
	root.AddCommand(graphNative("merge <graph...>", "Merge graphs", "merge-graphs", requireExistingArgs))
	root.AddCommand(graphGlobalCommand())
	root.AddCommand(graphProviderCommand())
	root.AddCommand(graphPRsCommand())
	root.AddCommand(graphBenchmarkCommand())
	root.AddCommand(graphResultCommand())
	root.AddCommand(graphReflectCommand())
	root.AddCommand(graphSemanticCommand())
	root.AddCommand(graphMCPCommand())
	return root
}

func graphBenchmarkCommand() *cobra.Command {
	var ledger string
	c := &cobra.Command{Use: "benchmark [graph.json]", Short: "Benchmark graph retrieval or evaluate the paired Haiku release ledger", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if ledger != "" {
			path, err := filepath.Abs(ledger)
			if err != nil {
				return err
			}
			result, err := graph.EvaluateBenchmarkLedgerFile(path)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("graph_benchmark_gate", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Agent benchmark gate pass=%t success=%d/%d control=%d/%d cost_reduction=%.1f%% graph_adoption=%.1f%%\n", result.Pass, result.TreatmentSuccesses, 16, result.ControlSuccesses, 16, result.CostReduction*100, result.GraphAdoption*100)
				for _, failure := range result.Failures {
					fmt.Fprintln(cmd.OutOrStdout(), "- "+failure)
				}
			}, result)
		}
		nativeArgs := []string{"benchmark"}
		if len(args) == 1 {
			path, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err != nil {
				return err
			}
			nativeArgs = append(nativeArgs, path)
		}
		r, err := graph.Run(cmd.Context(), repoRoot(), nativeArgs...)
		if len(r.Stderr) > 0 {
			fmt.Fprint(cmd.ErrOrStderr(), string(r.Stderr))
		}
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), string(r.Stdout))
		return nil
	}}
	c.Flags().StringVar(&ledger, "release-ledger", "", "Evaluate an opt-in 16-task paired coding-agent ledger")
	return c
}

func runGraphBuild(cmd *cobra.Command, args []string) error {
	root, paths := repoRoot(), harness.Resolve(repoRoot())
	if len(args) == 1 && args[0] != "." {
		abs, err := filepath.Abs(args[0])
		if err != nil || !withinRoot(root, abs) {
			return fmt.Errorf("extract path must be inside the repository")
		}
	}
	if err := graph.EnsureTool(); err != nil {
		return err
	}
	cfg, _ := config.Load(paths.Config)
	codeOnly, _ := cmd.Flags().GetBool("code-only")
	backend, _ := cmd.Flags().GetString("backend")
	if backend == "" {
		backend = cfg.Graph.SemanticBackend
	}
	target := root
	if len(args) == 1 {
		target, _ = filepath.Abs(args[0])
	}
	semanticOpts, err := semanticOptions(cmd, target, cfg.Graph.Mode)
	if err != nil {
		return err
	}
	if !codeOnly && cfg.Graph.Semantic && backend == "agent" {
		run, err := graph.StartSemanticRunWithOptions(cmd.Context(), root, semanticOpts)
		if err != nil {
			return err
		}
		return axi.Continuation("Graphify semantic extraction requires the coding agent", "run `so graph semantic briefs --run "+run.RunID+"` and continue through finalize, labels, and publish")
	}
	directed, _ := cmd.Flags().GetBool("directed")
	noCluster, _ := cmd.Flags().GetBool("no-cluster")
	opts := graph.BuildOptions{CodeOnly: codeOnly || !cfg.Graph.Semantic || backend == "none", SemanticBackend: backend, Target: target, Directed: directed, NoCluster: noCluster}
	if cmd.Flags().Changed("model") {
		v, _ := cmd.Flags().GetString("model")
		opts.ExtraArgs = append(opts.ExtraArgs, "--model", v)
	}
	mode := cfg.Graph.Mode
	if cmd.Flags().Changed("mode") {
		mode, _ = cmd.Flags().GetString("mode")
	}
	// Graphify's ordinary extraction mode is represented by no flag; its CLI
	// accepts --mode only for the richer `deep` protocol.
	if strings.EqualFold(strings.TrimSpace(mode), "deep") {
		opts.ExtraArgs = append(opts.ExtraArgs, "--mode", "deep")
	}
	if force, _ := cmd.Flags().GetBool("force"); force {
		opts.ExtraArgs = append(opts.ExtraArgs, "--force")
	}
	for _, name := range []string{"max-workers", "token-budget", "max-concurrency"} {
		if cmd.Flags().Changed(name) {
			v, _ := cmd.Flags().GetInt(name)
			opts.ExtraArgs = append(opts.ExtraArgs, "--"+name, strconv.Itoa(v))
		}
	}
	if cmd.Flags().Changed("api-timeout") {
		v, _ := cmd.Flags().GetFloat64("api-timeout")
		opts.ExtraArgs = append(opts.ExtraArgs, "--api-timeout", strconv.FormatFloat(v, 'f', -1, 64))
	}
	for _, name := range []string{"no-cluster", "google-workspace", "cargo", "allow-partial", "timing", "no-gitignore", "dedup-llm"} {
		if v, _ := cmd.Flags().GetBool(name); v {
			opts.ExtraArgs = append(opts.ExtraArgs, "--"+name)
		}
	}
	for _, name := range []string{"resolution", "exclude-hubs"} {
		if cmd.Flags().Changed(name) {
			v, _ := cmd.Flags().GetFloat64(name)
			opts.ExtraArgs = append(opts.ExtraArgs, "--"+name, strconv.FormatFloat(v, 'f', -1, 64))
		}
	}
	for _, value := range mustStringSlice(cmd, "exclude") {
		opts.ExtraArgs = append(opts.ExtraArgs, "--exclude", value)
	}
	if dsn, _ := cmd.Flags().GetString("postgres"); dsn != "" {
		if err := validateDSN(dsn, "postgres"); err != nil {
			return err
		}
		opts.ExtraArgs = append(opts.ExtraArgs, "--postgres", dsn)
	}
	res, err := graph.RefreshAtomicWithOptions(root, paths, opts)
	if err != nil {
		return err
	}
	o := out()
	payload := map[string]any{"path": res.Path, "nodes": res.NodeCount, "edges": res.EdgeCount, "engine": res.Engine, "engine_version": res.EngineVersion, "status": res.Status}
	return o.HumanOrJSON("graph_build", func() {
		fmt.Fprintf(o.W, "Wrote %s (%d nodes, %d edges, %s %s)\n", res.Path, res.NodeCount, res.EdgeCount, res.Engine, res.EngineVersion)
	}, payload)
}

func semanticOptions(cmd *cobra.Command, target, defaultMode string) (graph.SemanticStartOptions, error) {
	getBool := func(name string) bool { v, _ := cmd.Flags().GetBool(name); return v }
	getInt := func(name string) int { v, _ := cmd.Flags().GetInt(name); return v }
	getFloat := func(name string) float64 { v, _ := cmd.Flags().GetFloat64(name); return v }
	whisper, _ := cmd.Flags().GetString("whisper-model")
	postgres, _ := cmd.Flags().GetString("postgres")
	if postgres != "" {
		if err := validateDSN(postgres, "postgres"); err != nil {
			return graph.SemanticStartOptions{}, err
		}
	}
	mode, _ := cmd.Flags().GetString("mode")
	if !cmd.Flags().Changed("mode") {
		mode = defaultMode
	}
	return graph.SemanticStartOptions{
		Target: target, WhisperModel: whisper, Directed: getBool("directed"),
		Deep: strings.EqualFold(strings.TrimSpace(mode), "deep"), Force: getBool("force"),
		NoCluster: getBool("no-cluster"), Excludes: mustStringSlice(cmd, "exclude"),
		Resolution: getFloat("resolution"), ExcludeHubs: getFloat("exclude-hubs"),
		GoogleWorkspace: getBool("google-workspace"), NoGitignore: getBool("no-gitignore"),
		PostgresDSN: postgres, Cargo: getBool("cargo"), AllowPartial: getBool("allow-partial"),
		Timing: getBool("timing"), DedupLLM: getBool("dedup-llm"),
		MaxWorkers: getInt("max-workers"), TokenBudget: getInt("token-budget"),
		MaxConcurrency: getInt("max-concurrency"), APITimeout: getFloat("api-timeout"),
	}, nil
}

func mustStringSlice(cmd *cobra.Command, name string) []string {
	v, _ := cmd.Flags().GetStringSlice(name)
	return v
}
func validateDSN(raw, want string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid %s DSN", want)
	}
	if want == "postgres" && u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("PostgreSQL DSN must use postgres:// or postgresql://")
	}
	if u.User != nil {
		return fmt.Errorf("PostgreSQL DSN must not contain credentials; use PGUSER/PGPASSWORD environment variables")
	}
	for key := range u.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "api_key") {
			return fmt.Errorf("PostgreSQL DSN must not contain credential query parameters; use PostgreSQL environment variables")
		}
	}
	return nil
}

func graphQueryCommand() *cobra.Command {
	var dfs bool
	var contexts []string
	var terms []string
	var originalQuestion string
	var budget int
	c := &cobra.Command{Use: "query <question>", Short: "BFS/DFS-scoped graph retrieval", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root := repoRoot()
		question := strings.Join(args, " ")
		if len(terms) > 12 {
			return fmt.Errorf("at most 12 exact graph terms may be selected")
		}
		for _, term := range terms {
			if strings.TrimSpace(term) == "" {
				return fmt.Errorf("--term cannot be empty")
			}
		}
		if strings.TrimSpace(originalQuestion) == "" {
			originalQuestion = question
		}
		graphQuestion := question
		if len(terms) > 0 {
			graphQuestion = strings.Join(terms, " ")
		}
		if !cmd.Flags().Changed("budget") {
			if cfg, err := config.Load(harness.Resolve(root).Config); err == nil && cfg.Graph.QueryBudget > 0 {
				budget = cfg.Graph.QueryBudget
			}
		}
		extra := []string{"--budget", strconv.Itoa(budget)}
		if dfs {
			extra = append(extra, "--dfs")
		}
		for _, c := range contexts {
			extra = append(extra, "--context", c)
		}
		answer, err := graph.QueryWithArgs(root, graphQuestion, extra)
		if err != nil {
			return axi.Err(err)
		}
		o := out()
		truncated := strings.Contains(strings.ToLower(answer), "truncated")
		payload := map[string]any{"answer": answer, "question": originalQuestion, "selected_terms": terms, "mode": map[bool]string{true: "dfs", false: "bfs"}[dfs], "budget": budget, "graph_sha256": graph.GraphHash(root), "engine_version": graph.PinnedVersion, "truncated": truncated}
		return o.HumanOrJSON("graph_query", func() {
			if len(terms) > 0 {
				fmt.Fprintf(o.W, "Selected graph terms: %s\n", strings.Join(terms, ", "))
			}
			fmt.Fprint(o.W, answer)
		}, payload)
	}}
	c.Flags().BoolVar(&dfs, "dfs", false, "Use depth-first traversal")
	c.Flags().StringSliceVar(&contexts, "context", nil, "Restrict to graph context (repeatable)")
	c.Flags().StringSliceVar(&terms, "term", nil, "Exact graph vocabulary term selected by the agent (repeatable, max 12)")
	c.Flags().StringVar(&originalQuestion, "original-question", "", "Preserve the user's unexpanded question in AXI output")
	c.Flags().IntVar(&budget, "budget", 2000, "Graphify result token budget")
	return c
}

type argBuilder func(root string, args []string) ([]string, error)

func passthroughArgs(_ string, args []string) ([]string, error) { return args, nil }
func noPositionalArgs(_ string, args []string) ([]string, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("unexpected positional arguments: %s", strings.Join(args, " "))
	}
	return nil, nil
}
func pathDefaultArgs(_ string, args []string) ([]string, error) {
	if len(args) > 1 {
		return nil, fmt.Errorf("expected at most one path")
	}
	if len(args) == 0 {
		return []string{"."}, nil
	}
	return args, nil
}
func requireURLArgs(_ string, args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("URL is required")
	}
	u, e := url.Parse(args[0])
	if e != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid URL")
	}
	return args, nil
}
func requireExistingArgs(root string, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("at least one graph is required")
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		p := a
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if _, e := os.Stat(p); e != nil {
			return nil, fmt.Errorf("graph path %q: %w", a, e)
		}
	}
	return args, nil
}
func withinRoot(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func graphNative(use, short, native string, builder argBuilder) *cobra.Command {
	c := &cobra.Command{Use: use, Short: short}
	flags := bindGraphifyFlags(c, native)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		built, err := builder(repoRoot(), args)
		if err != nil {
			return err
		}
		built, err = appendGraphifyFlags(cmd, built, flags)
		if err != nil {
			return err
		}
		r, err := graph.Run(cmd.Context(), repoRoot(), append([]string{native}, built...)...)
		if len(r.Stderr) > 0 {
			fmt.Fprint(cmd.ErrOrStderr(), string(r.Stderr))
		}
		if err != nil {
			return err
		}
		if native == "path" || native == "explain" || native == "affected" {
			if err := graph.RecordQueryStamp(repoRoot(), native); err != nil {
				return err
			}
		}
		fmt.Fprint(cmd.OutOrStdout(), string(r.Stdout))
		return nil
	}
	return c
}

func graphRead(use, short, native string, validator cobra.PositionalArgs, prefix ...string) *cobra.Command {
	c := &cobra.Command{Use: use, Short: short}
	schemaKey := native
	if native == "export" && len(prefix) > 0 {
		alias := prefix[0]
		if alias == "callflow-html" {
			alias = "callflow"
		}
		schemaKey = "export:" + alias
	}
	flags := bindGraphifyFlags(c, schemaKey)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validator(cmd, args); err != nil {
			return err
		}
		flagArgs, err := appendGraphifyFlags(cmd, nil, flags)
		if err != nil {
			return err
		}
		if native == "export" {
			if err := validateExportArgs(repoRoot(), flagArgs); err != nil {
				return err
			}
		}
		if err := graph.ValidateQueryableGraph(repoRoot()); err != nil {
			return err
		}
		nativeArgs := append([]string{native}, prefix...)
		nativeArgs = append(nativeArgs, args...)
		nativeArgs = append(nativeArgs, flagArgs...)
		nativeArgs = append(nativeArgs, "--graph", harness.Resolve(repoRoot()).GraphJSON)
		r, err := graph.Run(cmd.Context(), repoRoot(), nativeArgs...)
		if len(r.Stderr) > 0 {
			fmt.Fprint(cmd.ErrOrStderr(), string(r.Stderr))
		}
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), string(r.Stdout))
		return nil
	}
	return c
}

func bindGraphifyFlags(cmd *cobra.Command, native string) []graph.FlagSchema {
	var schema graph.CommandSchemaEntry
	for key, candidate := range graph.CommandSchema {
		if key == native || (candidate.Native == native && !strings.Contains(native, ":")) {
			schema = candidate
			break
		}
	}
	for _, f := range schema.Flags {
		switch f.Type {
		case "bool":
			cmd.Flags().Bool(f.Name, f.Default.(bool), f.Usage)
		case "int":
			cmd.Flags().Int(f.Name, f.Default.(int), f.Usage)
		case "string":
			cmd.Flags().String(f.Name, f.Default.(string), f.Usage)
		case "stringSlice":
			cmd.Flags().StringSlice(f.Name, nil, f.Usage)
		}
	}
	return schema.Flags
}

func appendGraphifyFlags(cmd *cobra.Command, args []string, schema []graph.FlagSchema) ([]string, error) {
	for _, f := range schema {
		if !cmd.Flags().Changed(f.Name) {
			continue
		}
		if f.ConflictsWith != "" && cmd.Flags().Changed(f.ConflictsWith) {
			return nil, fmt.Errorf("--%s and --%s are mutually exclusive", f.Name, f.ConflictsWith)
		}
		switch f.Type {
		case "bool":
			v, _ := cmd.Flags().GetBool(f.Name)
			if v {
				args = append(args, "--"+f.Name)
			}
		case "int":
			v, _ := cmd.Flags().GetInt(f.Name)
			args = append(args, "--"+f.Name, strconv.Itoa(v))
		case "string":
			v, _ := cmd.Flags().GetString(f.Name)
			args = append(args, "--"+f.Name, v)
		case "stringSlice":
			values, _ := cmd.Flags().GetStringSlice(f.Name)
			for _, v := range values {
				args = append(args, "--"+f.Name, v)
			}
		}
	}
	return args, nil
}

func validateExportArgs(root string, args []string) error {
	for i, arg := range args {
		if arg == "--password" || strings.HasPrefix(arg, "--password=") {
			return fmt.Errorf("database passwords must use NEO4J_PASSWORD or FALKORDB_PASSWORD, not process arguments")
		}
		if arg == "--push" && i+1 < len(args) {
			u, err := url.Parse(args[i+1])
			if err != nil || u.Scheme == "" || u.Host == "" {
				return fmt.Errorf("invalid graph database URI")
			}
		}
		if (arg == "--output" || arg == "--dir") && i+1 < len(args) {
			p := args[i+1]
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			parent := filepath.Dir(p)
			if info, err := os.Stat(parent); err != nil || !info.IsDir() {
				return fmt.Errorf("export destination parent does not exist: %s", parent)
			}
		}
	}
	return nil
}

func graphStatsCommand() *cobra.Command {
	return &cobra.Command{Use: "stats", Short: "Show canonical graph statistics", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := graph.ValidateQueryableGraph(repoRoot()); err != nil {
			return err
		}
		data, err := os.ReadFile(harness.Resolve(repoRoot()).GraphState)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), string(data))
		return nil
	}}
}

func graphUpdateCommand() *cobra.Command {
	var codeOnly bool
	var backend string
	var force, noCluster bool
	c := &cobra.Command{Use: "update [path]", Short: "Incrementally update the cached graph", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root := repoRoot()
		paths := harness.Resolve(root)
		target, err := resolveGraphUpdateTarget(root, args)
		if err != nil {
			return err
		}
		cfg, err := config.Load(paths.Config)
		if err != nil {
			return err
		}
		if backend == "" {
			backend = cfg.Graph.SemanticBackend
		}
		nativeFlags := []string{}
		if force {
			nativeFlags = append(nativeFlags, "--force")
		}
		if noCluster {
			nativeFlags = append(nativeFlags, "--no-cluster")
		}
		res, err := graph.UpdateAtomicTargetWithFlags(cmd.Context(), root, target, paths, codeOnly || !cfg.Graph.Semantic || backend == "none", backend, nativeFlags)
		if err != nil {
			return err
		}
		if res.Status == "needs_agent_semantic" {
			out().Object("graph_update", res)
			return axi.Continuation("incremental semantic update requires the coding agent", "resume with `so graph semantic briefs --run "+res.RunID+"`")
		}
		out().Object("graph_update", res)
		return nil
	}}
	c.Flags().BoolVar(&codeOnly, "code-only", false, "Update only the real AST graph")
	c.Flags().StringVar(&backend, "backend", "", "Configured semantic backend")
	c.Flags().BoolVar(&force, "force", false, "Allow an intentional graph shrink")
	c.Flags().BoolVar(&noCluster, "no-cluster", false, "Skip clustering after the incremental refresh")
	return c
}

func resolveGraphUpdateTarget(root string, args []string) (string, error) {
	target := root
	if len(args) == 1 {
		var err error
		target, err = filepath.Abs(args[0])
		if err != nil {
			return "", err
		}
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	targetResolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("update path must be an existing directory: %s", target)
	}
	if !withinRoot(rootResolved, targetResolved) {
		return "", fmt.Errorf("update path must be inside the repository")
	}
	if info, statErr := os.Stat(targetResolved); statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("update path must be an existing directory: %s", targetResolved)
	}
	return targetResolved, nil
}

func graphExportCommand() *cobra.Command {
	c := &cobra.Command{Use: "export", Short: "Export graph artifacts"}
	for _, f := range []string{"html", "wiki", "obsidian", "svg", "graphml", "canvas", "cypher", "callflow", "tree", "neo4j", "falkordb"} {
		nativeFormat := f
		if f == "canvas" {
			var output string
			canvas := &cobra.Command{Use: "canvas", Short: "Export only an Obsidian Canvas artifact", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
				if err := graph.ValidateQueryableGraph(repoRoot()); err != nil {
					return err
				}
				return graph.ExportCanvas(cmd.Context(), repoRoot(), output)
			}}
			canvas.Flags().StringVar(&output, "output", "", "Canvas output path (default .so/graph/exports/graph.canvas)")
			c.AddCommand(canvas)
			continue
		}
		if f == "cypher" {
			cypher := &cobra.Command{Use: "cypher", Short: "Export Cypher statements without a database push", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
				if err := graph.ValidateQueryableGraph(repoRoot()); err != nil {
					return err
				}
				r, err := graph.Run(cmd.Context(), repoRoot(), "export", "neo4j", "--graph", harness.Resolve(repoRoot()).GraphJSON)
				if len(r.Stderr) > 0 {
					fmt.Fprint(cmd.ErrOrStderr(), string(r.Stderr))
				}
				if err == nil {
					fmt.Fprint(cmd.OutOrStdout(), string(r.Stdout))
				}
				return err
			}}
			c.AddCommand(cypher)
			continue
		}
		if f == "callflow" {
			nativeFormat = "callflow-html"
		}
		if f == "canvas" {
			nativeFormat = "obsidian"
		}
		if f == "cypher" {
			nativeFormat = "neo4j"
		}
		if f == "tree" {
			c.AddCommand(graphNative("tree", "Export tree", "tree", noPositionalArgs))
			continue
		}
		c.AddCommand(graphRead(f, "Export "+f, "export", cobra.NoArgs, nativeFormat))
	}
	return c
}

func graphGlobalCommand() *cobra.Command {
	c := &cobra.Command{Use: "global", Short: "Manage the global multi-repository graph"}
	add := graphNative("add <graph.json>", "Add or update a graph", "global", passthroughArgsWithPrefix("add"))
	add.Args = cobra.ExactArgs(1)
	add.Flags().String("as", "", "Repository tag")
	add.RunE = func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err != nil {
			return err
		}
		nativeArgs := []string{"global", "add", path}
		if cmd.Flags().Changed("as") {
			tag, _ := cmd.Flags().GetString("as")
			nativeArgs = append(nativeArgs, "--as", tag)
		}
		r, err := graph.Run(cmd.Context(), repoRoot(), nativeArgs...)
		if len(r.Stderr) > 0 {
			fmt.Fprint(cmd.ErrOrStderr(), string(r.Stderr))
		}
		if err == nil {
			fmt.Fprint(cmd.OutOrStdout(), string(r.Stdout))
			if filepath.Base(filepath.Dir(path)) == "graph" && filepath.Base(filepath.Dir(filepath.Dir(path))) == ".so" {
				projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(path)))
				_, _ = projects.Register(projectRoot, filepath.Join(projectRoot, ".so"), "")
			}
		}
		return err
	}
	remove := graphNative("remove <tag>", "Remove a repository graph", "global", passthroughArgsWithPrefix("remove"))
	remove.Args = cobra.ExactArgs(1)
	list := graphNative("list", "List repository graphs", "global", passthroughArgsWithPrefix("list"))
	list.Args = cobra.NoArgs
	path := graphNative("path", "Print the global graph path", "global", passthroughArgsWithPrefix("path"))
	path.Args = cobra.NoArgs
	c.AddCommand(add, remove, list, path)
	return c
}

func graphPRsCommand() *cobra.Command {
	c := &cobra.Command{Use: "prs [number]", Short: "PR impact, conflicts, worktrees, and triage", Args: cobra.MaximumNArgs(1)}
	flags := bindGraphifyFlags(c, "prs")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		built, err := appendGraphifyFlags(cmd, args, flags)
		if err != nil {
			return err
		}
		built = append(built, "--graph", harness.Resolve(repoRoot()).GraphJSON)
		r, err := graph.Run(cmd.Context(), repoRoot(), append([]string{"prs"}, built...)...)
		if len(r.Stderr) > 0 {
			fmt.Fprint(cmd.ErrOrStderr(), string(r.Stderr))
		}
		if err == nil {
			fmt.Fprint(cmd.OutOrStdout(), string(r.Stdout))
		}
		return err
	}
	return c
}

func graphProviderCommand() *cobra.Command {
	root := &cobra.Command{Use: "provider", Short: "Manage Graphify-compatible custom providers (secret values stay in environment variables)"}
	run := func(cmd *cobra.Command, args []string) error {
		r, err := graph.Run(cmd.Context(), repoRoot(), append([]string{"provider"}, args...)...)
		if len(r.Stderr) > 0 {
			fmt.Fprint(cmd.ErrOrStderr(), string(r.Stderr))
		}
		if err == nil {
			fmt.Fprint(cmd.OutOrStdout(), string(r.Stdout))
		}
		return err
	}
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return run(cmd, []string{"list"}) }}
	show := &cobra.Command{Use: "show <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return run(cmd, append([]string{"show"}, args...)) }}
	remove := &cobra.Command{Use: "remove <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return run(cmd, append([]string{"remove"}, args...)) }}
	var baseURL, model, envKey string
	var pricingInput, pricingOutput float64
	add := &cobra.Command{Use: "add <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		u, err := url.Parse(baseURL)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "https" && !isLoopbackHost(u.Hostname())) {
			return fmt.Errorf("--base-url must be HTTPS or a loopback development endpoint")
		}
		if model == "" || envKey == "" || strings.ContainsAny(envKey, "= \t\r\n") {
			return fmt.Errorf("--default-model and a valid --env-key variable name are required")
		}
		return run(cmd, []string{"add", args[0], "--base-url", baseURL, "--default-model", model, "--env-key", envKey, "--pricing-input", strconv.FormatFloat(pricingInput, 'f', -1, 64), "--pricing-output", strconv.FormatFloat(pricingOutput, 'f', -1, 64)})
	}}
	add.Flags().StringVar(&baseURL, "base-url", "", "Provider API base URL")
	add.Flags().StringVar(&model, "default-model", "", "Default model identifier")
	add.Flags().StringVar(&envKey, "env-key", "", "Environment variable containing the API key (never the secret itself)")
	add.Flags().Float64Var(&pricingInput, "pricing-input", 0, "Input price per million tokens")
	add.Flags().Float64Var(&pricingOutput, "pricing-output", 0, "Output price per million tokens")
	root.AddCommand(add, list, show, remove)
	return root
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
func passthroughArgsWithPrefix(prefix string) argBuilder {
	return func(_ string, args []string) ([]string, error) { return append([]string{prefix}, args...), nil }
}

func graphSemanticCommand() *cobra.Command {
	c := &cobra.Command{Use: "semantic", Short: "Resumable host-agent semantic extraction"}
	status := &cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runID, _ := cmd.Flags().GetString("run")
		run, _, err := graph.SemanticStatus(harness.Resolve(repoRoot()), runID)
		if err != nil {
			return err
		}
		out().Object("graph_semantic_status", run)
		return nil
	}}
	status.Flags().String("run", "", "Run id")
	briefs := &cobra.Command{Use: "briefs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runID, _ := cmd.Flags().GetString("run")
		items, err := graph.SemanticBriefs(harness.Resolve(repoRoot()), runID)
		if err != nil {
			var rebased *graph.SemanticRebasedError
			if errors.As(err, &rebased) {
				return axi.Continuation("semantic sources changed and the run was rebased", "resume with `so graph semantic briefs --run "+rebased.RunID+"`")
			}
			return err
		}
		return out().HumanOrJSON("graph_semantic_briefs", func() {
			for i, b := range items {
				fmt.Printf("--- chunk %d ---\n%s\n", i+1, b)
			}
		}, map[string]any{"run_id": runID, "briefs": items})
	}}
	briefs.Flags().String("run", "", "Run id")
	apply := &cobra.Command{Use: "apply [file|-]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runID, _ := cmd.Flags().GetString("run")
		chunk, _ := cmd.Flags().GetInt("chunk")
		b, err := readGraphInput(args)
		if err != nil {
			return err
		}
		err = graph.ApplySemanticChunk(harness.Resolve(repoRoot()), runID, chunk, b)
		var rebased *graph.SemanticRebasedError
		if errors.As(err, &rebased) {
			return axi.Continuation("semantic sources changed and the run was rebased", "resume with `so graph semantic briefs --run "+rebased.RunID+"`")
		}
		return err
	}}
	apply.Flags().String("run", "", "Run id")
	apply.Flags().Int("chunk", 0, "Chunk number")
	finalize := &cobra.Command{Use: "finalize", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runID, _ := cmd.Flags().GetString("run")
		return graph.FinalizeSemantic(cmd.Context(), harness.Resolve(repoRoot()), runID)
	}}
	finalize.Flags().String("run", "", "Run id")
	c.AddCommand(status, briefs, apply, finalize)
	return c
}

func graphLabelsCommand() *cobra.Command {
	c := &cobra.Command{Use: "labels", Short: "Host-agent community labeling protocol"}
	brief := &cobra.Command{Use: "brief", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runID, _ := cmd.Flags().GetString("run")
		b, err := graph.LabelsBrief(harness.Resolve(repoRoot()), runID)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), b)
		return nil
	}}
	brief.Flags().String("run", "", "Run id")
	apply := &cobra.Command{Use: "apply [file|-]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		runID, _ := cmd.Flags().GetString("run")
		b, err := readGraphInput(args)
		if err != nil {
			return err
		}
		return graph.ApplyLabels(harness.Resolve(repoRoot()), runID, b)
	}}
	apply.Flags().String("run", "", "Run id")
	c.AddCommand(brief, apply)
	return c
}

func graphPublishCommand() *cobra.Command {
	var runID string
	c := &cobra.Command{Use: "publish", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		res, err := graph.PublishSemantic(cmd.Context(), harness.Resolve(repoRoot()), runID)
		if err != nil {
			return err
		}
		out().Object("graph_publish", res)
		return nil
	}}
	c.Flags().StringVar(&runID, "run", "", "Run id")
	return c
}

func graphMCPCommand() *cobra.Command {
	c := &cobra.Command{Use: "mcp", Short: "Manage the opt-in Superopen graph MCP projection"}
	install := &cobra.Command{Use: "install", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		root := repoRoot()
		paths := harness.Resolve(root)
		cfg, err := config.Load(paths.Config)
		if err != nil {
			return err
		}
		server := config.MCPServer{Name: "superopen-graph", Command: "so", Args: []string{"graph", "serve", "--transport", "stdio", "--root", "."}}
		cfg.MCP.Servers = projectmcp.MergeServers(cfg.MCP.Servers, []config.MCPServer{server})
		if err := config.Save(paths.Config, cfg); err != nil {
			return err
		}
		return projectmcp.Project(root, cfg)
	}}
	uninstall := &cobra.Command{Use: "uninstall", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		root := repoRoot()
		paths := harness.Resolve(root)
		cfg, err := config.Load(paths.Config)
		if err != nil {
			return err
		}
		kept := cfg.MCP.Servers[:0]
		for _, s := range cfg.MCP.Servers {
			if s.Name != "superopen-graph" {
				kept = append(kept, s)
			}
		}
		cfg.MCP.Servers = kept
		if err := config.Save(paths.Config, cfg); err != nil {
			return err
		}
		return projectmcp.RemoveManaged(root, "superopen-graph")
	}}
	status := &cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(harness.Resolve(repoRoot()).Config)
		if err != nil {
			return err
		}
		enabled := false
		for _, s := range cfg.MCP.Servers {
			if s.Name == "superopen-graph" {
				enabled = true
			}
		}
		out().Object("graph_mcp", map[string]any{"installed": enabled, "name": "superopen-graph"})
		return nil
	}}
	c.AddCommand(install, uninstall, status)
	return c
}

func graphServeCommand() *cobra.Command {
	var transport, host, mountPath, rootArg string
	var port int
	var jsonResponse, stateless bool
	c := &cobra.Command{Use: "serve", Short: "Serve the canonical graph over MCP", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		root := repoRoot()
		if rootArg != "" && rootArg != "." {
			abs, err := filepath.Abs(rootArg)
			if err != nil {
				return err
			}
			if !withinRoot(root, abs) {
				return fmt.Errorf("--root must be inside the repository")
			}
			root = abs
		}
		if err := graph.ValidateQueryableGraph(root); err != nil {
			return err
		}
		args := []string{"--transport", transport}
		if transport == "http" {
			args = append(args, "--host", host, "--port", strconv.Itoa(port), "--path", mountPath)
			if jsonResponse {
				args = append(args, "--json-response")
			}
			if stateless {
				args = append(args, "--stateless")
			}
		}
		return graph.Serve(cmd.Context(), root, args...)
	}}
	c.Flags().StringVar(&transport, "transport", "stdio", "stdio or http")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "HTTP bind host")
	c.Flags().IntVar(&port, "port", 8080, "HTTP bind port")
	c.Flags().StringVar(&mountPath, "path", "/mcp", "HTTP mount path")
	c.Flags().StringVar(&rootArg, "root", ".", "Repository root")
	c.Flags().BoolVar(&jsonResponse, "json-response", false, "Use MCP JSON responses")
	c.Flags().BoolVar(&stateless, "stateless", false, "Use stateless HTTP sessions")
	return c
}

func readGraphInput(args []string) ([]byte, error) {
	if len(args) == 0 || args[0] == "-" {
		return io.ReadAll(os.Stdin)
	}
	p, err := filepath.Abs(args[0])
	if err != nil {
		return nil, err
	}
	if !withinRoot(repoRoot(), p) {
		return nil, fmt.Errorf("input file must be inside the repository")
	}
	return os.ReadFile(p)
}

func graphResultCommand() *cobra.Command {
	group := &cobra.Command{Use: "result", Short: "Record graph query outcomes in Superopen memory"}
	var question, answer, answerFile, queryType, outcome, correction, sessionID string
	var nodes []string
	save := &cobra.Command{Use: "save", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		root := repoRoot()
		paths := harness.Resolve(root)
		hash := graph.GraphHash(root)
		if hash == "" {
			return fmt.Errorf("graph state has no hash")
		}
		if answerFile != "" {
			path, err := filepath.Abs(answerFile)
			if err != nil || !withinRoot(root, path) {
				return fmt.Errorf("--answer-file must be inside the repository")
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			answer = string(body)
		}
		if queryType != "query" && queryType != "path_query" && queryType != "explain" {
			return fmt.Errorf("--type must be query, path_query, or explain")
		}
		if err := graph.ValidateNodes(paths.GraphJSON, nodes); err != nil {
			return err
		}
		saved, err := memory.NewStore(paths).AddGraphOutcome(memory.GraphOutcome{Question: question, QueryType: queryType, AnswerSummary: answer, SourceNodes: nodes, Outcome: outcome, Correction: correction, SessionID: sessionID, GraphSHA256: hash})
		if err != nil {
			return err
		}
		out().Object("graph_outcome", saved)
		return nil
	}}
	save.Flags().StringVar(&question, "question", "", "Original graph question")
	save.Flags().StringVar(&answer, "answer", "", "Concise answer summary")
	save.Flags().StringVar(&answerFile, "answer-file", "", "Read the answer summary from a repository file")
	save.Flags().StringVar(&queryType, "type", "query", "query, path_query, or explain")
	save.Flags().StringSliceVar(&nodes, "node", nil, "Cited graph node id (repeatable)")
	save.Flags().StringVar(&outcome, "outcome", "", "useful, dead_end, or corrected")
	save.Flags().StringVar(&correction, "correction", "", "Correction when outcome=corrected")
	save.Flags().StringVar(&sessionID, "session", "", "Originating session id")
	group.AddCommand(save)
	return group
}

func graphReflectCommand() *cobra.Command {
	var ifStale bool
	var halfLife float64
	var minCorroboration int
	c := &cobra.Command{Use: "reflect", Short: "Derive graph lessons from recorded outcomes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		paths := harness.Resolve(repoRoot())
		items, err := memory.NewStore(paths).ListGraphOutcomes()
		if err != nil {
			return err
		}
		if err := graph.ReflectOutcomesWithOptions(paths, items, graph.ReflectOptions{IfStale: ifStale, HalfLifeDays: halfLife, MinCorroboration: minCorroboration}); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Reflected %d graph outcomes\n", len(items))
		return nil
	}}
	c.Flags().BoolVar(&ifStale, "if-stale", false, "Skip deterministic reflection when lessons are current")
	c.Flags().Float64Var(&halfLife, "half-life-days", 30, "Signal half-life in days")
	c.Flags().IntVar(&minCorroboration, "min-corroboration", 2, "Useful results required to prefer a node")
	return c
}
