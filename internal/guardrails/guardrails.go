// Package guardrails enforces guardrails.yaml (advisory rules + denied commands/paths) for coding hooks.
package guardrails

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
	"gopkg.in/yaml.v3"
)

// Rule is an advisory agent-facing guardrail (soft guidance).
type Rule struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
	Severity    string `yaml:"severity" json:"severity"` // block | warn
	Source      string `yaml:"source,omitempty" json:"source,omitempty"`
}

// File is the single on-disk guardrails document (.so/guardrails.yaml).
type File struct {
	Rules          []Rule   `yaml:"rules,omitempty"`
	Approval       string   `yaml:"approval"` // yolo | auto | interactive
	DeniedTools    []string `yaml:"denied_tools"`
	DeniedCommands []string `yaml:"denied_commands"`
	SensitivePaths []string `yaml:"sensitive_paths"`
	RedactOutput   bool     `yaml:"redact_output"`
}

// Policy is the enforcement slice of File (used by hooks).
type Policy struct {
	Approval       string   `yaml:"approval"`
	DeniedTools    []string `yaml:"denied_tools"`
	DeniedCommands []string `yaml:"denied_commands"`
	SensitivePaths []string `yaml:"sensitive_paths"`
	RedactOutput   bool     `yaml:"redact_output"`
}

