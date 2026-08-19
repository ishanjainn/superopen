package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
)

type GrammarAsset struct {
	Language           string `json:"language"`
	File               string `json:"file"`
	Compression        string `json:"compression"`
	SHA256             string `json:"sha256"`
	Size               int64  `json:"size"`
	UncompressedSHA256 string `json:"uncompressed_sha256"`
	UncompressedSize   int64  `json:"uncompressed_size"`
}

type GrammarManifest struct {
	Format        int            `json:"format"`
	AssetRevision string         `json:"asset_revision"`
	TreeSitter    string         `json:"tree_sitter"`
	Target        string         `json:"target"`
	Assets        []GrammarAsset `json:"assets"`
}

// LoadGrammarAssets verifies every byte before passing modules to wazero. A
// complete load is all-or-nothing: callers never receive a runtime whose
// manifest silently omitted a pinned grammar.
func LoadGrammarAssets(ctx context.Context, files fs.FS, manifestPath string) (*GrammarRuntime, GrammarManifest, error) {
	return loadGrammarAssets(ctx, files, manifestPath, true)
}

func loadGrammarAssets(ctx context.Context, files fs.FS, manifestPath string, requireComplete bool) (*GrammarRuntime, GrammarManifest, error) {
	return loadSelectedGrammarAssets(ctx, files, manifestPath, requireComplete, nil)
}

func loadSelectedGrammarAssets(ctx context.Context, files fs.FS, manifestPath string, requireComplete bool, only []string) (*GrammarRuntime, GrammarManifest, error) {
	manifestBytes, err := fs.ReadFile(files, manifestPath)
	if err != nil {
		return nil, GrammarManifest{}, err
	}
	var manifest GrammarManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, GrammarManifest{}, fmt.Errorf("decode grammar manifest: %w", err)
	}
	if err := validateGrammarManifest(manifest, requireComplete); err != nil {
		return nil, GrammarManifest{}, err
	}
	allow := map[string]bool{}
	for _, language := range only {
		if !knownLanguage(language) {
			return nil, GrammarManifest{}, fmt.Errorf("unknown pinned grammar %q", language)
		}
		allow[language] = true
	}
	runtime := NewGrammarRuntime(ctx)
	directory := path.Dir(manifestPath)
	for _, item := range manifest.Assets {
		if len(allow) > 0 && !allow[item.Language] {
			continue
		}
		body, err := fs.ReadFile(files, path.Join(directory, item.File))
		if err != nil {
			runtime.Close(ctx)
			return nil, GrammarManifest{}, fmt.Errorf("read grammar %s: %w", item.Language, err)
		}
		digest := sha256.Sum256(body)
		if int64(len(body)) != item.Size || hex.EncodeToString(digest[:]) != item.SHA256 {
			runtime.Close(ctx)
			return nil, GrammarManifest{}, fmt.Errorf("grammar %s failed content verification", item.Language)
		}
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			runtime.Close(ctx)
			return nil, GrammarManifest{}, fmt.Errorf("decompress grammar %s: %w", item.Language, err)
		}
		wasm, err := io.ReadAll(io.LimitReader(reader, item.UncompressedSize+1))
		closeErr := reader.Close()
		if err != nil || closeErr != nil || int64(len(wasm)) != item.UncompressedSize {
			runtime.Close(ctx)
			return nil, GrammarManifest{}, fmt.Errorf("decompress grammar %s: size or stream mismatch", item.Language)
		}
		wasmDigest := sha256.Sum256(wasm)
		if hex.EncodeToString(wasmDigest[:]) != item.UncompressedSHA256 {
			runtime.Close(ctx)
			return nil, GrammarManifest{}, fmt.Errorf("grammar %s failed uncompressed verification", item.Language)
		}
		if err := runtime.Compile(ctx, item.Language, wasm); err != nil {
			runtime.Close(ctx)
			return nil, GrammarManifest{}, err
		}
	}
	if requireComplete && !runtime.Complete() {
		runtime.Close(ctx)
		return nil, GrammarManifest{}, fmt.Errorf("grammar runtime loaded %d modules, require %d", runtime.Count(), len(Languages))
	}
	if len(allow) > 0 && runtime.Count() != len(allow) {
		runtime.Close(ctx)
		return nil, GrammarManifest{}, fmt.Errorf("grammar runtime loaded %d modules, require %d", runtime.Count(), len(allow))
	}
	return runtime, manifest, nil
}

func validateGrammarManifest(manifest GrammarManifest, complete bool) error {
	if manifest.Format != 2 || manifest.AssetRevision != AssetRevision || manifest.TreeSitter != "0.24.4" || manifest.Target != "wasm32-wasip1" {
		return fmt.Errorf("incompatible grammar manifest metadata")
	}
	seen := map[string]bool{}
	prior := ""
	for _, item := range manifest.Assets {
		if !knownLanguage(item.Language) || seen[item.Language] {
			return fmt.Errorf("invalid or duplicate grammar %q", item.Language)
		}
		if item.Language < prior {
			return fmt.Errorf("grammar manifest is not deterministically sorted")
		}
		if path.Base(item.File) != item.File || !strings.HasSuffix(item.File, ".wasm.gz") || item.Compression != "gzip" || item.Size <= 0 || item.UncompressedSize <= 0 || len(item.SHA256) != 64 || len(item.UncompressedSHA256) != 64 {
			return fmt.Errorf("invalid asset metadata for grammar %s", item.Language)
		}
		if _, err := hex.DecodeString(item.SHA256); err != nil {
			return fmt.Errorf("invalid digest for grammar %s", item.Language)
		}
		if _, err := hex.DecodeString(item.UncompressedSHA256); err != nil {
			return fmt.Errorf("invalid uncompressed digest for grammar %s", item.Language)
		}
		seen[item.Language] = true
		prior = item.Language
	}
	if complete {
		var missing []string
		for _, language := range Languages {
			if !seen[language] {
				missing = append(missing, language)
			}
		}
		if len(missing) > 0 || len(seen) != len(Languages) {
			sort.Strings(missing)
			return fmt.Errorf("incomplete grammar manifest: %d/%d present; missing %v", len(seen), len(Languages), missing)
		}
	}
	return nil
}
