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

	_ = guardrails.EnsureDefaults(paths)
	_ = memory.NewStore(paths).Ensure()
	_, _ = memory.NewStore(paths).RefreshActive("")
	_ = inject.Apply(root)
	if _, err := retrieve.Rebuild(root, paths); err != nil {
		return fmt.Errorf("retrieve index: %w", err)
	}
	_ = recommend.MarkStaleFlags(paths)

	builtGraph := false
	if !opts.SkipGraph && (sharedChanged || shaChanged || opts.Force) {
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
