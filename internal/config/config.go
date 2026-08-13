package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the harness configuration stored at .so/config.yaml.
type Config struct {
	LayoutVersion   int                   `yaml:"layout_version"`
	Vendors         VendorsConfig         `yaml:"vendors"`
	LLM             LLMConfig             `yaml:"llm,omitempty"`
	Graph           GraphConfig           `yaml:"graph"`
	Evals           EvalsConfig           `yaml:"evals"`
	Recommendations RecommendationsConfig `yaml:"recommendations"`
	MCP             MCPConfig             `yaml:"mcp,omitempty"`
	Observability   ObservabilityConfig   `yaml:"observability"`
	Memory          MemoryConfig          `yaml:"memory"`
	Guardrails      GuardrailsConfig      `yaml:"guardrails"`
	Inject          InjectConfig          `yaml:"inject"`
	Cost            CostConfig            `yaml:"cost"`
	Retention       RetentionConfig       `yaml:"retention"`
}

type VendorsConfig struct {
	Enabled      []string `yaml:"enabled,omitempty"`
	SharedAgents bool     `yaml:"shared_agents"`
}

type LLMConfig struct {
	Provider  string `yaml:"provider"` // openai | anthropic | openrouter | local | compatible
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
	BaseURL   string `yaml:"base_url,omitempty"` // OpenAI-compatible gateway / local server
}

// HasExplicitLLM reports whether project configuration opted into an API or
// compatible local model backend. Ambient API keys alone do not opt reviews in.
func (c Config) HasExplicitLLM() bool {
	return strings.TrimSpace(c.LLM.Provider) != "" || strings.TrimSpace(c.LLM.Model) != "" ||
		strings.TrimSpace(c.LLM.APIKeyEnv) != "" || strings.TrimSpace(c.LLM.BaseURL) != ""
}

type GraphConfig struct {
	Code            bool   `yaml:"code"`
	Semantic        bool   `yaml:"semantic"`
	SemanticBackend string `yaml:"semantic_backend"`
	RefreshPolicy   string `yaml:"refresh_policy"`
}

type EvalsConfig struct {
	Auto         bool `yaml:"auto"`
	OnSessionEnd bool `yaml:"on_session_end"`
	// ActiveCooldownHours: min gap between snapshot evals on an open chat (default 6).
	// Manual `so eval --force` / Sessions UI Evaluate bypasses this.
	ActiveCooldownHours int `yaml:"active_cooldown_hours,omitempty"`
	// Backend: auto | agent_cli | llm_api | heuristics
	// Default auto: sealed Claude/Codex CLI → API key → heuristics. Prefer agent
	// judging for useful harness improvements; heuristics only when no model is available.
	Backend string `yaml:"backend"`
	// AgentCLI: auto | claude | codex - which sealed CLI to use for agent_cli/auto.
	AgentCLI string `yaml:"agent_cli,omitempty"`
	// Models maps CLI name → model slug for sealed backend calls (claude, codex).
	Models map[string]string `yaml:"models,omitempty"`
}

type RecommendationsConfig struct {
	Auto            bool `yaml:"auto"`
	RequireApproval bool `yaml:"require_approval"`
	// AutoApplyTiers: soft | policy | evals | all. Empty + require_approval=true → [soft].
	// require_approval=false → [all].
	AutoApplyTiers []string `yaml:"auto_apply_tiers,omitempty"`
}

// MCPConfig is committed team policy for project-scoped MCP servers.
// so sync projects these into vendor files (.mcp.json, .cursor/mcp.json).
type MCPConfig struct {
	Servers []MCPServer `yaml:"servers,omitempty"`
}

// MCPServer is a stdio MCP server definition without secrets/env blocks.
type MCPServer struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
}

type ObservabilityConfig struct {
	Vendors   []string         `yaml:"vendors"`
	Exporters []ExporterConfig `yaml:"exporters"`
	Viz       VizConfig        `yaml:"viz"`
}

type ExporterConfig struct {
	// Type must be local_jsonl (local file under Path).
	Type string `yaml:"type"`
	Path string `yaml:"path,omitempty"`
}

// LocalTracesDir returns the on-disk JSONL traces directory from
// observability.exporters (local_jsonl only). Not exposed in Settings UI -
// defaults apply unless edited in .so/config.yaml.
func (c Config) LocalTracesDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".so", "sessions")
}

type VizConfig struct {
	Citymap bool `yaml:"citymap"`
	Replay  bool `yaml:"replay"`
}

