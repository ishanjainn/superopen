package engine

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
)

const (
	pretrainedTokenCount    = 40856
	pretrainedDimensions    = 768
	pretrainedTokensSHA256  = "c928f5e2f9dd85f2294a50a05dd9f2f8bc95192727579aa16b062ff8ef301d25"
	pretrainedVectorsSHA256 = "c76bba4c5032323ded6202053af5afdbbac12f6d920c691b3b3b4cd708f99e83"
)

type pretrainedVectors struct {
	index   map[string]int
	vectors []byte
}

// VerifyPinnedPretrainedAssets validates the bundled tokenizer vocabulary and
// quantized Nomic table without exposing the model representation across the
// provider protocol boundary.
func VerifyPinnedPretrainedAssets(files fs.FS, tokensPath, vectorsPath string) (int, int, error) {
	model, err := loadPinnedPretrainedVectors(files, tokensPath, vectorsPath)
	if err != nil {
		return 0, 0, err
	}
	_ = model
	return pretrainedTokenCount, pretrainedDimensions, nil
}

func loadPinnedPretrainedVectors(files fs.FS, tokensPath, vectorsPath string) (*pretrainedVectors, error) {
	return loadPretrainedVectors(files, tokensPath, vectorsPath, pretrainedTokensSHA256, pretrainedVectorsSHA256, pretrainedTokenCount)
}

func loadPretrainedVectors(files fs.FS, tokensPath, vectorsPath, tokenDigest, vectorDigest string, expectedCount int) (*pretrainedVectors, error) {
	tokenBytes, err := fs.ReadFile(files, tokensPath)
	if err != nil {
		return nil, err
	}
	vectorBytes, err := fs.ReadFile(files, vectorsPath)
	if err != nil {
		return nil, err
	}
	if digestBytes(tokenBytes) != tokenDigest || digestBytes(vectorBytes) != vectorDigest {
		return nil, fmt.Errorf("pretrained semantic assets failed content verification")
	}
	if len(vectorBytes) < 8 {
		return nil, fmt.Errorf("pretrained vector header is truncated")
	}
	count := int(binary.LittleEndian.Uint32(vectorBytes[:4]))
	dimensions := int(binary.LittleEndian.Uint32(vectorBytes[4:8]))
	if count != expectedCount || dimensions != pretrainedDimensions || len(vectorBytes) != 8+count*dimensions {
		return nil, fmt.Errorf("pretrained vector shape is %dx%d with %d bytes", count, dimensions, len(vectorBytes))
	}
	index := make(map[string]int, count)
	scanner := bufio.NewScanner(bytes.NewReader(tokenBytes))
	scanner.Buffer(make([]byte, 1024), 1<<20)
	lineCount := 0
	for scanner.Scan() {
		token := strings.TrimSuffix(scanner.Text(), "\r")
		if token == "" {
			lineCount++
			continue
		}
		if _, duplicate := index[token]; duplicate {
			return nil, fmt.Errorf("duplicate pretrained token %q", token)
		}
		index[token] = lineCount
		lineCount++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if lineCount != count {
		return nil, fmt.Errorf("pretrained token rows are %d, want %d", lineCount, count)
	}
	return &pretrainedVectors{index: index, vectors: vectorBytes[8:]}, nil
}

func (model *pretrainedVectors) Vector(token string) (semanticVector, bool) {
	var result semanticVector
	if model == nil {
		return result, false
	}
	index, ok := model.index[token]
	if !ok {
		return result, false
	}
	start := index * pretrainedDimensions
	for dimension, encoded := range model.vectors[start : start+pretrainedDimensions] {
		result[dimension] = float32(int8(encoded)) / 127
	}
	return result, true
}

func semanticIndex(token string, model *pretrainedVectors) semanticVector {
	if vector, ok := model.Vector(token); ok {
		return vector
	}
	return sparseSemanticIndex(token)
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
