package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type BenchmarkRun struct {
	Success       bool    `json:"success"`
	CostUSD       float64 `json:"cost_usd"`
	GraphCalls    int     `json:"graph_calls"`
	FinalResponse bool    `json:"final_response"`
	PostEditState string  `json:"post_edit_graph_state,omitempty"`
}

type BenchmarkPair struct {
	ID            string       `json:"id"`
	Repository    string       `json:"repository"`
	Kind          string       `json:"kind"` // question|patch
	GraphSuitable bool         `json:"graph_suitable"`
	Control       BenchmarkRun `json:"control"`
	Treatment     BenchmarkRun `json:"treatment"`
}

type BenchmarkLedger struct {
	SchemaVersion int                `json:"schema_version"`
	Model         string             `json:"model"`
	AgentVersion  string             `json:"agent_version"`
	InitCostUSD   map[string]float64 `json:"initialization_cost_usd"`
	Pairs         []BenchmarkPair    `json:"pairs"`
}

type GateResult struct {
	Pass                   bool     `json:"pass"`
	ControlSuccesses       int      `json:"control_successes"`
	TreatmentSuccesses     int      `json:"treatment_successes"`
	MedianControlCostUSD   float64  `json:"median_control_cost_usd"`
	MedianTreatmentCostUSD float64  `json:"median_amortized_treatment_cost_usd"`
	CostReduction          float64  `json:"cost_reduction"`
	GraphAdoption          float64  `json:"graph_adoption"`
	Failures               []string `json:"failures,omitempty"`
}

func EvaluateBenchmarkLedgerFile(path string) (GateResult, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return GateResult{}, err
	}
	var ledger BenchmarkLedger
	if err := json.Unmarshal(body, &ledger); err != nil {
		return GateResult{}, err
	}
	return EvaluateBenchmarkLedger(ledger)
}

func EvaluateBenchmarkLedger(ledger BenchmarkLedger) (GateResult, error) {
	if len(ledger.Pairs) != 16 {
		return GateResult{}, fmt.Errorf("release ledger must contain exactly 16 paired tasks, got %d", len(ledger.Pairs))
	}
	if ledger.Model == "" {
		return GateResult{}, fmt.Errorf("release ledger must identify the evaluated model")
	}
	if ledger.AgentVersion == "" {
		return GateResult{}, fmt.Errorf("release ledger must identify the coding agent and version")
	}
	repoTasks := map[string]int{}
	ids := map[string]bool{}
	questions, patches := 0, 0
	for _, pair := range ledger.Pairs {
		if pair.ID == "" || ids[pair.ID] {
			return GateResult{}, fmt.Errorf("release ledger task ids must be non-empty and unique: %q", pair.ID)
		}
		ids[pair.ID] = true
		if pair.Repository == "" {
			return GateResult{}, fmt.Errorf("task %s must identify its repository", pair.ID)
		}
		if pair.Control.CostUSD < 0 || pair.Treatment.CostUSD < 0 {
			return GateResult{}, fmt.Errorf("task %s contains a negative cost", pair.ID)
		}
		repoTasks[pair.Repository]++
		switch pair.Kind {
		case "question":
			questions++
		case "patch":
			patches++
		default:
			return GateResult{}, fmt.Errorf("task %s has invalid kind %q", pair.ID, pair.Kind)
		}
	}
	if questions != 8 || patches != 8 {
		return GateResult{}, fmt.Errorf("release ledger needs 8 question and 8 patch tasks, got %d/%d", questions, patches)
	}
	result := GateResult{Pass: true}
	controls, treatments := make([]float64, 0, 16), make([]float64, 0, 16)
	suitable, adopted := 0, 0
	for _, pair := range ledger.Pairs {
		if pair.Control.Success {
			result.ControlSuccesses++
		}
		if pair.Treatment.Success {
			result.TreatmentSuccesses++
		}
		controls = append(controls, pair.Control.CostUSD)
		amortized := pair.Treatment.CostUSD
		if n := repoTasks[pair.Repository]; n > 0 {
			amortized += ledger.InitCostUSD[pair.Repository] / float64(n)
		}
		treatments = append(treatments, amortized)
		if pair.GraphSuitable {
			suitable++
			if pair.Treatment.GraphCalls > 0 {
				adopted++
			}
		}
		if pair.Control.FinalResponse && !pair.Treatment.FinalResponse {
			result.Failures = append(result.Failures, pair.ID+": treatment lost final response")
		}
		if pair.Kind == "patch" && pair.Treatment.PostEditState != "ready" && pair.Treatment.PostEditState != "continuation_required" {
			result.Failures = append(result.Failures, pair.ID+": invalid post-edit graph state "+pair.Treatment.PostEditState)
		}
	}
	result.MedianControlCostUSD = median(controls)
	result.MedianTreatmentCostUSD = median(treatments)
	if result.MedianControlCostUSD > 0 {
		result.CostReduction = 1 - result.MedianTreatmentCostUSD/result.MedianControlCostUSD
	}
	if suitable > 0 {
		result.GraphAdoption = float64(adopted) / float64(suitable)
	}
	if result.TreatmentSuccesses < result.ControlSuccesses {
		result.Failures = append(result.Failures, "treatment task success is below control")
	}
	if result.CostReduction < .10 {
		result.Failures = append(result.Failures, "median amortized cost reduction is below 10%")
	}
	if result.GraphAdoption < .80 {
		result.Failures = append(result.Failures, "graph adoption is below 80%")
	}
	result.Pass = len(result.Failures) == 0
	return result, nil
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}
