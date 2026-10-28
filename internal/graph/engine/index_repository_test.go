package engine

import (
	"context"
	"os/exec"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestDetachedRepositoryKeepsBranchNode(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q", root}, {"-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "initial"}, {"-C", root, "checkout", "-q", "--detach", "HEAD"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	graph := goGraph{nodes: []api.Node{{Label: "Project", Name: "repo", QualifiedName: "repo"}}}
	indexRepositoryBranch(context.Background(), root, "repo", &graph)
	if len(graph.nodes) != 2 || graph.nodes[1].Label != "Branch" || graph.nodes[1].Name != "DETACHED" || len(graph.edges) != 1 || graph.edges[0].kind != "HAS_BRANCH" {
		t.Fatalf("detached branch: nodes=%+v edges=%+v", graph.nodes, graph.edges)
	}
}
