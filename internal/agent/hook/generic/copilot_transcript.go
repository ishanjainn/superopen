package generic

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/pricing"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

type copilotModelMetric struct {
	Usage struct {
		Input      int64 `json:"inputTokens"`
		Output     int64 `json:"outputTokens"`
		CacheRead  int64 `json:"cacheReadTokens"`
		CacheWrite int64 `json:"cacheWriteTokens"`
	} `json:"usage"`
}

// The CLI's sessionEnd hook has no usage fields. Completed session event logs
// carry a modelMetrics rollup in session.shutdown. No summary means no claim of
// complete token totals; output-only assistant.message rows are not a substitute.
func copilotTranscriptUsage(transcriptPath, sessionID string) (normalize.LLMTurn, bool) {
	if sessionID == "" || filepath.Base(sessionID) != sessionID || sessionID == "." || sessionID == ".." {
		return normalize.LLMTurn{}, false
	}
	path := transcriptPath
	if path == "" {
		root := strings.TrimSpace(os.Getenv("COPILOT_HOME"))
		if root == "" {
			var err error
			root, err = os.UserHomeDir()
			if err != nil {
				return normalize.LLMTurn{}, false
			}
			root = filepath.Join(root, ".copilot")
		}
		path = filepath.Join(root, "session-state", sessionID, "events.jsonl")
	}
	if filepath.Base(path) != "events.jsonl" {
		return normalize.LLMTurn{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return normalize.LLMTurn{}, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxGeminiTranscriptBytes {
		return normalize.LLMTurn{}, false
	}
	var recordedID string
	var metrics map[string]copilotModelMetric
	scanner := bufio.NewScanner(io.LimitReader(file, maxGeminiTranscriptBytes+1))
	scanner.Buffer(make([]byte, 64*1024), maxGeminiTranscriptBytes)
	for scanner.Scan() {
		var record struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		switch record.Type {
		case "session.start":
			var data struct {
				SessionID string `json:"sessionId"`
			}
			if json.Unmarshal(record.Data, &data) == nil && data.SessionID != "" {
				recordedID = data.SessionID
			}
		case "session.shutdown":
			var data struct {
				ModelMetrics map[string]copilotModelMetric `json:"modelMetrics"`
			}
			if json.Unmarshal(record.Data, &data) == nil {
				metrics = data.ModelMetrics
			}
		}
	}
	if scanner.Err() != nil || recordedID != sessionID || len(metrics) == 0 {
		return normalize.LLMTurn{}, false
	}
	usage := normalize.LLMTurn{}
	for model, metric := range metrics {
		m := metric.Usage
		if m.Input < 0 || m.Output < 0 || m.CacheRead < 0 || m.CacheWrite < 0 {
			continue
		}
		usage.InputTokens += m.Input
		usage.OutputTokens += m.Output // reasoningTokens is a subset of outputTokens.
		usage.CostUSD += pricing.Lookup(model).Cost(m.Input, m.Output, m.CacheRead, m.CacheWrite)
	}
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	return usage, usage.TotalTokens > 0
}

// BackfillCopilotSessionUsage repairs a session root when the CLI writes its
// shutdown rollup after the sessionEnd hook. The complete per-model rollup is
// used only when the hook did not already report usage.
func BackfillCopilotSessionUsage(sessionID string, spans []trace.Span) bool {
	root := -1
	for i := range spans {
		if spans[i].Name == "coding_agent.session" && spans[i].Attributes["gen_ai.usage.total_tokens"] == "" {
			root = i
		}
	}
	if root < 0 {
		return false
	}
	usage, ok := copilotTranscriptUsage("", sessionID)
	if !ok {
		return false
	}
	attrs := spans[root].Attributes
	if attrs == nil {
		attrs = make(map[string]string)
		spans[root].Attributes = attrs
	}
	attrs["gen_ai.usage.input_tokens"] = strconv.FormatInt(usage.InputTokens, 10)
	attrs["gen_ai.usage.output_tokens"] = strconv.FormatInt(usage.OutputTokens, 10)
	attrs["gen_ai.usage.total_tokens"] = strconv.FormatInt(usage.TotalTokens, 10)
	if usage.CostUSD > 0 {
		attrs["gen_ai.usage.cost"] = strconv.FormatFloat(usage.CostUSD, 'f', -1, 64)
		attrs["coding_agent.session.cost_usd"] = attrs["gen_ai.usage.cost"]
	}
	return true
}
