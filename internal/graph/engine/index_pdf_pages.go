package engine

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"rsc.io/pdf"
)

var pdfPageDictionary = regexp.MustCompile(`/Type\s*/Page\b`)

// Page dictionaries are the PDF page tree's leaf objects. Text-layer parsing
// is separate; an image-only page still has an addressable Section.
func indexPDFPages(files []ParsedSyntaxFile, graph *goGraph) {
	for _, parsed := range files {
		if parsed.File.Language != "pdf" {
			continue
		}
		file := filepath.ToSlash(parsed.File.Path)
		pages := pdfPageDictionary.FindAllIndex(parsed.Body, 10000)
		reader := openPDFReader(parsed.Body)
		if reader != nil {
			if reader.NumPage() > len(pages) {
				pages = make([][]int, reader.NumPage())
			}
		}
		for i := range pages {
			page := i + 1
			qn := syntaxModuleQN(file) + fmt.Sprintf(".page_%d", page)
			properties := api.Properties{"page": page}
			if reader != nil && page <= reader.NumPage() {
				if body := pdfPageText(reader, page); body != "" {
					properties["docstring"] = body
				}
			}
			graph.nodes = append(graph.nodes, api.Node{Project: parsed.File.Project, Label: "Section", Name: fmt.Sprintf("page %d", page),
				QualifiedName: qn, Location: api.Location{File: file, StartLine: page, EndLine: page}, Properties: properties})
			graph.addEdge(pendingEdge{source: fileQualifiedName(file), target: qn, kind: "DEFINES"})
		}
	}
}

func openPDFReader(body []byte) (reader *pdf.Reader) {
	defer func() {
		if recover() != nil {
			reader = nil
		}
	}()
	parsed, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil || parsed.NumPage() < 1 || parsed.NumPage() > 10000 {
		return nil
	}
	return parsed
}

func pdfPageText(reader *pdf.Reader, page int) (text string) {
	defer func() {
		if recover() != nil {
			text = ""
		}
	}()
	return formatPDFText(reader.Page(page).Content().Text)
}

func formatPDFText(items []pdf.Text) string {
	var body strings.Builder
	var lastY, lastX, lastSize float64
	lastVertical := false
	for _, item := range items {
		if item.S == "" {
			continue
		}
		vertical := item.W == 0 && item.FontSize == 0
		if body.Len() > 0 {
			if vertical && lastVertical && math.Abs(item.X-lastX) < 1 && math.Abs(item.Y-lastY) < 12 {
				// The PDF reader emits rotated labels one glyph at a time.
			} else if math.Abs(item.Y-lastY) > math.Max(1, lastSize*0.4) {
				body.WriteByte('\n')
			} else if item.X-lastX > 1 {
				body.WriteByte(' ')
			}
		}
		if body.Len()+len(item.S) > 256*1024 {
			break
		}
		body.WriteString(item.S)
		lastY, lastX, lastSize = item.Y, item.X+item.W, item.FontSize
		lastVertical = vertical
	}
	return body.String()
}
