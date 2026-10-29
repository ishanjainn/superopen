package generic

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/pricing"
)

// Gemini records usage on assistant messages, not on its hook payloads. In
// JSONL a message can be appended again after its tokens arrive, so the last
// record for each id is authoritative. Rewound messages still incurred cost.
const maxGeminiTranscriptBytes = 32 << 20

type geminiRecordedMessage struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Model  string `json:"model"`
	Tokens struct {
		Input    int64 `json:"input"`
		Output   int64 `json:"output"`
		Cached   int64 `json:"cached"`
		Thoughts int64 `json:"thoughts"`
		Total    int64 `json:"total"`
	} `json:"tokens"`
}

func geminiTranscriptUsage(path, sessionID, fallbackModel string) (normalize.LLMTurn, bool) {
	if path == "" || sessionID == "" {
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
	messages := make(map[string]geminiRecordedMessage)
	var recordedID string
	if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return normalize.LLMTurn{}, false
	}
	scanner := bufio.NewScanner(io.LimitReader(file, maxGeminiTranscriptBytes+1))
	scanner.Buffer(make([]byte, 64*1024), maxGeminiTranscriptBytes)
	for scanner.Scan() {
		var record struct {
			SessionID string `json:"sessionId"`
			Set       struct {
				SessionID string                  `json:"sessionId"`
				Messages  []geminiRecordedMessage `json:"messages"`
			} `json:"$set"`
			Messages []geminiRecordedMessage `json:"messages"`
			ID       string                  `json:"id"`
			Type     string                  `json:"type"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue // An interrupted append can leave a partial last line.
		}
		if record.SessionID != "" {
			recordedID = record.SessionID
		}
		if record.Set.SessionID != "" {
			recordedID = record.Set.SessionID
		}
		for _, message := range record.Messages {
			storeGeminiMessage(messages, message)
		}
		for _, message := range record.Set.Messages {
			storeGeminiMessage(messages, message)
		}
		if record.ID != "" && record.Type == "gemini" {
			var message geminiRecordedMessage
			if json.Unmarshal(scanner.Bytes(), &message) == nil {
				storeGeminiMessage(messages, message)
			}
		}
	}
	if scanner.Err() != nil {
		return normalize.LLMTurn{}, false
	}
	if recordedID != sessionID {
		return normalize.LLMTurn{}, false
	}
	usage := normalize.LLMTurn{Model: fallbackModel}
	for _, message := range messages {
		if message.Type != "gemini" || message.Tokens.Input < 0 || message.Tokens.Output < 0 || message.Tokens.Thoughts < 0 || message.Tokens.Cached < 0 {
			continue
		}
		output := message.Tokens.Output + message.Tokens.Thoughts
		usage.InputTokens += message.Tokens.Input
		usage.OutputTokens += output
		total := message.Tokens.Total
		if total <= 0 {
			total = message.Tokens.Input + output
		}
		usage.TotalTokens += total
		model := message.Model
		if model == "" {
			model = fallbackModel
		}
		usage.CostUSD += pricing.Lookup(model).Cost(message.Tokens.Input, output, message.Tokens.Cached, 0)
	}
	return usage, usage.TotalTokens > 0
}

func storeGeminiMessage(messages map[string]geminiRecordedMessage, message geminiRecordedMessage) {
	if message.ID == "" || message.Type != "gemini" {
		return
	}
	// Token updates are appended as replacement messages. A later copy
	// without tokens must not erase a previously recorded API call.
	if message.Tokens.Total > 0 || message.Tokens.Input > 0 || message.Tokens.Output > 0 || message.Tokens.Thoughts > 0 {
		messages[message.ID] = message
	}
}
