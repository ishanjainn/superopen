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
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/klauspost/compress/zstd"
)

const (
	artifactMagic       = "SOGRAPH2\n"
	maxArtifactManifest = 1 << 20
	maxArtifactDatabase = int64(16 << 30)
)

type ArtifactManifest struct {
	Format         int    `json:"format"`
	Protocol       int    `json:"protocol"`
	Schema         int    `json:"schema"`
	AssetRevision string `json:"asset_revision"`
	Project        string `json:"project"`
	Generation     string `json:"generation"`
	DatabaseSHA256 string `json:"database_sha256"`
	DatabaseSize   int64  `json:"database_size"`
}

// ExportArtifact creates an explicit team artifact without writing anything
// inside the repository unless the caller deliberately chooses such a path.
func ExportArtifact(ctx context.Context, repoRoot, destination string) (ArtifactManifest, error) {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return ArtifactManifest{}, err
	}
	store, err := OpenReadOnly(paths.Database)
	if err != nil {
		return ArtifactManifest{}, err
	}
	if err := store.Verify(ctx); err != nil {
		store.Close()
		return ArtifactManifest{}, err
	}
	status, err := store.Status(ctx, "")
	closeErr := store.Close()
	if err != nil {
		return ArtifactManifest{}, err
	}
	if closeErr != nil {
		return ArtifactManifest{}, closeErr
	}
	database, err := os.Open(paths.Database)
	if err != nil {
		return ArtifactManifest{}, err
	}
	defer database.Close()
	info, err := database.Stat()
	if err != nil {
		return ArtifactManifest{}, err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, database); err != nil {
		return ArtifactManifest{}, err
	}
	if _, err := database.Seek(0, io.SeekStart); err != nil {
		return ArtifactManifest{}, err
	}
	manifest := ArtifactManifest{
		Format: 2, Protocol: api.ProtocolVersion, Schema: api.SchemaVersion,
		AssetRevision: AssetRevision, Project: status.Project, Generation: status.Generation,
		DatabaseSHA256: hex.EncodeToString(hasher.Sum(nil)), DatabaseSize: info.Size(),
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ArtifactManifest{}, err
	}
	if len(manifestBytes) > maxArtifactManifest {
		return ArtifactManifest{}, errors.New("artifact manifest exceeds limit")
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return ArtifactManifest{}, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return ArtifactManifest{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(abs), ".so-graph-artifact-*.tmp")
	if err != nil {
		return ArtifactManifest{}, err
	}
	tempPath := temp.Name()
	complete := false
	defer func() {
		_ = temp.Close()
		if !complete {
			_ = os.Remove(tempPath)
		}
	}()
	compressor, err := zstd.NewWriter(temp,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderConcurrency(1),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return ArtifactManifest{}, err
	}
	if _, err := io.WriteString(compressor, artifactMagic); err != nil {
		return ArtifactManifest{}, err
	}
	if err := binary.Write(compressor, binary.BigEndian, uint32(len(manifestBytes))); err != nil {
		return ArtifactManifest{}, err
	}
	if _, err := compressor.Write(manifestBytes); err != nil {
		return ArtifactManifest{}, err
	}
	if _, err := io.Copy(compressor, database); err != nil {
		return ArtifactManifest{}, err
	}
	if err := compressor.Close(); err != nil {
		return ArtifactManifest{}, err
	}
	if err := temp.Sync(); err != nil {
		return ArtifactManifest{}, err
	}
	if err := temp.Close(); err != nil {
		return ArtifactManifest{}, err
	}
	if err := os.Rename(tempPath, abs); err != nil {
		return ArtifactManifest{}, err
	}
	complete = true
	return manifest, nil
}

// ImportArtifact verifies metadata, compressed framing, content digest, and
// SQLite invariants before atomically replacing the cached generation.
func ImportArtifact(ctx context.Context, repoRoot, artifactPath string) (ArtifactManifest, string, error) {
	canonicalRoot, err := CanonicalRoot(repoRoot)
	if err != nil {
		return ArtifactManifest{}, "", err
	}
	artifact, err := os.Open(artifactPath)
	if err != nil {
		return ArtifactManifest{}, "", err
	}
	defer artifact.Close()
	reader, err := zstd.NewReader(artifact, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(uint64(maxArtifactDatabase+maxArtifactManifest)))
	if err != nil {
		return ArtifactManifest{}, "", err
	}
	defer reader.Close()
	buffered := bufio.NewReader(reader)
	magic := make([]byte, len(artifactMagic))
	if _, err := io.ReadFull(buffered, magic); err != nil || string(magic) != artifactMagic {
		return ArtifactManifest{}, "", errors.New("invalid so-graph artifact magic")
	}
	var manifestSize uint32
	if err := binary.Read(buffered, binary.BigEndian, &manifestSize); err != nil || manifestSize == 0 || manifestSize > maxArtifactManifest {
		return ArtifactManifest{}, "", errors.New("invalid so-graph artifact manifest size")
	}
	manifestBytes := make([]byte, manifestSize)
	if _, err := io.ReadFull(buffered, manifestBytes); err != nil {
		return ArtifactManifest{}, "", err
	}
	var manifest ArtifactManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ArtifactManifest{}, "", err
	}
	if manifest.Format != 2 || manifest.Protocol != api.ProtocolVersion || manifest.Schema != api.SchemaVersion || manifest.AssetRevision != AssetRevision || manifest.DatabaseSize <= 0 || manifest.DatabaseSize > maxArtifactDatabase || len(manifest.DatabaseSHA256) != 64 {
		return ArtifactManifest{}, "", errors.New("incompatible so-graph artifact metadata")
	}
	live, err := Publish(ctx, repoRoot, func(_ context.Context, staging string) error {
		file, err := os.OpenFile(staging, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(buffered, manifest.DatabaseSize+1))
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written != manifest.DatabaseSize || hex.EncodeToString(hasher.Sum(nil)) != manifest.DatabaseSHA256 {
			return errors.New("artifact database content verification failed")
		}
		if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
			return errors.New("artifact contains trailing uncompressed data")
		}
		store, err := OpenWritable(staging)
		if err != nil {
			return err
		}
		_, updateErr := store.db.ExecContext(ctx, `UPDATE projects SET root_path=? WHERE name=?`, canonicalRoot, manifest.Project)
		if updateErr == nil {
			updateErr = store.Seal(ctx)
		}
		if closeErr := store.Close(); updateErr == nil {
			updateErr = closeErr
		}
		return updateErr
	})
	if err != nil {
		return ArtifactManifest{}, "", fmt.Errorf("import graph artifact: %w", err)
	}
	return manifest, live, nil
}