type MemoryConfig struct {
	// Enabled defaults true. When on, SessionStart always injects Active Context.
	Enabled  *bool  `yaml:"enabled,omitempty"`
	Provider string `yaml:"provider"`
	// Backend: auto | agent_cli | llm_api | heuristics - same semantics as evals.backend.
	// auto prefers Claude Code / Codex CLI, else API key, else heuristics.
	Backend string `yaml:"backend,omitempty"`
	// IdleHarvestHours: harvest open sessions with no activity for this many hours (default 6).
	IdleHarvestHours int `yaml:"idle_harvest_hours,omitempty"`
	// Models optional per-CLI overrides; falls back to evals.models.
	Models map[string]string `yaml:"models,omitempty"`
}

// GuardrailsConfig toggles enforcement of .so/guardrails.yaml at hook boundaries.
// Advisory rules + denied commands/sensitive paths all live in that single file.
type GuardrailsConfig struct {
	// Enabled defaults true. Set false (or SUPEROPEN_GUARDRAILS=off) to pause denies while dogfooding.
	Enabled *bool `yaml:"enabled,omitempty"`
}

// InjectConfig soft-controls always-on agent instruction pressure.
type InjectConfig struct {
	// Rules defaults true. Set false (or SUPEROPEN_INJECT_RULES=off) to omit guardrail obedience lines from Brief.
	Rules *bool `yaml:"rules,omitempty"`
}

type CostConfig struct {
	Track bool `yaml:"track"`
}

// RetentionConfig controls how long compact session documents are kept.
type RetentionConfig struct {
	// Days defaults to 7. Removing a session removes its embedded events,
	// evaluation, recommendations, replay, and checkpoints as one unit.
	Days int `yaml:"days,omitempty"`
}

// Default returns MVP defaults from the product plan.
func Default() Config {
	return Config{
		LayoutVersion: 2,
		Vendors:       VendorsConfig{},
		Graph: GraphConfig{
			Code:            true,
			Semantic:        false,
			SemanticBackend: "auto",
			RefreshPolicy:   "after_changed_session",
		},
		Evals: EvalsConfig{
			Auto:         true,
			OnSessionEnd: true,
			Backend:      "auto",
			AgentCLI:     "auto",
			Models: map[string]string{
				"claude": "claude-sonnet-5",
				"codex":  "gpt-5.6-luna",
			},
		},
		Recommendations: RecommendationsConfig{
			Auto:            true,
			RequireApproval: true,
			AutoApplyTiers:  []string{"soft"},
		},
		Observability: ObservabilityConfig{
			Vendors: []string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"},
			Exporters: []ExporterConfig{
				{Type: "local_jsonl", Path: ".so/sessions"},
			},
			Viz: VizConfig{Citymap: false, Replay: true},
		},
		Memory: MemoryConfig{
			Provider:         "file_lessons",
			Backend:          "auto",
			IdleHarvestHours: 6,
		},
		Guardrails: GuardrailsConfig{},
		Inject:     InjectConfig{},
		Cost:       CostConfig{Track: true},
		Retention:  RetentionConfig{Days: 7},
	}
}

func boolPtr(v bool) *bool { return &v }

// MemoryEnabled respects SUPEROPEN_MEMORY env then config (default true).
func (c Config) MemoryEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SUPEROPEN_MEMORY"))) {
	case "off", "0", "false", "no", "disabled":
		return false
	case "on", "1", "true", "yes", "enabled":
		return true
	}
	if c.Memory.Enabled != nil {
		return *c.Memory.Enabled
	}
	return true
}

// IdleHarvestHours returns configured idle harvest threshold (default 6).
func (c Config) IdleHarvestHours() int {
	if c.Memory.IdleHarvestHours > 0 {
		return c.Memory.IdleHarvestHours
	}
	return 6
}

// EvalsActiveCooldownHours returns min gap between active-chat snapshot evals (default 6).
func (c Config) EvalsActiveCooldownHours() int {
	if c.Evals.ActiveCooldownHours > 0 {
		return c.Evals.ActiveCooldownHours
	}
	return 6
}

// AutoApplyTiersResolved returns which recommendation tiers auto-apply on finalize.
// require_approval=false → all. Empty tiers + approval → soft only.
func (c Config) AutoApplyTiersResolved() []string {
	if !c.Recommendations.RequireApproval {
		return []string{"all"}
	}
	if len(c.Recommendations.AutoApplyTiers) == 0 {
		return []string{"soft"}
	}
	out := make([]string, 0, len(c.Recommendations.AutoApplyTiers))
	for _, t := range c.Recommendations.AutoApplyTiers {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return []string{"soft"}
	}
	return out
}

