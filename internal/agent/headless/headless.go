// Package headless probes authenticated coding-agent CLIs for one-shot
// session distill. Graph refresh never waits on this package.
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
)

type Provider struct {
	Name string
	Bin  string
	Args []string
}

// Available returns the first authenticated headless CLI. Cursor and Copilot
// have no -p equivalent; a machine with Claude/Codex/OpenCode/Pi logged in
// can still distill Cursor sessions.
func Available() (Provider, bool) {
	for _, probe := range []func() (Provider, bool){claude, codex, opencode, pi} {
		if p, ok := probe(); ok {
			return p, true
		}
	}
	return Provider{}, false
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
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
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
