package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/inject"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/recommend"
	"github.com/ishanjainn/superopen/internal/retrieve"
)

// RefreshOptions controls the cheap post-pull refresh path.
type RefreshOptions struct {
	RepoRoot  string
	SkipGraph bool
	Force     bool
}

type refreshMarker struct {
	SHA       string    `json:"sha"`
	At        time.Time `json:"at"`
	GraphBuilt bool     `json:"graph_built,omitempty"`
}

func refreshMarkerPath(paths harness.Paths) string {
	return filepath.Join(paths.MemoryDir, "last-refresh.json")
}

// Refresh is a lite sync for post-merge / post-checkout / so refresh.
// Skips coding-hook reinstall and citymap; rebuilds graph only when shared harness or HEAD changed.
func Refresh(opts RefreshOptions) error {
	root := opts.RepoRoot
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return fmt.Errorf("no .so/ harness found - run `so init` first")
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		cfg = config.Default()
	}

	sha := gitSHA(root)
	marker := loadRefreshMarker(paths)
	sharedChanged := sharedHarnessChanged(paths, marker.At) || opts.Force
	shaChanged := sha != "" && sha != marker.SHA
	// A commit always changes HEAD, but Graphify's own clustering is not
	// deterministic - rebuilding on every commit regardless of what changed
	// churns graph.json/graph.html/GRAPH_REPORT.md even when nothing Graphify
	// would index actually moved (e.g. a commit touching only .so/ or docs).
	sourceChanged := shaChanged && (marker.SHA == "" || indexableFilesChanged(root, marker.SHA, sha))

	_ = guardrails.EnsureDefaults(paths)
	_ = memory.NewStore(paths).Ensure()
	_, _ = memory.NewStore(paths).RefreshActive("")
	_ = inject.Apply(root)
	if _, err := retrieve.Rebuild(root, paths); err != nil {
		return fmt.Errorf("retrieve index: %w", err)
	}
	_ = recommend.MarkStaleFlags(paths)

	builtGraph := false
	if !opts.SkipGraph && (sharedChanged || sourceChanged || opts.Force) {
		codeOnly := !cfg.Graph.Semantic
		if _, err := graph.Build(root, paths, codeOnly, cfg.Graph.SemanticBackend); err != nil {
			return fmt.Errorf("graph: %w", err)
		}
		builtGraph = true
	}

	return saveRefreshMarker(paths, refreshMarker{
		SHA: sha, At: time.Now().UTC(), GraphBuilt: builtGraph,
	})
}

func gitSHA(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// nonIndexablePrefixes are paths Graphify never indexes for code structure, so
// a commit that only touches these should not trigger a graph rebuild.
var nonIndexablePrefixes = []string{".so/", ".git/"}

// nonIndexableExts are file types that don't change code structure/edges even
// when they legitimately change (docs, lockfiles, generated data).
var nonIndexableExts = map[string]bool{
	".md": true, ".txt": true, ".lock": true,
}

func isIndexablePath(p string) bool {
	for _, prefix := range nonIndexablePrefixes {
		if strings.HasPrefix(p, prefix) {
			return false
		}
	}
	base := filepath.Base(p)
	if base == "package-lock.json" || base == "go.sum" {
		return false
	}
	return !nonIndexableExts[filepath.Ext(p)]
}

// indexableFilesChanged reports whether any file Graphify would actually
// index changed between two commits. Fails open (true) on any git error so a
// diff failure never silently suppresses a real rebuild.
func indexableFilesChanged(root, fromSHA, toSHA string) bool {
	if fromSHA == "" || toSHA == "" || fromSHA == toSHA {
		return true
	}
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", fromSHA, toSHA).Output()
	if err != nil {
		return true
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isIndexablePath(line) {
			return true
		}
	}
	return false
}

func loadRefreshMarker(paths harness.Paths) refreshMarker {
	data, err := os.ReadFile(refreshMarkerPath(paths))
	if err != nil {
		return refreshMarker{}
	}
	var m refreshMarker
	_ = json.Unmarshal(data, &m)
	return m
}

func saveRefreshMarker(paths harness.Paths, m refreshMarker) error {
	_ = os.MkdirAll(paths.MemoryDir, 0o755)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(refreshMarkerPath(paths), data, 0o644)
}

func sharedHarnessChanged(paths harness.Paths, since time.Time) bool {
	if since.IsZero() {
		return true
	}
	dirs := []string{
		paths.KnowledgeDir, paths.RulesDir, paths.SkillsDir,
		paths.GuardrailsDir, paths.EvalsDir,
	}
	for _, d := range dirs {
		changed := false
		_ = filepath.Walk(d, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if info.ModTime().After(since) {
				changed = true
				return filepath.SkipAll
			}
			return nil
		})
		if changed {
			return true
		}
	}
	return false
}
