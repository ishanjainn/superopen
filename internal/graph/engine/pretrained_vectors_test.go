package engine

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
	"testing/fstest"
)

func TestPinnedPretrainedVectorAssets(t *testing.T) {
	directory := os.Getenv("SUPEROPEN_NOMIC_DIR")
	if directory == "" {
		t.Skip("set SUPEROPEN_NOMIC_DIR to verify the pinned pretrained model assets")
	}
	model, err := loadPinnedPretrainedVectors(os.DirFS(directory), "code_tokens.txt", "code_vectors.bin")
	if err != nil {
		t.Fatal(err)
	}
	if len(model.index) >= pretrainedTokenCount || len(model.index) < 40_000 {
		t.Fatalf("loaded %d non-empty pretrained tokens", len(model.index))
	}
	if _, ok := model.Vector("function"); !ok {
		t.Fatal("representative pretrained token missing")
	}
}

func TestPretrainedVectorLoaderVerifiesAndDecodes(t *testing.T) {
	tokens := []byte("alpha\nbeta\n")
	vectors := make([]byte, 8+2*pretrainedDimensions)
	binary.LittleEndian.PutUint32(vectors[:4], 2)
	binary.LittleEndian.PutUint32(vectors[4:8], pretrainedDimensions)
	vectors[8], vectors[8+pretrainedDimensions] = 127, 129 // int8(-127) in two's complement
	files := fstest.MapFS{
		"tokens.txt":  &fstest.MapFile{Data: tokens},
		"vectors.bin": &fstest.MapFile{Data: vectors},
	}
	model, err := loadPretrainedVectors(files, "tokens.txt", "vectors.bin", digestBytes(tokens), digestBytes(vectors), 2)
	if err != nil {
		t.Fatal(err)
	}
	alpha, ok := model.Vector("alpha")
	if !ok || alpha[0] != 1 {
		t.Fatalf("alpha vector = %f, %t", alpha[0], ok)
	}
	beta, ok := model.Vector("beta")
	if !ok || beta[0] != -1 {
		t.Fatalf("beta vector = %f, %t", beta[0], ok)
	}
	if fallback := semanticIndex("missing", model); fallback == (semanticVector{}) {
		t.Fatal("missing token did not use sparse fallback")
	}
}

func TestPretrainedVectorLoaderRejectsCorruptionAndShape(t *testing.T) {
	tokens := []byte("alpha\n")
	vectors := make([]byte, 8+pretrainedDimensions)
	binary.LittleEndian.PutUint32(vectors[:4], 1)
	binary.LittleEndian.PutUint32(vectors[4:8], pretrainedDimensions)
	files := fstest.MapFS{"t": &fstest.MapFile{Data: tokens}, "v": &fstest.MapFile{Data: vectors}}
	if _, err := loadPretrainedVectors(files, "t", "v", stringsOfZero(64), digestBytes(vectors), 1); err == nil {
		t.Fatal("bad token digest accepted")
	}
	broken := append([]byte(nil), vectors...)
	broken = broken[:len(broken)-1]
	files["v"] = &fstest.MapFile{Data: broken}
	if _, err := loadPretrainedVectors(files, "t", "v", digestBytes(tokens), digestBytes(broken), 1); err == nil {
		t.Fatal("bad vector shape accepted")
	}
	if bytes.Equal(vectors, broken) {
		t.Fatal("test corruption ineffective")
	}
}

func stringsOfZero(count int) string {
	return string(bytes.Repeat([]byte{'0'}, count))
}
