package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
)

type ReflectOptions struct {
	IfStale          bool
	HalfLifeDays     float64
	MinCorroboration int
}

func ReflectOutcomes(paths harness.Paths, outcomes []memory.GraphOutcome) error {
	return ReflectOutcomesWithOptions(paths, outcomes, ReflectOptions{})
}

func ReflectOutcomesWithOptions(paths harness.Paths, outcomes []memory.GraphOutcome, opts ReflectOptions) error {
	if err := os.MkdirAll(filepath.Join(paths.GraphDir, "reflections"), 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(paths.GraphDir, ".reflection-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	for _, o := range outcomes {
		args := []string{"save-result", "--question", o.Question, "--answer", o.AnswerSummary, "--outcome", o.Outcome, "--memory-dir", work}
		if o.QueryType != "" {
			args = append(args, "--type", o.QueryType)
		}
		if len(o.SourceNodes) > 0 {
			args = append(args, "--nodes")
			args = append(args, o.SourceNodes...)
		}
		if o.Correction != "" {
			args = append(args, "--correction", o.Correction)
		}
		if _, err := Run(context.Background(), paths.RepoRoot, args...); err != nil {
			return err
		}
	}
	lessonsPath := filepath.Join(paths.GraphDir, "reflections", "LESSONS.md")
	if len(outcomes) > 0 {
		args := []string{"reflect", "--memory-dir", work, "--out", lessonsPath, "--graph", paths.GraphJSON}
		if opts.IfStale {
			args = append(args, "--if-stale")
		}
		if opts.HalfLifeDays > 0 {
			args = append(args, "--half-life-days", fmt.Sprint(opts.HalfLifeDays))
		}
		if opts.MinCorroboration > 0 {
			args = append(args, "--min-corroboration", fmt.Sprint(opts.MinCorroboration))
		}
		if _, err := Run(context.Background(), paths.RepoRoot, args...); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(lessonsPath, []byte("# Graph lessons\n\nNo recorded graph outcomes.\n"), 0o644); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(map[string]any{"outcomes": outcomes, "engine": "graphify", "engine_version": PinnedVersion}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(paths.GraphDir, ".graphify_learning.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	lessons, _ := os.ReadFile(lessonsPath)
	compact := string(lessons)
	if len(compact) > 4000 {
		compact = compact[:4000]
	}
	active, _ := os.ReadFile(paths.MemoryActive)
	marker := "\n## Graph lessons\n\n"
	text := string(active)
	if i := strings.Index(text, marker); i >= 0 {
		text = text[:i]
	}
	return os.WriteFile(paths.MemoryActive, []byte(text+marker+compact), 0o644)
}
