package engine

import (
	"strings"
	"testing"

	"rsc.io/pdf"
)

func TestPDFPageDictionariesExcludePageTree(t *testing.T) {
	graph := goGraph{}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/flow.pdf", Language: "pdf"}, Body: []byte("%PDF-1.4\n/Type /Pages /Count 1\n/Type /Page /Parent 2 0 R\n")}}
	indexPDFPages(files, &graph)
	if len(graph.nodes) != 1 || graph.nodes[0].Name != "page 1" {
		t.Fatalf("page nodes: %+v", graph.nodes)
	}
}

func TestPDFRotatedGlyphsStayOneLabel(t *testing.T) {
	items := []pdf.Text{{S: "p", X: 53, Y: 271, W: 0, FontSize: 0}, {S: "r", X: 53, Y: 264, W: 0, FontSize: 0}, {S: "o", X: 53, Y: 257, W: 0, FontSize: 0}}
	if text := formatPDFText(items); !strings.Contains(text, "pro") {
		t.Fatalf("rotated label: %q", text)
	}
}