type Decision struct {
	Allow   bool   `json:"allow"`
	Reason  string `json:"reason,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Matcher string `json:"matcher,omitempty"`
}

type Engine struct {
	Policy Policy
	Rules  []Rule
}

func DefaultPolicy() Policy {
	return Policy{
		Approval:     "interactive",
		RedactOutput: true,
		DeniedTools:  []string{},
		DeniedCommands: []string{
			"rm -rf /",
			"rm -rf /*",
			"mkfs*",
			"dd if=/dev/zero*",
			":(){ :|:& };:",
			"curl *| sh",
			"curl *| bash",
			"wget *| sh",
			"wget *| bash",
			"chmod -R 777 /",
		},
		// Narrow by default: only Superopen audit trail.
		// Broader secret-path blocks are opt-in via editing guardrails.yaml.
		SensitivePaths: []string{
			"**/.so/sessions/**",
			"**/.so/audit/**",
		},
	}
}

// Path is the canonical single guardrails file.
func Path(paths harness.Paths) string {
	return paths.GuardrailsFile
}

func Load(paths harness.Paths) (Engine, error) {
	eng := Engine{Policy: DefaultPolicy()}
	data, err := os.ReadFile(Path(paths))
	if err != nil {
		return eng, nil
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return eng, fmt.Errorf("guardrails.yaml: %w", err)
	}
	eng.Rules = f.Rules
	if len(f.DeniedCommands) > 0 {
		eng.Policy.DeniedCommands = f.DeniedCommands
	}
	if f.DeniedTools != nil {
		eng.Policy.DeniedTools = f.DeniedTools
	}
	if len(f.SensitivePaths) > 0 {
		eng.Policy.SensitivePaths = f.SensitivePaths
	}
	if f.Approval != "" {
		eng.Policy.Approval = f.Approval
	}
	eng.Policy.RedactOutput = f.RedactOutput
	return eng, nil
}

// EnsureDefaults ensures guardrails.yaml exists with baseline policy fields.
func EnsureDefaults(paths harness.Paths) error {
	if err := os.MkdirAll(filepath.Dir(paths.GuardrailsFile), 0o755); err != nil {
		return err
	}
	dst := Path(paths)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	def := DefaultPolicy()
	f := File{
		Approval:       def.Approval,
		RedactOutput:   def.RedactOutput,
		DeniedTools:    def.DeniedTools,
		DeniedCommands: def.DeniedCommands,
		SensitivePaths: def.SensitivePaths,
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	header := "# Superopen guardrails. These are shared project safety rules enforced at coding-agent hook boundaries.\n# Authoritative project policy updated by project maintainers; so sync will not overwrite.\n\n"
	return os.WriteFile(dst, append([]byte(header), data...), 0o644)
}

func (e Engine) deniedCommands() []string {
	return append([]string{}, e.Policy.DeniedCommands...)
}

func (e Engine) deniedTools() []string {
	return append([]string{}, e.Policy.DeniedTools...)
}

// CheckTool enforces exact or wildcard matches against a vendor's tool name.
func (e Engine) CheckTool(tool string) Decision {
	tool = strings.ToLower(strings.TrimSpace(tool))
	if tool == "" {
		return Decision{Allow: true}
	}
	for _, pattern := range e.deniedTools() {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern != "" && wildMatch(pattern, tool) {
			return Decision{Allow: false, Reason: "denied tool pattern", Rule: pattern, Matcher: "tool"}
		}
	}
	return Decision{Allow: true}
}

func (e Engine) sensitivePaths() []string {
	return append([]string{}, e.Policy.SensitivePaths...)
}

func (e Engine) Approval() string {
	a := strings.ToLower(e.Policy.Approval)
	if a == "" {
		return "interactive"
	}
	return a
}

func (e Engine) CheckCommand(cmd string) Decision {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return Decision{Allow: true}
	}
	for _, pat := range e.deniedCommands() {
		if matchGlob(strings.ToLower(pat), strings.ToLower(cmd)) {
			return Decision{Allow: false, Reason: "denied command pattern", Rule: pat, Matcher: "command"}
		}
	}
	return Decision{Allow: true}
}

func (e Engine) CheckPath(path string) Decision {
	path = normalizePath(path)
	if path == "" {
		return Decision{Allow: true}
	}
	for _, pat := range e.sensitivePaths() {
		if matchPath(pat, path) {
			return Decision{Allow: false, Reason: "sensitive path", Rule: pat, Matcher: "path"}
		}
	}
	return Decision{Allow: true}
}

func (e Engine) Explain() map[string]any {
	return map[string]any{
		"rules":           e.Rules,
		"rules_count":     len(e.Rules),
		"approval":        e.Approval(),
		"denied_tools":    e.deniedTools(),
		"denied_commands": e.deniedCommands(),
		"sensitive_paths": e.sensitivePaths(),
		"redact_output":   e.Policy.RedactOutput,
		"tool_count":      len(e.deniedTools()),
		"denied_count":    len(e.deniedCommands()),
		"sensitive_count": len(e.sensitivePaths()),
	}
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	// lexical only - collapse .. without resolving symlinks
	p = filepath.Clean(p)
	// collapse leading //
	for strings.HasPrefix(p, "//") {
		p = "/" + strings.TrimLeft(p, "/")
	}
	return p
}

func matchGlob(pattern, s string) bool {
	// Exact or glob match only - never bare substring. Substring matching
	// false-positives on scripts that *mention* deny patterns (e.g. editing
	// guardrails.yaml or writing docs).
	if !strings.Contains(pattern, "*") {
		nl := string([]byte{10})
		return s == pattern || strings.HasPrefix(s, pattern+" ") || strings.HasSuffix(s, " "+pattern) ||
			strings.Contains(s, " && "+pattern) || strings.Contains(s, " || "+pattern) ||
			strings.Contains(s, "; "+pattern) || strings.Contains(s, nl+pattern)
	}
	return wildMatch(pattern, s)
}

func matchPath(pattern, path string) bool {
	pathLower := strings.ToLower(path)
	patLower := strings.ToLower(pattern)
	base := filepath.Base(path)

	// Full-path glob. Collapse ** to * for the simple wild matcher.
	expanded := strings.ReplaceAll(patLower, "**", "*")
	if wildMatch(expanded, pathLower) {
		return true
	}

	// Basename match only when the pattern's final segment is a real
	// name/glob (e.g. "**/id_rsa", "**/.env.*"). Never treat a bare
	// "*" from patterns like "**/.ssh/**" as matching every file -
	// that previously denied all reads under guardrails.
	basePat := filepath.Base(strings.ReplaceAll(pattern, "**", "*"))
	basePatLower := strings.ToLower(basePat)
	if basePat != "" && basePat != "*" && basePat != "." && basePat != string(filepath.Separator) {
		if wildMatch(basePatLower, strings.ToLower(base)) {
			return true
		}
	}

	// .env style (basename)
	if strings.Contains(patLower, ".env") && (base == ".env" || strings.HasPrefix(base, ".env.")) {
		return true
	}
	// Directory segment match: "/.ssh/" or "/.aws/" anywhere in the path.
	for _, seg := range []string{"/.ssh/", "/.aws/"} {
		if strings.Contains(patLower, strings.Trim(seg, "/")) && strings.Contains(pathLower, seg) {
			return true
		}
	}
	return false
}

func wildMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

func CheckCommandString(paths harness.Paths, cmd string) (Decision, error) {
	eng, err := Load(paths)
	if err != nil {
		return Decision{}, err
	}
	return eng.CheckCommand(cmd), nil
}

func CheckPathString(paths harness.Paths, path string) (Decision, error) {
	eng, err := Load(paths)
	if err != nil {
		return Decision{}, err
	}
	return eng.CheckPath(path), nil
}

func FormatDecision(d Decision) string {
	if d.Allow {
		return "allow"
	}
	return fmt.Sprintf("deny (%s: %s)", d.Matcher, d.Rule)
}
