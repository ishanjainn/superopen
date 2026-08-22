package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type SyntaxParser interface {
	Parse(context.Context, string, []byte) (SyntaxNode, error)
}

type ParsedSyntaxFile struct {
	File       FileRecord
	Detection  DetectedLanguage
	Extraction FileResult
	Body       []byte
}

type SyntaxRepository struct {
	Files      []ParsedSyntaxFile
	Coverage   api.Coverage
	Generation string
	Root       string
	GoModule   string
}

type syntaxOutcome struct {
	file       *ParsedSyntaxFile
	generation string
	coverage   *api.CoverageRow
	err        error
}

// ParseSyntaxRepository performs deterministic discovery-to-extraction work
// for every pinned grammar. It intentionally stops before family-specific
// resolution so individual pipeline passes can be compared with golden fixtures.
func ParseSyntaxRepository(ctx context.Context, parser SyntaxParser, root, project string, files []string, overrides map[string]string, workers int) (SyntaxRepository, error) {
	if parser == nil {
		return SyntaxRepository{}, errors.New("syntax parser is required")
	}
	if workers < 1 {
		workers = 1
	}
	if workers > maxParseWorkers {
		workers = maxParseWorkers
	}
	discoveryPosition := make(map[string]int, len(files))
	for index, rel := range files {
		discoveryPosition[filepath.ToSlash(rel)] = index
	}
	tasks := make(chan string)
	results := make(chan syntaxOutcome, workers*2)
	var parsedCount atomic.Int64
	total := int64(len(files))
	factory, _ := parser.(parseSessionFactory)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			local := parser
			if factory != nil {
				session := factory.NewParseSession(ctx)
				defer session.Close(ctx)
				local = session
			}
			for rel := range tasks {
				outcome := parseSyntaxFile(ctx, local, root, project, rel, overrides)
				reclaimAfterParse()
				done := parsedCount.Add(1)
				if lang := outcomeLanguage(outcome); lang != "" && (done%10 == 0 || done == total) {
					reportIndexProgress("parse %d/%d %s", done, total, lang)
				}
				results <- outcome
			}
		}()
	}
	go func() {
		defer close(tasks)
		for _, rel := range files {
			select {
			case tasks <- rel:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		group.Wait()
		close(results)
	}()

	repository := SyntaxRepository{Root: root, GoModule: readGoModulePath(root), Coverage: api.Coverage{IndexMode: parserIndexMode(parser), RecordingStatus: "complete"}}
	var generationParts []string
	for result := range results {
		if result.err != nil {
			return SyntaxRepository{}, result.err
		}
		if result.file != nil {
			repository.Files = append(repository.Files, *result.file)
			generationParts = append(generationParts, result.generation)
		}
		if result.coverage != nil {
			repository.Coverage.Rows = append(repository.Coverage.Rows, *result.coverage)
		}
	}
	if err := ctx.Err(); err != nil {
		return SyntaxRepository{}, err
	}
	// Graph insertion and collision behavior is observably tied to Superopen's
	// raw discovery order. Parsing may complete in any worker order, so restore
	// the input positions rather than alphabetically reordering the files.
	sort.SliceStable(repository.Files, func(i, j int) bool {
		return discoveryPosition[repository.Files[i].File.Path] < discoveryPosition[repository.Files[j].File.Path]
	})
	sort.Slice(repository.Coverage.Rows, func(i, j int) bool {
		if repository.Coverage.Rows[i].Path != repository.Coverage.Rows[j].Path {
			return repository.Coverage.Rows[i].Path < repository.Coverage.Rows[j].Path
		}
		return repository.Coverage.Rows[i].Kind < repository.Coverage.Rows[j].Kind
	})
	sort.Strings(generationParts)
	generation := sha256.Sum256([]byte(strings.Join(generationParts, "\n")))
	repository.Generation = hex.EncodeToString(generation[:])
	repository.Coverage.Generation = repository.Generation
	now := time.Now().UTC()
	repository.Coverage.RecordedAt = &now
	repository.Coverage.HashRecordsComplete = len(repository.Files) > 0 || len(files) == 0
	if len(repository.Coverage.Rows) == 0 {
		repository.Coverage.Status = "complete"
	} else {
		repository.Coverage.Status = "partial"
		repository.Coverage.RecordingStatus = "truncated"
	}
	return repository, nil
}

func readGoModulePath(root string) string {
	body, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func parseSyntaxFile(ctx context.Context, parser SyntaxParser, root, project, rel string, overrides map[string]string) syntaxOutcome {
	if err := ctx.Err(); err != nil {
		return syntaxOutcome{err: err}
	}
	if _, ok := DetectLanguage(rel, nil, overrides); !ok {
		return syntaxOutcome{}
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	body, err := os.ReadFile(abs)
	if err != nil {
		return syntaxOutcome{coverage: &api.CoverageRow{Path: filepath.ToSlash(rel), Kind: "read", Detail: err.Error()}}
	}
	detection, ok := DetectLanguage(rel, body, overrides)
	if !ok {
		return syntaxOutcome{}
	}
	record := FileRecord{
		Project: project, Path: filepath.ToSlash(rel), SHA256: fileContentDigest(body),
		Size: int64(len(body)), Language: detection.Language, LineCount: countBodyLines(body),
	}
	if info, statErr := os.Stat(abs); statErr == nil {
		record.MTimeNS = info.ModTime().UnixNano()
		record.Size = info.Size()
	}
	generation := record.Path + ":" + record.SHA256
	if detection.Grammar == "" {
		if detection.Language == "objectscript_export" {
			return parseObjectScriptExport(ctx, parser, record, detection, generation, body)
		}
		return syntaxOutcome{file: &ParsedSyntaxFile{File: record, Detection: detection}, generation: generation,
			coverage: &api.CoverageRow{Path: record.Path, Kind: "transform", Detail: "container has no registered transform"}}
	}
	extraction, err := extractSyntaxFile(ctx, parser, detection.Language, detection.Grammar, body)
	if err == nil {
		extraction = enrichCFamilyExtract(ctx, parser, detection.Language, detection.Grammar, record.Path, body, extraction)
	}
	if err != nil {
		if ctx.Err() != nil {
			return syntaxOutcome{err: ctx.Err()}
		}
		return syntaxOutcome{
			file: &ParsedSyntaxFile{File: record, Detection: detection}, generation: generation,
			coverage: &api.CoverageRow{Path: record.Path, Kind: "parse", Detail: err.Error()},
		}
	}
	parsed := &ParsedSyntaxFile{File: record, Detection: detection, Extraction: extraction, Body: body}
	result := syntaxOutcome{file: parsed, generation: generation}
	if extraction.Partial {
		result.coverage = &api.CoverageRow{Path: record.Path, Kind: "parse_partial", Detail: "Tree-sitter recovered a partial syntax tree"}
	}
	return result
}

func extractSyntaxFile(ctx context.Context, parser SyntaxParser, language, grammar string, body []byte) (FileResult, error) {
	if extractor, ok := parser.(factExtractor); ok {
		return extractor.ExtractFacts(ctx, language, grammar, body)
	}
	tree, err := parser.Parse(ctx, grammar, body)
	if err != nil {
		return FileResult{}, err
	}
	return ExtractSyntaxFacts(language, tree, body)
}

func parseObjectScriptExport(ctx context.Context, parser SyntaxParser, record FileRecord, detection DetectedLanguage, generation string, body []byte) syntaxOutcome {
	classes := TranscodeObjectScriptExport(body)
	if len(classes) == 0 {
		return syntaxOutcome{file: &ParsedSyntaxFile{File: record, Detection: detection}, generation: generation,
			coverage: &api.CoverageRow{Path: record.Path, Kind: "transform", Detail: "ObjectScript export contained no complete classes"}}
	}
	combined := FileResult{ParseStatus: ParseStatus{Parsed: true}}
	for _, udl := range classes {
		tree, err := parser.Parse(ctx, "objectscript_udl", []byte(udl))
		if err != nil {
			if ctx.Err() != nil {
				return syntaxOutcome{err: ctx.Err()}
			}
			return syntaxOutcome{file: &ParsedSyntaxFile{File: record, Detection: detection}, generation: generation,
				coverage: &api.CoverageRow{Path: record.Path, Kind: "parse", Detail: err.Error()}}
		}
		extraction, err := ExtractSyntaxFacts("objectscript_udl", tree, []byte(udl))
		if err != nil {
			return syntaxOutcome{file: &ParsedSyntaxFile{File: record, Detection: detection}, generation: generation,
				coverage: &api.CoverageRow{Path: record.Path, Kind: "extract", Detail: err.Error()}}
		}
		combined.Definitions = append(combined.Definitions, extraction.Definitions...)
		combined.Calls = append(combined.Calls, extraction.Calls...)
		combined.Usages = append(combined.Usages, extraction.Usages...)
		combined.Writes = append(combined.Writes, extraction.Writes...)
		combined.Bindings = append(combined.Bindings, extraction.Bindings...)
		combined.Imports = append(combined.Imports, extraction.Imports...)
		combined.Sections = append(combined.Sections, extraction.Sections...)
		combined.Branches = append(combined.Branches, extraction.Branches...)
		combined.Throws = append(combined.Throws, extraction.Throws...)
		combined.Partial = combined.Partial || extraction.Partial
	}
	sortSyntaxFacts(&combined)
	parsed := &ParsedSyntaxFile{File: record, Detection: detection, Extraction: combined, Body: body}
	result := syntaxOutcome{file: parsed, generation: generation}
	if combined.Partial {
		result.coverage = &api.CoverageRow{Path: record.Path, Kind: "parse_partial", Detail: "Tree-sitter recovered a partial transcoded syntax tree"}
	}
	return result
}

func outcomeLanguage(outcome syntaxOutcome) string {
	if outcome.file != nil && outcome.file.File.Language != "" {
		return outcome.file.File.Language
	}
	return ""
}
