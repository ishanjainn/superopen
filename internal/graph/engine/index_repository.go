package engine

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// gitContextProperties matches Superopen engine helper so a Branch node
// carries the same worktree identity that the pinned engine records.
func gitContextProperties(ctx context.Context, root, branch string) api.Properties {
	capture := func(args ...string) string {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		output, err := command.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(output))
	}
	worktreeRoot := capture("rev-parse", "--show-toplevel")
	gitDir := capture("rev-parse", "--git-dir")
	commonDir := capture("rev-parse", "--git-common-dir")
	absoluteCommonDir := capture("rev-parse", "--path-format=absolute", "--git-common-dir")
	canonicalRoot := worktreeRoot
	// A linked worktree canonicalizes onto the primary checkout so every
	// worktree of one repository shares a single graph identity.
	if gitDir != commonDir && absoluteCommonDir != "" {
		canonicalRoot = strings.TrimSuffix(strings.TrimSuffix(absoluteCommonDir, "/"), "/.git")
	}
	if canonicalRoot == "" {
		canonicalRoot = root
	}
	_, statErr := os.Stat(root)
	return api.Properties{
		"is_git": worktreeRoot != "", "is_worktree": gitDir != "" && commonDir != "" && gitDir != commonDir,
		"is_detached": capture("symbolic-ref", "--quiet", "--short", "HEAD") == "", "root_exists": statErr == nil,
		"canonical_root": canonicalRoot, "worktree_root": worktreeRoot, "git_common_dir": commonDir,
		"branch": branch, "head_sha": capture("rev-parse", "--verify", "HEAD"),
		"base_sha": capture("merge-base", "HEAD", "@{upstream}"),
	}
}

func indexRepositoryBranch(ctx context.Context, root, project string, graph *goGraph) {
	command := exec.CommandContext(ctx, "git", "-C", root, "branch", "--show-current")
	output, err := command.Output()
	branch := strings.TrimSpace(string(output))
	if err != nil || branch == "" {
		return
	}
	slug := gitBranchSlug(branch, false)
	qn := "__branch__." + slug
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Branch", Name: branch, QualifiedName: qn,
		Location: api.Location{File: "{}"}, Properties: gitContextProperties(ctx, root, branch)})
	graph.edges = append(graph.edges, pendingEdge{source: project, target: qn, kind: "HAS_BRANCH",
		evidence: &api.Evidence{Strategy: "git_branch", Confidence: 1}})
	// The pinned repository model roots every top-level folder and file at the
	// current branch rather than directly at Project. Rewire existing layout
	// edges and synthesize any top-level folder edge lost to a language-family
	// replacement.
	topLevel := map[string]bool{}
	for _, node := range graph.nodes {
		if node.Label == "Folder" && node.Location.File != "" && !strings.Contains(node.Location.File, "/") {
			topLevel[node.QualifiedName] = true
		}
	}
	for index := range graph.edges {
		edge := &graph.edges[index]
		if edge.kind == "CONTAINS_FOLDER" && edge.source == project && topLevel[edge.target] {
			edge.source = qn
			delete(topLevel, edge.target)
		} else if edge.kind == "CONTAINS_FILE" && edge.source == project {
			edge.source = qn
		}
	}
	for target := range topLevel {
		graph.edges = append(graph.edges, pendingEdge{source: qn, target: target, kind: "CONTAINS_FOLDER",
			evidence: &api.Evidence{Strategy: "repository_layout", Confidence: 1}})
	}
}

// gitBranchSlug matches Superopen slug_from_branch: non-alnum runes collapse to
// a single dash so feat/foo and feat foo share one Branch identity.
func gitBranchSlug(branch string, detached bool) string {
	fallback := "working-tree"
	if detached {
		fallback = "detached"
	}
	src := branch
	if detached || src == "" {
		src = fallback
	}
	var slug strings.Builder
	inDash := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.'
		if alnum {
			if slug.Len() == 0 && c == '-' {
				inDash = true
				continue
			}
			slug.WriteByte(c)
			inDash = false
			continue
		}
		if slug.Len() > 0 && !inDash {
			slug.WriteByte('-')
			inDash = true
		}
	}
	result := strings.TrimRight(slug.String(), "-")
	if result == "" {
		return fallback
	}
	return result
}
