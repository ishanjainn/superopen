package engine

import "testing"

func TestDockerRuntimeIndexesBaseImageAndEntrypoint(t *testing.T) {
	files := []ParsedSyntaxFile{{File: FileRecord{Project: "test", Path: "Dockerfile", Language: "dockerfile"}, Body: []byte("FROM python:3.10\nCMD [\"ignored\"]\nENTRYPOINT [\"pylint\"]\n")}}
	graph := goGraph{}
	indexDockerRuntime(files, &graph)
	counts := map[string]int{}
	for _, node := range graph.nodes {
		counts[node.Label+":"+node.Name]++
	}
	for _, edge := range graph.edges {
		counts[edge.kind+":"+edge.target]++
	}
	for _, key := range []string{"Image:python", "Process:pylint", "DEPENDS_ON:__image__python", "SPAWNS:__process__pylint"} {
		if counts[key] != 1 {
			t.Errorf("missing %s: %+v", key, counts)
		}
	}
}
