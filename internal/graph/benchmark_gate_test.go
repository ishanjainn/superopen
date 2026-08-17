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

func TestEvaluateBenchmarkLedgerRequiresNonNegativeInitializationCost(t *testing.T) {
	ledger := BenchmarkLedger{SchemaVersion: 1, Model: "model", AgentVersion: "agent", InitCostUSD: map[string]float64{"repo": 0}}
	for i := 0; i < 16; i++ {
		kind, state := "question", ""
		if i >= 8 {
			kind, state = "patch", "ready"
		}
		ledger.Pairs = append(ledger.Pairs, BenchmarkPair{
			ID: fmt.Sprint(i), Repository: "repo", Kind: kind,
			Control:   BenchmarkRun{Success: true, CostUSD: 1, FinalResponse: true},
			Treatment: BenchmarkRun{Success: true, CostUSD: .5, FinalResponse: true, PostEditState: state},
		})
	}
	if _, err := EvaluateBenchmarkLedger(ledger); err != nil {
		t.Fatalf("zero measured initialization cost rejected: %v", err)
	}
	delete(ledger.InitCostUSD, "repo")
	if _, err := EvaluateBenchmarkLedger(ledger); err == nil {
		t.Fatal("ledger without repository initialization cost was accepted")
	}
	ledger.InitCostUSD["repo"] = -0.01
	if _, err := EvaluateBenchmarkLedger(ledger); err == nil {
		t.Fatal("negative initialization cost was accepted")
	}
}

func TestEvaluateBenchmarkLedgerV2MeasuresLifecycleEffectiveness(t *testing.T) {
	ledger := BenchmarkLedger{SchemaVersion: 2, Model: "small-model", AgentVersion: "agent 2.0", InitCostUSD: map[string]float64{"repo": 1}}
	classes := []string{"local_lookup", "cross_cutting", "impact_analysis", "temporal_update", "semantic_document"}
	for i := 0; i < 16; i++ {
		kind, state := "question", ""
		if i >= 8 {
			kind, state = "patch", "ready"
		}
		class := classes[i%len(classes)]
		suitable := class != "local_lookup"
		graphCalls := 0
		if suitable {
			graphCalls = 1
		}
		ledger.Pairs = append(ledger.Pairs, BenchmarkPair{
			ID: fmt.Sprintf("v2-%02d", i), Repository: "repo", Kind: kind, TaskClass: class, GraphSuitable: suitable,
			Control:   BenchmarkRun{Success: true, CostUSD: 1, FinalResponse: true},
			Treatment: BenchmarkRun{Success: true, CostUSD: .7, RefreshCostUSD: .01, GraphCalls: graphCalls, FinalResponse: true, PostEditState: state},
		})
	}
	got, err := EvaluateBenchmarkLedger(ledger)
	if err != nil || !got.Pass {
		t.Fatalf("v2 gate=%+v err=%v", got, err)
	}
	if got.BreakEvenSessions == nil || *got.BreakEvenSessions > 25 {
		t.Fatalf("break-even=%v, want within 25 sessions", got.BreakEvenSessions)
	}
	if got.LifecycleCostPerSuccessUSD["treatment_25"] >= got.LifecycleCostPerSuccessUSD["control_25"] {
		t.Fatalf("lifecycle costs=%+v", got.LifecycleCostPerSuccessUSD)
	}
}

func TestEvaluateBenchmarkLedgerV2RejectsUnclassifiedTasks(t *testing.T) {
	ledger := BenchmarkLedger{SchemaVersion: 2, Model: "model", AgentVersion: "agent", Pairs: make([]BenchmarkPair, 16)}
	for i := range ledger.Pairs {
		ledger.Pairs[i] = BenchmarkPair{ID: fmt.Sprint(i), Repository: "repo", Kind: map[bool]string{true: "patch", false: "question"}[i >= 8]}
	}
	if _, err := EvaluateBenchmarkLedger(ledger); err == nil {
		t.Fatal("schema-v2 ledger accepted an unclassified task")
	}
}

func TestEvaluateBenchmarkLedgerCompactProfileCoversEveryClassAndKind(t *testing.T) {
	ledger := BenchmarkLedger{SchemaVersion: 2, Profile: "compact", Model: "model", AgentVersion: "agent", InitCostUSD: map[string]float64{"repo": .1}}
	for classIndex, class := range benchmarkTaskClasses() {
		for kindIndex, kind := range []string{"question", "patch"} {
			state := ""
			if kind == "patch" {
				state = "ready"
			}
			suitable := class != "local_lookup"
			graphCalls := 0
			if suitable {
				graphCalls = 1
			}
			ledger.Pairs = append(ledger.Pairs, BenchmarkPair{
				ID: fmt.Sprintf("compact-%d-%d", classIndex, kindIndex), Repository: "repo", Kind: kind,
				TaskClass: class, GraphSuitable: suitable,
				Control:   BenchmarkRun{Success: true, CostUSD: 1, FinalResponse: true},
				Treatment: BenchmarkRun{Success: true, CostUSD: .7, GraphCalls: graphCalls, FinalResponse: true, PostEditState: state},
			})
		}
	}
	if result, err := EvaluateBenchmarkLedger(ledger); err != nil || !result.Pass {
		t.Fatalf("valid compact profile failed: result=%+v err=%v", result, err)
	}
	ledger.Pairs[0].TaskClass = "cross_cutting"
	if _, err := EvaluateBenchmarkLedger(ledger); err == nil {
		t.Fatal("compact profile without one question and patch per class was accepted")
	}
}