// AllowsAutoApplyTier reports whether tier (soft|policy|evals) may auto-apply.
func (c Config) AllowsAutoApplyTier(tier string) bool {
	tier = strings.ToLower(strings.TrimSpace(tier))
	for _, t := range c.AutoApplyTiersResolved() {
		if t == "all" || t == tier {
			return true
		}
	}
	return false
}

// ModelForCLI returns the sealed-call model for claude|codex from memory.models then evals.models.
func (c Config) ModelForCLI(cli string) string {
	cli = strings.ToLower(strings.TrimSpace(cli))
	if cli == "" {
		return ""
	}
	if c.Memory.Models != nil {
		if m := strings.TrimSpace(c.Memory.Models[cli]); m != "" {
			return NormalizeModelSlug(cli, m)
		}
	}
	if c.Evals.Models != nil {
		if m := strings.TrimSpace(c.Evals.Models[cli]); m != "" {
			return NormalizeModelSlug(cli, m)
		}
	}
	// Sensible defaults for sealed harvest/eval.
	switch cli {
	case "claude":
		return "claude-sonnet-5"
	case "codex":
		return "gpt-5.6-luna"
	}
	return ""
}

// NormalizeModelSlug maps colloquial names to CLI/API ids.
func NormalizeModelSlug(cli, model string) string {
	model = strings.TrimSpace(model)
	low := strings.ToLower(model)
	cli = strings.ToLower(cli)
	switch cli {
	case "codex":
		switch low {
		case "luna":
			return "gpt-5.6-luna"
		case "terra":
			return "gpt-5.6-terra"
		case "sol":
			return "gpt-5.6-sol"
		case "sonnet-5", "sonnet5":
			return model // not a codex model; leave as-is
		}
	case "claude", "claude-code":
		switch low {
		case "sonnet-5", "sonnet5":
			return "claude-sonnet-5"
		case "sonnet":
			return "sonnet"
		case "haiku":
			return "haiku"
		case "opus":
			return "opus"
		}
	}
	return model
}

// RetentionDays returns how long to keep sessions/evals/audit/recs (default 7).
func (c Config) RetentionDays() int {
	if c.Retention.Days > 0 {
		return c.Retention.Days
	}
	return 7
}

// GuardrailsEnabled respects SUPEROPEN_GUARDRAILS env then config (default true).
func (c Config) GuardrailsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SUPEROPEN_GUARDRAILS"))) {
	case "off", "0", "false", "no", "disabled":
		return false
	case "on", "1", "true", "yes", "enabled":
		return true
	}
	if c.Guardrails.Enabled != nil {
		return *c.Guardrails.Enabled
	}
	return true
}

