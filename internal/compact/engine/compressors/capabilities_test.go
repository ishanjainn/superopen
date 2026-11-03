package compressors

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestCapabilityManifestStableAndComplete(t *testing.T) {
	registry := Default()
	first, err := registry.CapabilityManifest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.CapabilityManifest()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("capability manifest is not deterministic")
	}
	var manifest CapabilityRegistry
	if err := json.Unmarshal(first, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || len(manifest.Capabilities) != 14 {
		t.Fatalf("unexpected manifest: version=%d capabilities=%d", manifest.SchemaVersion, len(manifest.Capabilities))
	}
	canonical, err := json.Marshal(manifest.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	if manifest.RegistrySHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("registry digest does not pin ordered capabilities")
	}
	for i, capability := range manifest.Capabilities {
		if i > 0 && manifest.Capabilities[i-1].TransformID >= capability.TransformID {
			t.Fatal("capabilities not sorted")
		}
		if capability.Provenance.Kind != "native" || capability.Provenance.SourceManifestSHA256 != nil {
			t.Fatalf("unexpected provenance for %s", capability.TransformID)
		}
		if capability.NotSmallerFallback != "original" || capability.Recovery != "exact_ccr" {
			t.Fatalf("unsafe fallback for %s", capability.TransformID)
		}
	}
}

// TestCapabilityManifestDigestIsPinned guards the core transform manifest.
// A digest change requires reviewing any persisted plans that pin it.
func TestCapabilityManifestDigestIsPinned(t *testing.T) {
	const pinned = "4555d200667e33a88063be823c5ced5e5f1b9b144a2c6c257840bbd206164324"
	raw, err := Default().CapabilityManifest()
	if err != nil {
		t.Fatal(err)
	}
	var manifest CapabilityRegistry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.RegistrySHA256 != pinned {
		t.Fatalf("registry digest changed: got %s want %s", manifest.RegistrySHA256, pinned)
	}
	for _, capability := range manifest.Capabilities {
		if capability.TransformID == "superopen.engine."+toolSchemaAnnotationsType+".v1" {
			t.Fatal("an excluded compressor reached the compiled-plan ABI")
		}
	}
	if _, ok := Default().For(toolSchemaAnnotationsType); !ok {
		t.Fatal("exclusion from the manifest must not unregister the compressor")
	}
}
