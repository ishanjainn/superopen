package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
)

func TestGrammarManifestRequiresExactInventoryAndMetadata(t *testing.T) {
	manifest := GrammarManifest{
		Format: 2, AssetRevision: AssetRevision, TreeSitter: "0.24.4", Target: "wasm32-wasip1",
	}
	for _, language := range Languages {
		manifest.Assets = append(manifest.Assets, GrammarAsset{
			Language: language, File: language + "-0000000000000000.wasm.gz", Compression: "gzip",
			SHA256: strings.Repeat("0", 64), Size: 1, UncompressedSHA256: strings.Repeat("1", 64), UncompressedSize: 1,
		})
	}
	if err := validateGrammarManifest(manifest, true); err != nil {
		t.Fatal(err)
	}
	missing := manifest
	missing.Assets = append([]GrammarAsset(nil), manifest.Assets[:len(manifest.Assets)-1]...)
	if err := validateGrammarManifest(missing, true); err == nil {
		t.Fatal("incomplete inventory accepted")
	}
	unsorted := manifest
	unsorted.Assets = append([]GrammarAsset(nil), manifest.Assets...)
	unsorted.Assets[0], unsorted.Assets[1] = unsorted.Assets[1], unsorted.Assets[0]
	if err := validateGrammarManifest(unsorted, true); err == nil {
		t.Fatal("unsorted manifest accepted")
	}
	escaping := manifest
	escaping.Assets = append([]GrammarAsset(nil), manifest.Assets...)
	escaping.Assets[0].File = "../escape.wasm"
	if err := validateGrammarManifest(escaping, true); err == nil {
		t.Fatal("escaping asset path accepted")
	}
}

func TestLoadGrammarAssetsVerifiesCompressedAndRawContent(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(minimalGoGrammarWASM); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	compressedHash := sha256.Sum256(compressed.Bytes())
	rawHash := sha256.Sum256(minimalGoGrammarWASM)
	manifest := GrammarManifest{
		Format: 2, AssetRevision: AssetRevision, TreeSitter: "0.24.4", Target: "wasm32-wasip1",
		Assets: []GrammarAsset{{
			Language: "go", File: "go.wasm.gz", Compression: "gzip",
			SHA256: hex.EncodeToString(compressedHash[:]), Size: int64(compressed.Len()),
			UncompressedSHA256: hex.EncodeToString(rawHash[:]), UncompressedSize: int64(len(minimalGoGrammarWASM)),
		}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"assets/manifest.json": &fstest.MapFile{Data: manifestBytes},
		"assets/go.wasm.gz":    &fstest.MapFile{Data: compressed.Bytes()},
	}
	runtime, _, err := loadGrammarAssets(context.Background(), files, "assets/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	if runtime.Count() != 1 {
		t.Fatalf("loaded %d modules", runtime.Count())
	}
	tampered := append([]byte(nil), compressed.Bytes()...)
	tampered[len(tampered)-1] ^= 1
	files["assets/go.wasm.gz"] = &fstest.MapFile{Data: tampered}
	if _, _, err := loadGrammarAssets(context.Background(), files, "assets/manifest.json", false); err == nil {
		t.Fatal("tampered compressed module accepted")
	}
}

func TestLoadGrammarAssetsCompilesPinnedInventory(t *testing.T) {
	if raceEnabled {
		t.Skip("compiling 159 wazero AOT modules under -race exceeds CI time limits")
	}
	ctx := context.Background()
	runtime, _, err := LoadGrammarAssets(ctx, EngineAssets, "assets/grammars/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if !runtime.Complete() {
		t.Fatalf("loaded %d modules, want %d", runtime.Count(), len(Languages))
	}
}