// InjectRulesEnabled respects SUPEROPEN_INJECT_RULES env then config (default true).
func (c Config) InjectRulesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SUPEROPEN_INJECT_RULES"))) {
	case "off", "0", "false", "no", "disabled":
		return false
	case "on", "1", "true", "yes", "enabled":
		return true
	}
	if c.Inject.Rules != nil {
		return *c.Inject.Rules
	}
	return true
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var header struct {
		LayoutVersion *int `yaml:"layout_version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if header.LayoutVersion == nil || *header.LayoutVersion != 2 {
		return Config{}, fmt.Errorf("unsupported Superopen layout: expected layout_version: 2; remove .so and run so init")
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.normalizeObservability()
	return cfg, nil
}

// normalizeObservability keeps a single local_jsonl file exporter. The UI
// reads this store directly and never owns a telemetry receiver.
func (c *Config) normalizeObservability() {
	var local []ExporterConfig
	for _, e := range c.Observability.Exporters {
		t := strings.ToLower(strings.TrimSpace(e.Type))
		if t == "" || t == "local_jsonl" {
			local = append(local, ExporterConfig{Type: "local_jsonl", Path: ".so/sessions"})
			break
		}
	}
	if len(local) == 0 {
		local = []ExporterConfig{{Type: "local_jsonl", Path: ".so/sessions"}}
	}
	c.Observability.Exporters = local
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# Superopen project configuration. This is the authoritative source for enabled vendors, review behavior, graph refresh, retention, and feature settings.\n# Updated by project maintainers and Superopen configuration commands.\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}

// ResolvedLLM is the effective provider settings after env autodetection.
type ResolvedLLM struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Source   string // which env/config supplied the key
}

// ResolveLLM picks provider/key/base URL from config + common env vars.
func (c Config) ResolveLLM() ResolvedLLM {
	r := ResolvedLLM{
		Provider: strings.ToLower(strings.TrimSpace(c.LLM.Provider)),
		Model:    c.LLM.Model,
		BaseURL:  strings.TrimRight(c.LLM.BaseURL, "/"),
	}
	if r.Provider == "" {
		r.Provider = "openai"
	}
	if r.Model == "" {
		r.Model = "gpt-4.1-mini"
	}

	// Explicit base URL overrides (config or env).
	if r.BaseURL == "" {
		if v := os.Getenv("SUPEROPEN_LLM_BASE_URL"); v != "" {
			r.BaseURL = strings.TrimRight(v, "/")
		} else if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
			r.BaseURL = strings.TrimRight(v, "/")
		}
	}

	key, src := c.lookupAPIKey()
	r.APIKey = key
	r.Source = src

	// Autodetect provider from which key/base was found.
	switch {
	case src == "OPENROUTER_API_KEY" || strings.Contains(strings.ToLower(r.BaseURL), "openrouter.ai"):
		r.Provider = "openrouter"
		if r.BaseURL == "" {
			r.BaseURL = "https://openrouter.ai/api/v1"
		}
		if c.LLM.Model == "" || c.LLM.Model == "gpt-4.1-mini" {
			if m := os.Getenv("SUPEROPEN_LLM_MODEL"); m != "" {
				r.Model = m
			} else {
				r.Model = "openai/gpt-4.1-mini"
			}
		}
	case src == "ANTHROPIC_API_KEY" || r.Provider == "anthropic":
		r.Provider = "anthropic"
		if r.Model == "gpt-4.1-mini" && c.LLM.Provider == "" {
			r.Model = "claude-sonnet-4-20250514"
		}
	case isLocalBase(r.BaseURL) || r.Provider == "local" || r.Provider == "ollama":
		r.Provider = "local"
		if r.BaseURL == "" {
			r.BaseURL = "http://127.0.0.1:11434/v1" // Ollama OpenAI-compatible API
		}
		if m := os.Getenv("SUPEROPEN_LLM_MODEL"); m != "" {
			r.Model = m
		} else if r.Model == "gpt-4.1-mini" {
			r.Model = "llama3.2"
		}
		if r.APIKey == "" {
			r.APIKey = "local" // many local servers require a non-empty Bearer token
			if r.Source == "" {
				r.Source = "local"
			}
		}
	case src == "OPENAI_API_KEY" || r.Provider == "openai" || r.Provider == "compatible":
		if r.Provider != "compatible" {
			r.Provider = "openai"
		}
	}

	if m := os.Getenv("SUPEROPEN_LLM_MODEL"); m != "" {
		r.Model = m
	}
	return r
}

func (c Config) lookupAPIKey() (key, source string) {
	env := c.LLM.APIKeyEnv
	if env == "" {
		env = "SUPEROPEN_LLM_API_KEY"
	}
	order := []string{
		env,
		"SUPEROPEN_LLM_API_KEY",
		"OPENROUTER_API_KEY",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	}
	seen := map[string]bool{}
	for _, name := range order {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if v := os.Getenv(name); v != "" {
			return v, name
		}
	}
	return "", ""
}

func isLocalBase(u string) bool {
	u = strings.ToLower(u)
	return strings.Contains(u, "127.0.0.1") ||
		strings.Contains(u, "localhost") ||
		strings.Contains(u, "0.0.0.0")
}

// APIKey resolves the LLM API key (compat helper).
func (c Config) APIKey() string {
	return c.ResolveLLM().APIKey
}

// HasLLM reports whether an LLM can be called (cloud key or local gateway).
func (c Config) HasLLM() bool {
	r := c.ResolveLLM()
	if r.Provider == "local" && r.BaseURL != "" {
		return true
	}
	return r.APIKey != "" && r.Source != "local"
}

// LLMSetupGuide is printed only when `so init --llm` fails without a key.
func LLMSetupGuide() string {
	return strings.TrimSpace(`
Headless LLM upgrade needs an API key (optional - /so init in Cursor/Claude uses the assistant model instead):

  export OPENAI_API_KEY=sk-...
  # or ANTHROPIC_API_KEY / OPENROUTER_API_KEY / OPENAI_BASE_URL=…

Then:  so init --llm
`) + "\n"
}
