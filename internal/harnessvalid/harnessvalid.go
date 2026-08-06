// Package harnessvalid validates AI-generated harness mutations before write.
package harnessvalid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/superopen/so/internal/guardrails"
	"gopkg.in/yaml.v3"
)

// SoftWrite is a candidate soft-tier mutation (knowledge/rules/skills/memory).
type SoftWrite struct {
	Path     string
	Body     string
	Evidence []string
	// CreateOnly rejects overwrite of non-empty existing files.
	CreateOnly bool
	// AppendOnly means body is an append fragment (path must be relative-safe).
	AppendOnly bool
}

// ValidatePath rejects traversal and absolute paths for relative harness writes.
func ValidatePath(rel string) error {
	rel = filepath.Clean(rel)
	if rel == "." || rel == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(rel) {
		return fmt.Errorf("absolute path not allowed: %s", rel)
	}
	if strings.Contains(rel, "..") {
		return fmt.Errorf("path traversal not allowed: %s", rel)
	}
	return nil
}

// HasEvidence requires at least one non-empty evidence string.
func HasEvidence(evidence []string) bool {
	for _, e := range evidence {
		if strings.TrimSpace(e) != "" {
			return true
		}
	}
	return false
}

// ValidateSoftWrite checks evidence + path safety + create-only policy.
func ValidateSoftWrite(w SoftWrite) error {
	if !HasEvidence(w.Evidence) {
		return fmt.Errorf("missing evidence")
	}
	if strings.TrimSpace(w.Body) == "" && !w.AppendOnly {
		// append-only may still need non-empty body
	}
	if strings.TrimSpace(w.Body) == "" {
		return fmt.Errorf("empty body")
	}
	base := filepath.Base(w.Path)
	dir := filepath.Dir(w.Path)
	// Allow absolute paths under .so when already resolved; still block ..
	if strings.Contains(w.Path, "..") {
		return fmt.Errorf("path traversal not allowed")
	}
	_ = base
	_ = dir
	if w.CreateOnly {
		if info, err := os.Stat(w.Path); err == nil && !info.IsDir() && info.Size() > 0 {
			return fmt.Errorf("refusing to clobber existing file: %s", w.Path)
		}
	}
	return nil
}

// ValidateGuardrailsBody ensures YAML parses as guardrails.File and is not comment-only.
func ValidateGuardrailsBody(body string) (guardrails.File, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return guardrails.File{}, fmt.Errorf("empty guardrails body")
	}
	// Reject comment-only / harvest-note style stubs.
	stripped := stripYAMLComments(body)
	if strings.TrimSpace(stripped) == "" {
		return guardrails.File{}, fmt.Errorf("guardrails body is comment-only")
	}
	var f guardrails.File
	if err := yaml.Unmarshal([]byte(body), &f); err != nil {
		return guardrails.File{}, fmt.Errorf("guardrails yaml: %w", err)
	}
	if len(f.Rules) == 0 && len(f.DeniedCommands) == 0 && len(f.SensitivePaths) == 0 && f.Approval == "" {
		return guardrails.File{}, fmt.Errorf("guardrails body has no rules, denies, paths, or approval")
	}
	// Dry-run: engine load from body
	eng := guardrails.Engine{Policy: guardrails.DefaultPolicy(), Rules: f.Rules}
	if len(f.DeniedCommands) > 0 {
		eng.Policy.DeniedCommands = f.DeniedCommands
	}
	if len(f.SensitivePaths) > 0 {
		eng.Policy.SensitivePaths = f.SensitivePaths
	}
	if f.Approval != "" {
		eng.Policy.Approval = f.Approval
	}
	_ = eng.CheckCommand("echo ok")
	_ = eng.CheckPath("/tmp/safe.txt")
	return f, nil
}

// ValidateEvalsBody ensures evals config YAML/JSON is parseable and non-empty.
func ValidateEvalsBody(body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("empty evals body")
	}
	if strings.HasPrefix(body, "- ") || strings.HasPrefix(body, "* ") {
		return fmt.Errorf("evals body looks like notes, not config")
	}
	var raw any
	if err := yaml.Unmarshal([]byte(body), &raw); err != nil {
		return fmt.Errorf("evals yaml: %w", err)
	}
	if raw == nil {
		return fmt.Errorf("evals body empty after parse")
	}
	return nil
}

// SafeRel under root: join and ensure result stays under root.
func SafeJoin(root, rel string) (string, error) {
	if err := ValidatePath(rel); err != nil {
		return "", err
	}
	full := filepath.Join(root, rel)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absFull, absRoot+string(os.PathSeparator)) && absFull != absRoot {
		return "", fmt.Errorf("path escapes root")
	}
	return full, nil
}

func stripYAMLComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// Tier classifies a recommendation type for auto-apply.
func Tier(recType string) string {
	switch strings.ToLower(strings.TrimSpace(recType)) {
	case "skill", "docs", "graph", "memory", "rules":
		return "soft"
	case "guardrail", "policy":
		return "policy"
	case "eval", "evals":
		return "evals"
	default:
		return "soft"
	}
}

// Applyable reports whether a rec has enough body/evidence for its type.
func Applyable(recType, proposedPath, proposedBody string, evidence []string) error {
	if !HasEvidence(evidence) {
		return fmt.Errorf("missing evidence")
	}
	t := strings.ToLower(recType)
	body := strings.TrimSpace(proposedBody)
	switch t {
	case "docs", "guardrail", "eval", "evals":
		if body == "" {
			return fmt.Errorf("%s recommendation requires proposed_body", t)
		}
		if t == "guardrail" {
			_, err := ValidateGuardrailsBody(body)
			return err
		}
		if t == "eval" || t == "evals" {
			return ValidateEvalsBody(body)
		}
	case "skill":
		if body == "" && proposedPath != "" {
			return fmt.Errorf("skill recommendation requires proposed_body")
		}
	}
	return nil
}
