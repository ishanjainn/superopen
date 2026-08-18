package engine

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

//go:embed readiness_manifest.json
var embeddedReadinessManifest []byte

var (
	loadManifestOnce sync.Once
	loadedManifest   readinessManifestDocument
	manifestLoadErr  error
)

type readinessManifestDocument struct {
	AssetRevision string                       `json:"asset_revision"`
	SourceRevision string                       `json:"source_revision"`
	Project        string                       `json:"project"`
	GateProofs     map[string]readinessGateProof   `json:"gate_proofs"`
	GateOverrides  map[string]string            `json:"gate_overrides,omitempty"`
}

type readinessGateProof struct {
	Proof string `json:"proof"`
	Notes string `json:"notes,omitempty"`
}

func loadReadinessManifest() (readinessManifestDocument, error) {
	loadManifestOnce.Do(func() {
		manifestLoadErr = json.Unmarshal(embeddedReadinessManifest, &loadedManifest)
	})
	return loadedManifest, manifestLoadErr
}

func gateStateFromManifest(id, defaultState string) string {
	manifest, err := loadReadinessManifest()
	if err != nil {
		return defaultState
	}
	if proof, ok := manifest.GateProofs[id]; ok && proof.Proof == "executed" {
		return gateVerified
	}
	if override, ok := manifest.GateOverrides[id]; ok && override != "" {
		return override
	}
	return defaultState
}

func readinessGatesWithManifest() []api.ReadinessGate {
	gates := readinessGateDefinitions()
	for index := range gates {
		gates[index].State = gateStateFromManifest(gates[index].ID, gates[index].State)
	}
	return gates
}
