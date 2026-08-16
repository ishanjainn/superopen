package graph

import (
	"fmt"
	"testing"
)

func TestEvaluateBenchmarkLedgerReleaseThresholds(t *testing.T) {
	ledger := BenchmarkLedger{SchemaVersion: 1, Model: "any-agent-model", AgentVersion: "codex 1.0", InitCostUSD: map[string]float64{"repo": .16}}
	for i := 0; i < 16; i++ {
		kind := "question"
		state := ""
		if i >= 8 {
			kind, state = "patch", "ready"
		}
		ledger.Pairs = append(ledger.Pairs, BenchmarkPair{ID: fmt.Sprintf("task-%02d", i+1), Repository: "repo", Kind: kind, GraphSuitable: true,
			Control:   BenchmarkRun{Success: true, CostUSD: 1, FinalResponse: true},
			Treatment: BenchmarkRun{Success: true, CostUSD: .75, GraphCalls: 1, FinalResponse: true, PostEditState: state}})
	}
	got, err := EvaluateBenchmarkLedger(ledger)
	if err != nil || !got.Pass || got.CostReduction < .10 || got.GraphAdoption != 1 {
		t.Fatalf("gate=%+v err=%v", got, err)
	}
	ledger.Pairs[0].Treatment.FinalResponse = false
	got, err = EvaluateBenchmarkLedger(ledger)
	if err != nil || got.Pass {
		t.Fatalf("missing final response must fail: %+v err=%v", got, err)
	}
}

func TestEvaluateBenchmarkLedgerRequiresAgentIdentity(t *testing.T) {
	ledger := BenchmarkLedger{Model: "model", Pairs: make([]BenchmarkPair, 16)}
	if _, err := EvaluateBenchmarkLedger(ledger); err == nil {
		t.Fatal("release gate accepted a ledger without a coding-agent identity")
	}
}
