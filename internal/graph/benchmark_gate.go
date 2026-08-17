package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type BenchmarkRun struct {
	Success        bool    `json:"success"`
	CostUSD        float64 `json:"cost_usd"`
	RefreshCostUSD float64 `json:"refresh_cost_usd,omitempty"`
	GraphCalls     int     `json:"graph_calls"`
	FinalResponse  bool    `json:"final_response"`
	PostEditState  string  `json:"post_edit_graph_state,omitempty"`
	InputTokens    int64   `json:"input_tokens,omitempty"`
	OutputTokens   int64   `json:"output_tokens,omitempty"`
	CacheTokens    int64   `json:"cache_tokens,omitempty"`
	Turns          int     `json:"turns,omitempty"`
	Truncated      bool    `json:"truncated,omitempty"`
}

type BenchmarkPair struct {
	ID            string       `json:"id"`
	Repository    string       `json:"repository"`
	Kind          string       `json:"kind"` // question|patch
	TaskClass     string       `json:"task_class,omitempty"`
	RepeatGroup   string       `json:"repeat_group,omitempty"`
	RepeatIndex   int          `json:"repeat_index,omitempty"`
	GraphSuitable bool         `json:"graph_suitable"`
	Control       BenchmarkRun `json:"control"`
	Treatment     BenchmarkRun `json:"treatment"`
}

type BenchmarkLedger struct {
	SchemaVersion int                `json:"schema_version"`
	Profile       string             `json:"profile,omitempty"`
	Model         string             `json:"model"`
	AgentVersion  string             `json:"agent_version"`
	InitCostUSD   map[string]float64 `json:"initialization_cost_usd"`
	Pairs         []BenchmarkPair    `json:"pairs"`
}

