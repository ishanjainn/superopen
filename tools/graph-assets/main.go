// Command graph-assets reproducibly compiles the pinned generated
// Tree-sitter grammars into combined WASI parser modules. It is a development
// tool and is not included in Superopen release archives.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/engine"
)

func main() {
	var sourceTree, sdk, output, selected, optimization, reuse, grammarDir, runtimeDir string
	var workers int
	var compileTimeout time.Duration
	flag.StringVar(&sourceTree, "source-tree", os.Getenv("SUPEROPEN_GRAPH_SOURCE"), "grammar source tree (or SUPEROPEN_GRAPH_SOURCE)")
	flag.StringVar(&grammarDir, "grammar-dir", "grammars", "grammar wrapper dir relative to source-tree")
	flag.StringVar(&runtimeDir, "runtime-dir", "vendored/ts_runtime", "tree-sitter runtime dir relative to source-tree")
	flag.StringVar(&sdk, "wasi-sdk", "", "WASI SDK root containing bin/clang")
	flag.StringVar(&output, "out", "", "output directory")
	flag.StringVar(&selected, "languages", "", "comma-separated language subset (default all)")
	flag.StringVar(&optimization, "optimization", "1", "LLVM optimization level: 0, 1, 2, s, or z")
	flag.StringVar(&reuse, "reuse", "", "verified complete grammar bundle to seed into the output")
	flag.IntVar(&workers, "workers", runtime.NumCPU(), "parallel compiler processes")
	flag.DurationVar(&compileTimeout, "compile-timeout", 5*time.Minute, "maximum compile time per grammar")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, sourceTree, sdk, output, selected, optimization, reuse, grammarDir, runtimeDir, workers, compileTimeout); err != nil {
		fmt.Fprintln(os.Stderr, "graph-assets:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, sourceTree, sdk, output, selected, optimization, reuse, grammarDir, runtimeDir string, workers int, compileTimeout time.Duration) error {
	if sourceTree == "" || sdk == "" || output == "" {
		return errors.New("-source-tree, -wasi-sdk, and -out are required")
	}
	root, err := filepath.Abs(sourceTree)
	if err != nil {
		return err
	}
	commit, err := commandOutput(ctx, "git", "-C", root, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve source-tree commit: %w", err)
	}
	if commit != engine.AssetRevision {
		return fmt.Errorf("source-tree HEAD is %s, require asset revision %s", commit, engine.AssetRevision)
	}
	clang := filepath.Join(sdk, "bin", "clang")
	if info, err := os.Stat(clang); err != nil || info.IsDir() {
		return fmt.Errorf("WASI clang not found at %s", clang)
	}
	languages, err := selectedLanguages(selected)
	if err != nil {
		return err
	}
	if workers < 1 {
		workers = 1
	}
	if !map[string]bool{"0": true, "1": true, "2": true, "s": true, "z": true}[optimization] {
		return fmt.Errorf("invalid optimization level %q", optimization)
	}
	if compileTimeout <= 0 {
		return errors.New("compile timeout must be positive")
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	if reuse != "" {
		if err := seedGrammarAssets(ctx, reuse, output, languages); err != nil {
			return err
		}
	}
	temp, err := os.MkdirTemp(output, ".build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	grammarRoot := filepath.Join(root, grammarDir)
	runtimeRoot := filepath.Join(root, runtimeDir)
	runtimeObject, err := compileTreeSitterRuntime(ctx, clang, runtimeRoot, temp)
	if err != nil {
		return err
	}

	tasks := make(chan string)
	results := make(chan engine.GrammarAsset, len(languages))
	errorsOut := make(chan error, len(languages))
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for language := range tasks {
				compileCtx, cancel := context.WithTimeout(ctx, compileTimeout)
				built, err := compileGrammar(compileCtx, clang, root, grammarRoot, runtimeRoot, temp, output, runtimeObject, language, optimization)
				cancel()
				if err != nil {
					errorsOut <- err
					continue
				}
				results <- built
			}
		}()
	}
	for _, language := range languages {
		tasks <- language
	}
	close(tasks)
	group.Wait()
	close(results)
	close(errorsOut)
	if len(errorsOut) > 0 {
		var messages []string
		for err := range errorsOut {
			messages = append(messages, err.Error())
		}
		sort.Strings(messages)
		return errors.New(strings.Join(messages, "\n"))
	}
	result := engine.GrammarManifest{Format: 2, AssetRevision: commit, TreeSitter: "0.24.4", Target: "wasm32-wasip1"}
	for item := range results {
		result.Assets = append(result.Assets, item)
	}
	sort.Slice(result.Assets, func(i, j int) bool { return result.Assets[i].Language < result.Assets[j].Language })
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := atomicWrite(filepath.Join(output, "manifest.json"), body, 0o644); err != nil {
		return err
	}
	if len(languages) == len(engine.Languages) {
		grammarRuntime, _, err := engine.LoadGrammarAssets(ctx, os.DirFS(output), "manifest.json")
		if err != nil {
			return fmt.Errorf("verify complete grammar bundle: %w", err)
		}
		if err := grammarRuntime.Close(ctx); err != nil {
			return fmt.Errorf("close complete grammar bundle: %w", err)
		}
	}
	return nil
}

func seedGrammarAssets(ctx context.Context, source, output string, languages []string) error {
	runtime, manifest, err := engine.LoadGrammarAssets(ctx, os.DirFS(source), "manifest.json")
	if err != nil {
		return fmt.Errorf("verify reuse bundle: %w", err)
	}
	if err := runtime.Close(ctx); err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, language := range languages {
		wanted[language] = true
	}
	for _, asset := range manifest.Assets {
		if !wanted[asset.Language] {
			continue
		}
		body, err := os.ReadFile(filepath.Join(source, asset.File))
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(output, asset.File), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func selectedLanguages(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return append([]string(nil), engine.Languages...), nil
	}
	seen := map[string]bool{}
	var result []string
	for _, language := range strings.Split(value, ",") {
		language = strings.TrimSpace(language)
		if _, ok := engine.GrammarExport(language); !ok {
			return nil, fmt.Errorf("unknown language %q", language)
		}
		if !seen[language] {
			seen[language] = true
			result = append(result, language)
		}
	}
	sort.Strings(result)
	return result, nil
}

func compileGrammar(ctx context.Context, clang, sourceRoot, grammarRoot, runtimeRoot, temp, output, runtimeObject, language, optimization string) (engine.GrammarAsset, error) {
	if existing, ok := existingAsset(ctx, output, language); ok {
		return existing, nil
	}
	export, _ := engine.GrammarExport(language)
	bridge := []byte(strings.ReplaceAll(parserBridge, "TREE_SITTER_LANGUAGE", export))
	bridgePath := filepath.Join(temp, language+"-bridge.c")
	if err := os.WriteFile(bridgePath, bridge, 0o600); err != nil {
		return engine.GrammarAsset{}, err
	}
	wrapper := filepath.Join(grammarRoot, "grammar_"+language+".c")
	if _, err := os.Stat(wrapper); err != nil {
		return engine.GrammarAsset{}, fmt.Errorf("%s: missing grammar wrapper: %w", language, err)
	}
	staging := filepath.Join(temp, language+".wasm")
	common := []string{
		"--target=wasm32-wasip1", "-std=c11",
		"-D_DEFAULT_SOURCE", "-ffunction-sections", "-fdata-sections", "-w",
		"-I", filepath.Join(runtimeRoot, "include"), "-I", filepath.Join(runtimeRoot, "src"), "-I", grammarRoot,
	}
	bridgeObject := filepath.Join(temp, language+"-bridge.o")
	bridgeArgs := append(append([]string(nil), common...), "-O2", "-c", bridgePath, "-o", bridgeObject)
	if outputBytes, err := runCompiler(ctx, clang, bridgeArgs...); err != nil {
		return engine.GrammarAsset{}, fmt.Errorf("%s: compile bridge: %w: %s", language, err, strings.TrimSpace(string(outputBytes)))
	}
	grammarObject := filepath.Join(temp, language+"-grammar.o")
	grammarArgs := append(append([]string(nil), common...), "-O"+optimization, "-c", wrapper, "-o", grammarObject)
	if outputBytes, err := runCompiler(ctx, clang, grammarArgs...); err != nil {
		return engine.GrammarAsset{}, fmt.Errorf("%s: compile grammar: %w: %s", language, err, strings.TrimSpace(string(outputBytes)))
	}
	args := []string{
		"--target=wasm32-wasip1", "-mexec-model=reactor", bridgeObject, runtimeObject, grammarObject,
		"-Wl,--gc-sections", "-Wl,--strip-all", "-Wl,--export=" + export,
	}
	for _, name := range parserExports {
		args = append(args, "-Wl,--export="+name)
	}
	args = append(args, "-o", staging)
	if outputBytes, err := runCompiler(ctx, clang, args...); err != nil {
		return engine.GrammarAsset{}, fmt.Errorf("%s: compile: %w: %s", language, err, strings.TrimSpace(string(outputBytes)))
	}
	body, err := os.ReadFile(staging)
	if err != nil {
		return engine.GrammarAsset{}, err
	}
	grammarRuntime := engine.NewGrammarRuntime(ctx)
	if err := grammarRuntime.Compile(ctx, language, body); err != nil {
		grammarRuntime.Close(ctx)
		return engine.GrammarAsset{}, fmt.Errorf("%s: validate module: %w", language, err)
	}
	rootNode, parseErr := grammarRuntime.Parse(ctx, language, nil)
	closeErr := grammarRuntime.Close(ctx)
	if parseErr != nil || rootNode.Type == "" {
		return engine.GrammarAsset{}, fmt.Errorf("%s: execute parser: %w", language, parseErr)
	}
	if closeErr != nil {
		return engine.GrammarAsset{}, fmt.Errorf("%s: close parser validation runtime: %w", language, closeErr)
	}
	wasmHash := sha256.Sum256(body)
	wasmDigest := hex.EncodeToString(wasmHash[:])
	var compressed bytes.Buffer
	compressor, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return engine.GrammarAsset{}, err
	}
	compressor.Header.OS = 255
	if _, err := compressor.Write(body); err != nil {
		return engine.GrammarAsset{}, err
	}
	if err := compressor.Close(); err != nil {
		return engine.GrammarAsset{}, err
	}
	compressedHash := sha256.Sum256(compressed.Bytes())
	digest := hex.EncodeToString(compressedHash[:])
	name := language + "-" + digest[:16] + ".wasm.gz"
	if err := atomicWrite(filepath.Join(output, name), compressed.Bytes(), 0o644); err != nil {
		return engine.GrammarAsset{}, err
	}
	return engine.GrammarAsset{
		Language: language, File: name, Compression: "gzip", SHA256: digest, Size: int64(compressed.Len()),
		UncompressedSHA256: wasmDigest, UncompressedSize: int64(len(body)),
	}, nil
}

func compileTreeSitterRuntime(ctx context.Context, clang, runtimeRoot, temp string) (string, error) {
	object := filepath.Join(temp, "tree-sitter-runtime.o")
	args := []string{
		"--target=wasm32-wasip1", "-std=c11", "-O2", "-D_DEFAULT_SOURCE",
		"-ffunction-sections", "-fdata-sections", "-w",
		"-I", filepath.Join(runtimeRoot, "include"), "-I", filepath.Join(runtimeRoot, "src"),
		"-c", filepath.Join(runtimeRoot, "src", "lib.c"), "-o", object,
	}
	if output, err := runCompiler(ctx, clang, args...); err != nil {
		return "", fmt.Errorf("compile Tree-sitter runtime: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return object, nil
}

func runCompiler(ctx context.Context, clang string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, clang, args...)
	configureCompilerCommand(command)
	return command.CombinedOutput()
}

func existingAsset(ctx context.Context, output, language string) (engine.GrammarAsset, bool) {
	matches, err := filepath.Glob(filepath.Join(output, language+"-*.wasm.gz"))
	if err != nil || len(matches) == 0 {
		return engine.GrammarAsset{}, false
	}
	sort.Strings(matches)
	for _, candidate := range matches {
		compressed, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			continue
		}
		wasm, readErr := io.ReadAll(io.LimitReader(reader, 128<<20))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || len(wasm) == 0 {
			continue
		}
		grammarRuntime := engine.NewGrammarRuntime(ctx)
		compileErr := grammarRuntime.Compile(ctx, language, wasm)
		root, parseErr := grammarRuntime.Parse(ctx, language, nil)
		_ = grammarRuntime.Close(ctx)
		if compileErr != nil || parseErr != nil || root.Type == "" {
			continue
		}
		compressedHash := sha256.Sum256(compressed)
		wasmHash := sha256.Sum256(wasm)
		return engine.GrammarAsset{
			Language: language, File: filepath.Base(candidate), Compression: "gzip",
			SHA256: hex.EncodeToString(compressedHash[:]), Size: int64(len(compressed)),
			UncompressedSHA256: hex.EncodeToString(wasmHash[:]), UncompressedSize: int64(len(wasm)),
		}, true
	}
	return engine.GrammarAsset{}, false
}

func atomicWrite(path string, body []byte, mode os.FileMode) error {
	temp := fmt.Sprintf("%s.tmp-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(temp, body, mode); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}

var parserExports = []string{
	"so_alloc", "so_free", "so_parse", "so_node_type", "so_node_start_byte", "so_node_end_byte",
	"so_node_child_count", "so_node_child", "so_node_child_field_name", "so_node_is_named", "so_node_has_error",
}

const parserBridge = `
#include <stdint.h>
#include <stdlib.h>
#include "tree_sitter/api.h"
extern const TSLanguage *TREE_SITTER_LANGUAGE(void);
static TSParser *parser;
static TSTree *tree;
__attribute__((visibility("default"))) uint32_t so_alloc(uint32_t n) { return (uint32_t)(uintptr_t)malloc(n ? n : 1); }
__attribute__((visibility("default"))) void so_free(uint32_t p) { free((void *)(uintptr_t)p); }
__attribute__((visibility("default"))) uint32_t so_parse(uint32_t p, uint32_t n) {
  if (!parser) parser = ts_parser_new();
  if (!parser || !ts_parser_set_language(parser, TREE_SITTER_LANGUAGE())) return 0;
  if (tree) ts_tree_delete(tree);
  tree = ts_parser_parse_string(parser, NULL, (const char *)(uintptr_t)p, n);
  if (!tree) return 0;
  TSNode *root = malloc(sizeof(TSNode));
  if (!root) return 0;
  *root = ts_tree_root_node(tree);
  return (uint32_t)(uintptr_t)root;
}
static TSNode *node(uint32_t h) { return (TSNode *)(uintptr_t)h; }
__attribute__((visibility("default"))) uint32_t so_node_type(uint32_t h) { return h ? (uint32_t)(uintptr_t)ts_node_type(*node(h)) : 0; }
__attribute__((visibility("default"))) uint32_t so_node_start_byte(uint32_t h) { return h ? ts_node_start_byte(*node(h)) : 0; }
__attribute__((visibility("default"))) uint32_t so_node_end_byte(uint32_t h) { return h ? ts_node_end_byte(*node(h)) : 0; }
__attribute__((visibility("default"))) uint32_t so_node_child_count(uint32_t h) { return h ? ts_node_child_count(*node(h)) : 0; }
__attribute__((visibility("default"))) uint32_t so_node_child(uint32_t h, uint32_t i) {
  if (!h) return 0;
  TSNode value = ts_node_child(*node(h), i);
  if (ts_node_is_null(value)) return 0;
  TSNode *child = malloc(sizeof(TSNode));
  if (!child) return 0;
  *child = value;
  return (uint32_t)(uintptr_t)child;
}
__attribute__((visibility("default"))) uint32_t so_node_child_field_name(uint32_t h, uint32_t i) {
  return h ? (uint32_t)(uintptr_t)ts_node_field_name_for_child(*node(h), i) : 0;
}
__attribute__((visibility("default"))) uint32_t so_node_is_named(uint32_t h) { return h && ts_node_is_named(*node(h)); }
__attribute__((visibility("default"))) uint32_t so_node_has_error(uint32_t h) { return h && ts_node_has_error(*node(h)); }
`
