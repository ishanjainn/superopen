package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/klauspost/compress/zstd"
)

// VerifyArtifact validates framing, compatibility, digest, size, and SQLite
// invariants without publishing or otherwise mutating repository state.
func VerifyArtifact(ctx context.Context, artifactPath string) (ArtifactManifest, error) {
	artifact, err := os.Open(artifactPath)
	if err != nil {
		return ArtifactManifest{}, err
	}
	defer artifact.Close()
	reader, err := zstd.NewReader(artifact, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(uint64(maxArtifactDatabase+maxArtifactManifest)))
	if err != nil {
		return ArtifactManifest{}, err
	}
	defer reader.Close()
	buffered := bufio.NewReader(reader)
	magic := make([]byte, len(artifactMagic))
	if _, err := io.ReadFull(buffered, magic); err != nil || string(magic) != artifactMagic {
		return ArtifactManifest{}, errors.New("invalid so-graph artifact magic")
	}
	var manifestSize uint32
	if err := binary.Read(buffered, binary.BigEndian, &manifestSize); err != nil || manifestSize == 0 || manifestSize > maxArtifactManifest {
		return ArtifactManifest{}, errors.New("invalid so-graph artifact manifest size")
	}
	manifestBytes := make([]byte, manifestSize)
	if _, err := io.ReadFull(buffered, manifestBytes); err != nil {
		return ArtifactManifest{}, err
	}
	var manifest ArtifactManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ArtifactManifest{}, err
	}
	if manifest.Format != 2 || manifest.Protocol != api.ProtocolVersion || manifest.Schema != api.SchemaVersion || manifest.AssetRevision != AssetRevision || manifest.DatabaseSize <= 0 || manifest.DatabaseSize > maxArtifactDatabase || len(manifest.DatabaseSHA256) != 64 {
		return ArtifactManifest{}, errors.New("incompatible so-graph artifact metadata")
	}
	temp, err := os.CreateTemp("", "so-graph-artifact-verify-*.db")
	if err != nil {
		return ArtifactManifest{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(buffered, manifest.DatabaseSize+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return ArtifactManifest{}, copyErr
	}
	if closeErr != nil {
		return ArtifactManifest{}, closeErr
	}
	if written != manifest.DatabaseSize || hex.EncodeToString(hasher.Sum(nil)) != manifest.DatabaseSHA256 {
		return ArtifactManifest{}, errors.New("artifact database content verification failed")
	}
	if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
		return ArtifactManifest{}, errors.New("artifact contains trailing uncompressed data")
	}
	store, err := OpenReadOnly(tempPath)
	if err != nil {
		return ArtifactManifest{}, err
	}
	verifyErr := store.Verify(ctx)
	closeErr = store.Close()
	if verifyErr != nil {
		return ArtifactManifest{}, verifyErr
	}
	if closeErr != nil {
		return ArtifactManifest{}, closeErr
	}
	return manifest, nil
}