type GateResult struct {
	Pass                          bool               `json:"pass"`
	ControlSuccesses              int                `json:"control_successes"`
	TreatmentSuccesses            int                `json:"treatment_successes"`
	MedianControlCostUSD          float64            `json:"median_control_cost_usd"`
	MedianRawTreatmentCostUSD     float64            `json:"median_raw_treatment_cost_usd"`
	MedianTreatmentCostUSD        float64            `json:"median_amortized_treatment_cost_usd"`
	CostReduction                 float64            `json:"cost_reduction"`
	GraphAdoption                 float64            `json:"graph_adoption"`
	TruncationRate                float64            `json:"truncation_rate,omitempty"`
	LocalTaskOverhead             float64            `json:"local_task_overhead,omitempty"`
	ControlCostPerSuccessUSD      float64            `json:"control_cost_per_success_usd,omitempty"`
	RawTreatmentCostPerSuccessUSD float64            `json:"raw_treatment_cost_per_success_usd,omitempty"`
	LifecycleCostPerSuccessUSD    map[string]float64 `json:"lifecycle_cost_per_success_usd,omitempty"`
	BreakEvenSessions             *int               `json:"break_even_sessions,omitempty"`
	Failures                      []string           `json:"failures,omitempty"`
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
	if ledger.SchemaVersion < 0 || ledger.SchemaVersion > 2 {
		return GateResult{}, fmt.Errorf("unsupported release ledger schema version %d", ledger.SchemaVersion)
	}
	compact := ledger.SchemaVersion >= 2 && ledger.Profile == "compact"
	wantPairs := 16
	if compact {
		wantPairs = 10
	} else if ledger.Profile != "" && ledger.Profile != "full" && ledger.Profile != "historical" {
		return GateResult{}, fmt.Errorf("unsupported benchmark profile %q", ledger.Profile)
	}
	if len(ledger.Pairs) != wantPairs {
		return GateResult{}, fmt.Errorf("%s benchmark ledger must contain exactly %d paired tasks, got %d", benchmarkProfileName(ledger), wantPairs, len(ledger.Pairs))
	}
	if ledger.Model == "" {
		return GateResult{}, fmt.Errorf("release ledger must identify the evaluated model")
	}
	if ledger.AgentVersion == "" {
		return GateResult{}, fmt.Errorf("release ledger must identify the coding agent and version")
	}
	repoTasks := map[string]int{}
	ids := map[string]bool{}
	classKinds := map[string]map[string]int{}
	questions, patches := 0, 0
	for _, pair := range ledger.Pairs {
		if pair.ID == "" || ids[pair.ID] {
			return GateResult{}, fmt.Errorf("release ledger task ids must be non-empty and unique: %q", pair.ID)
		}
		ids[pair.ID] = true
		if pair.Repository == "" {
			return GateResult{}, fmt.Errorf("task %s must identify its repository", pair.ID)
		}
		if pair.Control.CostUSD < 0 || pair.Treatment.CostUSD < 0 || pair.Control.RefreshCostUSD < 0 || pair.Treatment.RefreshCostUSD < 0 {
			return GateResult{}, fmt.Errorf("task %s contains a negative cost", pair.ID)
		}
		if ledger.SchemaVersion >= 2 && !validBenchmarkTaskClass(pair.TaskClass) {
			return GateResult{}, fmt.Errorf("task %s has invalid task_class %q", pair.ID, pair.TaskClass)
		}
		if ledger.SchemaVersion >= 2 {
			if classKinds[pair.TaskClass] == nil {
				classKinds[pair.TaskClass] = map[string]int{}
			}
			classKinds[pair.TaskClass][pair.Kind]++
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
	wantKind := wantPairs / 2
	if questions != wantKind || patches != wantKind {
		return GateResult{}, fmt.Errorf("%s benchmark ledger needs %d question and %d patch tasks, got %d/%d", benchmarkProfileName(ledger), wantKind, wantKind, questions, patches)
	}
	if compact {
		for _, class := range benchmarkTaskClasses() {
			if classKinds[class]["question"] != 1 || classKinds[class]["patch"] != 1 {
				return GateResult{}, fmt.Errorf("compact benchmark needs one question and one patch for task_class %s", class)
			}
		}
	}
	for repository, cost := range ledger.InitCostUSD {
		if cost < 0 {
			return GateResult{}, fmt.Errorf("repository %s has a negative initialization cost", repository)
		}
	}
	for repository := range repoTasks {
		if _, ok := ledger.InitCostUSD[repository]; !ok {
			return GateResult{}, fmt.Errorf("release ledger must record initialization cost for repository %s (use zero when measured as zero)", repository)
		}
	}
	result := GateResult{Pass: true}
	controls, rawTreatments, treatments := make([]float64, 0, 16), make([]float64, 0, 16), make([]float64, 0, 16)
	localControls, localTreatments := []float64{}, []float64{}
	suitable, adopted, truncated := 0, 0, 0
	totalControl, totalTreatment := 0.0, 0.0
	for _, pair := range ledger.Pairs {
		if pair.Control.Success {
			result.ControlSuccesses++
		}
		if pair.Treatment.Success {
			result.TreatmentSuccesses++
		}
		controls = append(controls, pair.Control.CostUSD)
		rawTreatment := pair.Treatment.CostUSD + pair.Treatment.RefreshCostUSD
		rawTreatments = append(rawTreatments, rawTreatment)
		totalControl += pair.Control.CostUSD + pair.Control.RefreshCostUSD
		totalTreatment += rawTreatment
		amortized := rawTreatment
		if n := repoTasks[pair.Repository]; n > 0 {
			amortized += ledger.InitCostUSD[pair.Repository] / float64(n)
		}
		treatments = append(treatments, amortized)
		if pair.GraphSuitable {
			suitable++
			if pair.Treatment.GraphCalls > 0 {
				adopted++
			}
			if pair.Treatment.Truncated {
				truncated++
			}
		}
		if pair.TaskClass == "local_lookup" {
			localControls = append(localControls, pair.Control.CostUSD+pair.Control.RefreshCostUSD)
			localTreatments = append(localTreatments, rawTreatment)
		}
		if pair.Control.FinalResponse && !pair.Treatment.FinalResponse {
			result.Failures = append(result.Failures, pair.ID+": treatment lost final response")
		}
		if pair.Kind == "patch" && pair.Treatment.PostEditState != "ready" && pair.Treatment.PostEditState != "continuation_required" {
			result.Failures = append(result.Failures, pair.ID+": invalid post-edit graph state "+pair.Treatment.PostEditState)
		}
	}
	result.MedianControlCostUSD = median(controls)
	result.MedianRawTreatmentCostUSD = median(rawTreatments)
	result.MedianTreatmentCostUSD = median(treatments)
	if result.MedianControlCostUSD > 0 {
		result.CostReduction = 1 - result.MedianTreatmentCostUSD/result.MedianControlCostUSD
	}
	if suitable > 0 {
		result.GraphAdoption = float64(adopted) / float64(suitable)
		result.TruncationRate = float64(truncated) / float64(suitable)
	}
	if len(localControls) > 0 && median(localControls) > 0 {
		result.LocalTaskOverhead = median(localTreatments)/median(localControls) - 1
	}
	if result.ControlSuccesses > 0 {
		result.ControlCostPerSuccessUSD = totalControl / float64(result.ControlSuccesses)
	}
	if result.TreatmentSuccesses > 0 {
		result.RawTreatmentCostPerSuccessUSD = totalTreatment / float64(result.TreatmentSuccesses)
	}
	if result.TreatmentSuccesses < result.ControlSuccesses {
		result.Failures = append(result.Failures, "treatment task success is below control")
	}
	if result.GraphAdoption < .80 {
		result.Failures = append(result.Failures, "graph adoption is below 80%")
	}
	if ledger.SchemaVersion < 2 {
		if result.CostReduction < .10 {
			result.Failures = append(result.Failures, "median amortized cost reduction is below 10%")
		}
	} else {
		result.LifecycleCostPerSuccessUSD, result.BreakEvenSessions = lifecycleCosts(ledger, totalControl, totalTreatment, result.ControlSuccesses, result.TreatmentSuccesses)
		if result.LifecycleCostPerSuccessUSD["treatment_25"] > result.LifecycleCostPerSuccessUSD["control_25"] {
			result.Failures = append(result.Failures, "treatment lifecycle cost per successful result has not broken even by 25 sessions")
		}
		if len(localControls) > 0 && result.LocalTaskOverhead > .05 {
			result.Failures = append(result.Failures, "median local-task overhead exceeds 5%")
		}
		if suitable > 0 && result.TruncationRate > .10 {
			result.Failures = append(result.Failures, "graph query truncation exceeds 10% on graph-suitable tasks")
		}
	}
	result.Pass = len(result.Failures) == 0
	return result, nil
}

func benchmarkProfileName(ledger BenchmarkLedger) string {
	if ledger.Profile != "" {
		return ledger.Profile
	}
	return "full"
}

func benchmarkTaskClasses() []string {
	return []string{"local_lookup", "cross_cutting", "impact_analysis", "temporal_update", "semantic_document"}
}

func validBenchmarkTaskClass(value string) bool {
	for _, class := range benchmarkTaskClasses() {
		if value == class {
			return true
		}
	}
	return false
}

func lifecycleCosts(ledger BenchmarkLedger, totalControl, totalTreatment float64, controlSuccesses, treatmentSuccesses int) (map[string]float64, *int) {
	result := map[string]float64{}
	n := float64(len(ledger.Pairs))
	controlPerAttempt, treatmentPerAttempt := totalControl/n, totalTreatment/n
	controlRate, treatmentRate := float64(controlSuccesses)/n, float64(treatmentSuccesses)/n
	initTotal := 0.0
	for _, cost := range ledger.InitCostUSD {
		initTotal += cost
	}
	initPerRepo := 0.0
	if len(ledger.InitCostUSD) > 0 {
		initPerRepo = initTotal / float64(len(ledger.InitCostUSD))
	}
	for _, horizon := range []int{1, 10, 25, 50} {
		h := float64(horizon)
		if controlRate > 0 {
			result[fmt.Sprintf("control_%d", horizon)] = controlPerAttempt / controlRate
		}
		if treatmentRate > 0 {
			result[fmt.Sprintf("treatment_%d", horizon)] = (initPerRepo + h*treatmentPerAttempt) / (h * treatmentRate)
		}
	}
	if controlRate == 0 || treatmentRate == 0 {
		return result, nil
	}
	controlCostPerSuccess := controlPerAttempt / controlRate
	for horizon := 1; horizon <= 10000; horizon++ {
		h := float64(horizon)
		if (initPerRepo+h*treatmentPerAttempt)/(h*treatmentRate) <= controlCostPerSuccess {
			value := horizon
			return result, &value
		}
	}
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
