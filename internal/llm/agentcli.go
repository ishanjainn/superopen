package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/agentcli"
	"github.com/ishanjainn/superopen/internal/config"
)

// Completer is anything that can answer a system+user prompt (API key or agent CLI).
type Completer interface {
	Available() bool
	Complete(system, user string) (string, error)
	Backend() string
}

// AgentCLI shells out to a sealed coding-agent CLI (`claude -p`, `codex exec`,
// `opencode run`, `pi --print --no-tools`). Uses the user's logged-in
// subscription; no separate API key.
type AgentCLI struct {
	CLI   string // "claude" | "codex" | "opencode" | "pi"
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

// NewBestCompleter picks how backend evals talk to a model:
//
//	evals.backend:
//	  auto       - prefer agent CLI on PATH, else API key, else none/heuristics (default)
//	  agent_cli  - sealed coding-agent CLI only (reuse coding-agent login)
//	  llm_api    - API key / gateway only
//	  heuristics - never call a model
//
// Cost balance: agent/LLM judging produces useful harness improvements; heuristics
// is the free fallback when no model backend is available. Cursor, Gemini, and
// Copilot have no sealed headless CLI — use claude, codex, opencode, or pi for
// judging; those vendors still work for `/so init` and live-agent review.
func NewBestCompleter(cfg config.Config) Completer {
	backend := cfg.Evals.Backend
	if strings.TrimSpace(backend) == "" {
		backend = "auto"
	}
	return NewCompleterForBackend(cfg, backend)
}

// NewVendorCompleter keeps self-review vendor-affine when that vendor has a
// sealed CLI. Other vendors use the configured fallback and record its backend.
func NewVendorCompleter(cfg config.Config, vendor string) Completer {
	if prefer := preferCLIForVendor(vendor); prefer != "" {
		if c := newAgentCLI(cfg, prefer); c != nil && c.Available() {
			return c
		}
	}
	return NewBestCompleter(cfg)
}

func preferCLIForVendor(vendor string) string {
	v := strings.ToLower(strings.TrimSpace(vendor))
	switch {
	case strings.Contains(v, "claude"):
		return "claude"
	case strings.Contains(v, "codex"):
		return "codex"
	case strings.Contains(v, "opencode"):
		return "opencode"
	case v == "pi" || strings.HasPrefix(v, "pi-"):
		return "pi"
	default:
		return ""
	}
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
		if cfg.HasExplicitLLM() && api.Available() {
			return api
		}
		return nil
	}
}

func newAgentCLI(cfg config.Config, prefer string) *AgentCLI {
	prefer = strings.ToLower(strings.TrimSpace(prefer))
	switch prefer {
	case "claude", "claude-code", "codex", "opencode", "pi":
		if prefer == "claude-code" {
			prefer = "claude"
		}
		for _, c := range agentcli.DetectAll() {
			if c == prefer {
				return &AgentCLI{CLI: c, Model: cfg.ModelForCLI(c)}
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
