package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/superopen/so/internal/agentcli"
	"github.com/superopen/so/internal/config"
)

// Completer is anything that can answer a system+user prompt (API key or agent CLI).
type Completer interface {
	Available() bool
	Complete(system, user string) (string, error)
	Backend() string
}

// AgentCLI shells out to Claude Code (`claude -p`) or Codex (`codex exec`)
// in sealed/non-interactive mode. Uses the user's logged-in coding-agent
// subscription; no separate API key.
type AgentCLI struct {
	CLI   string // "claude" | "codex"
	Model string // optional override
}

func (a *AgentCLI) Available() bool {
	return a != nil && a.CLI != ""
}

func (a *AgentCLI) Backend() string {
	if a == nil {
		return ""
	}
	return "agent_cli:" + a.CLI
}

func (a *AgentCLI) Complete(system, user string) (string, error) {
	if !a.Available() {
		return "", fmt.Errorf("agent CLI not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	out, err := agentcli.Runner{CLI: a.CLI, Model: a.Model}.Run(ctx, system, user)
	if err != nil {
		return "", err
	}
	return out.Text, nil
}

func (c *Client) Backend() string {
	if c == nil {
		return ""
	}
	return "llm_api:" + c.Provider
}

// NewBestCompleter picks how backend evals/recommendations talk to a model:
//
//	evals.backend:
//	  heuristics - never call a model
//	  agent_cli  - Claude Code / Codex CLI only (reuse coding-agent login)
//	  llm_api    - API key / gateway only
//	  auto       - prefer agent CLI on PATH, else API key, else none
//
// Cursor has no sealed headless CLI for this; use Claude Code or Codex for
// backend judging. Cursor still works for interactive `/so init` upgrades.
func NewBestCompleter(cfg config.Config) Completer {
	return NewCompleterForBackend(cfg, cfg.Evals.Backend)
}

// NewMemoryCompleter uses memory.backend (default auto) - agent CLI preferred.
func NewMemoryCompleter(cfg config.Config) Completer {
	backend := cfg.Memory.Backend
	if backend == "" {
		backend = "auto"
	}
	return NewCompleterForBackend(cfg, backend)
}

// NewCompleterForBackend picks a Completer for the given backend selector.
func NewCompleterForBackend(cfg config.Config, backend string) Completer {
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		backend = "auto"
	}
	api := NewFromConfig(cfg)
	wantCLI := cfg.Evals.AgentCLI

	switch backend {
	case "heuristics", "none", "off":
		return nil
	case "llm_api", "api":
		if api.Available() {
			return api
		}
		return nil
	case "agent_cli", "cli", "coding_agent":
		return newAgentCLI(cfg, wantCLI)
	default: // auto
		if c := newAgentCLI(cfg, wantCLI); c != nil && c.Available() {
			return c
		}
		if api.Available() {
			return api
		}
		return nil
	}
}

func newAgentCLI(cfg config.Config, prefer string) *AgentCLI {
	prefer = strings.ToLower(strings.TrimSpace(prefer))
	switch prefer {
	case "claude", "claude-code":
		for _, c := range agentcli.DetectAll() {
			if c == "claude" {
				return &AgentCLI{CLI: "claude", Model: cfg.ModelForCLI("claude")}
			}
		}
		return nil
	case "codex":
		for _, c := range agentcli.DetectAll() {
			if c == "codex" {
				return &AgentCLI{CLI: "codex", Model: cfg.ModelForCLI("codex")}
			}
		}
		return nil
	default: // auto - first on PATH (claude preferred by Supported order)
		cli, err := agentcli.Detect()
		if err != nil {
			return nil
		}
		return &AgentCLI{CLI: cli, Model: cfg.ModelForCLI(cli)}
	}
}
