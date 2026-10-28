package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHTMLInlineScriptsKeepHostCoordinatesAndSkipDataBlocks(t *testing.T) {
	root := t.TempDir()
	body := []byte("<h1>Title</h1>\n<script>\nfunction toggle() { return true; }\n</script>\n<script type=\"application/json\">function ignored() {}</script>\n<script src=\"external.js\">function remote() {}</script>\n<script lang=\"ts\">function typed(x: number) { return x; }</script>\n")
	if err := os.WriteFile(filepath.Join(root, "page.html"), body, 0600); err != nil {
		t.Fatal(err)
	}
	result := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "page.html", nil)
	if result.err != nil || result.file == nil || result.coverage != nil {
		t.Fatalf("parse: %+v", result)
	}
	found := map[string]int{}
	for _, fact := range result.file.Extraction.Definitions {
		found[fact.Name] = fact.StartLine
	}
	if found["toggle"] != 3 || found["typed"] != 7 {
		t.Fatalf("inline functions at wrong lines: %v", found)
	}
	if _, ok := found["ignored"]; ok {
		t.Fatalf("data script became code: %v", found)
	}
	if _, ok := found["remote"]; ok {
		t.Fatalf("external script became inline code: %v", found)
	}
}
