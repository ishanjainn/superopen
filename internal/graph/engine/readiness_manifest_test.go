package engine

import (
	"testing"
)

func TestReadinessManifestLoadsAndRequiresExecutedProof(t *testing.T) {
	manifest, err := loadReadinessManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.AssetRevision != AssetRevision {
		t.Fatalf("manifest commit = %q", manifest.AssetRevision)
	}
	for id, proof := range manifest.GateProofs {
		if proof.Proof == "executed" {
			state := gateStateFromManifest(id, gatePending)
			if state != gateVerified {
				t.Fatalf("gate %q with executed proof did not verify", id)
			}
		}
	}
	capabilities := Capabilities()
	if capabilities.Readiness.Verified != 0 {
		t.Fatalf("expected no verified gates without executed proofs, got %d", capabilities.Readiness.Verified)
	}
}
