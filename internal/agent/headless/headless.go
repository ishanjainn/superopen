// Package headless probes authenticated coding-agent CLIs for one-shot
// session distill and harvest. Graph refresh never waits on this package.
package headless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/identity"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

const (
	// EnvIsolated marks a worker process so Superopen hooks no-op.
	EnvIsolated = "SUPEROPEN_HEADLESS"

	WorkerDistillPrefix = "You write Superopen memory"
	WorkerHarvestPrefix = "You propose playbook patches"

	maxProviderFails = 2
)

type Provider struct {
	Name string
	Bin  string
	Args []string
}

// Isolated is true when this process is a distill/harvest worker.
func Isolated() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvIsolated)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// HasOneShot is true when vendor has a one-shot CLI (not Cursor/Copilot/Gemini).
func HasOneShot(vendor string) bool {
	switch canonicalVendor(vendor) {
	case "claude-code", "opencode", "codex", "pi":
		return true
	default:
		return false
	}
}

// AvailableLive returns the one-shot CLI for this session vendor only.
// There is no cross-vendor fallback: a Cursor session never launches claude.
func AvailableLive(vendor string) (Provider, bool) {
	probe := probeForVendor(vendor)
	if probe == nil {
		return Provider{}, false
	}
	return probe()
}

func probeForVendor(vendor string) func() (Provider, bool) {
	switch canonicalVendor(vendor) {
	case "claude-code":
		return claude
	case "opencode":
		return opencode
	case "codex":
		return codex
	case "pi":
		return pi
	default:
		return nil
	}
}

func canonicalVendor(vendor string) string {
	v := strings.ToLower(strings.TrimSpace(vendor))
	switch v {
	case "cc", "claude", "claudecode":
		return "claude-code"
	default:
		return v
	}
}

func Run(ctx context.Context, p Provider, prompt string) (string, error) {
	if strings.TrimSpace(p.Bin) == "" {
		return "", fmt.Errorf("headless provider missing binary")
	}
	args := append(append([]string{}, p.Args...), prompt)
	cmd := exec.CommandContext(ctx, p.Bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = bytes.NewReader(nil)
	cmd.Env = append(os.Environ(), EnvIsolated+"=1")
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		} else {
			msg = err.Error() + ": " + clip(msg, 240)
		}
		return "", fmt.Errorf("%s: %s", p.Name, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func claude() (Provider, bool) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return Provider{}, false
	}
	if identity.ResolveForVendor("claude-code") == "" && !fileHasJSONKey(filepath.Join(home(), ".claude.json"), "oauthAccount") {
		return Provider{}, false
	}
	return Provider{Name: "claude", Bin: bin, Args: []string{"-p", "--output-format", "text"}}, true
}

func codex() (Provider, bool) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return Provider{}, false
	}
	homeDir, err := paths.CodexHome()
	if err != nil {
		return Provider{}, false
	}
	auth := filepath.Join(homeDir, "auth.json")
	if !fileHasJSONKey(auth, "tokens") && identity.ResolveForVendor("codex") == "" {
		return Provider{}, false
	}
	return Provider{Name: "codex", Bin: bin, Args: []string{"exec"}}, true
}

func opencode() (Provider, bool) {
	bin, err := exec.LookPath("opencode")
	if err != nil {
		return Provider{}, false
	}
	cfg, err := paths.OpenCodeConfigDir()
	if err != nil {
		return Provider{}, false
	}
	if !exists(filepath.Join(cfg, "auth.json")) && !exists(filepath.Join(cfg, "oh-my-opencode.json")) {
		data, dataErr := paths.OpenCodeDataDir()
		if dataErr != nil || !exists(filepath.Join(data, "auth.json")) {
			return Provider{}, false
		}
	}
	return Provider{Name: "opencode", Bin: bin, Args: []string{"run"}}, true
}

func pi() (Provider, bool) {
	bin, err := exec.LookPath("pi")
	if err != nil {
		return Provider{}, false
	}
	if !exists(filepath.Join(home(), ".pi", "agent")) && !exists(filepath.Join(home(), ".pi", "config.json")) {
		return Provider{}, false
	}
	return Provider{Name: "pi", Bin: bin, Args: []string{"-p"}}, true
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileHasJSONKey(path, key string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var obj map[string]any
	if json.Unmarshal(data, &obj) != nil {
		return false
	}
	_, ok := obj[key]
	return ok
}

func clip(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// WorkerFingerprint is true for distill/harvest worker sessions.
func WorkerFingerprint(title, preview, model string) bool {
	if strings.EqualFold(strings.TrimSpace(model), "<synthetic>") {
		return true
	}
	blob := title + "\n" + preview
	return strings.Contains(blob, WorkerDistillPrefix) || strings.Contains(blob, WorkerHarvestPrefix)
}

func LoadMeta(root, sessionID string) (session.Meta, error) {
	return session.NewStore(paths.Resolve(root)).Get(strings.TrimSpace(sessionID))
}

// SkipWorker reports why a session must not spawn distill/harvest.
func SkipWorker(root, sessionID string) (string, bool) {
	if Isolated() {
		return "worker-env", true
	}
	meta, err := LoadMeta(root, sessionID)
	if err != nil {
		return "", false
	}
	if WorkerFingerprint(meta.Title, meta.PromptPreview, meta.Model) {
		return "worker-session", true
	}
	return "", false
}

// SessionEndPolicy is the finalize spawn decision: live vendor only.
// spawn=false with pending=true means mark pending for the next live SessionStart.
func SessionEndPolicy(root, sessionID string) (skipped string, pending, spawn bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "no-session", false, false
	}
	if reason, ok := SkipWorker(root, sessionID); ok {
		return reason, false, false
	}
	vendor := ""
	if meta, err := LoadMeta(root, sessionID); err == nil {
		vendor = meta.Vendor
	}
	if !HasOneShot(vendor) {
		return "await-live", true, false
	}
	if _, ok := AvailableLive(vendor); !ok {
		return "no-auth", true, false
	}
	return "", false, true
}

func MaxProviderFails() int { return maxProviderFails }

func LockPath(root, name string) string {
	return filepath.Join(paths.Resolve(root).DBDir, name+".lock")
}
