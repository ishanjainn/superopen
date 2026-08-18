package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func cmdOpen() *cobra.Command {
	return &cobra.Command{
		Use:   "open [sessions|graph]",
		Short: "Open the local Superopen UI",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := "/sessions"
			if len(args) == 1 {
				selected := strings.Trim(strings.TrimSpace(args[0]), "/")
				if selected != "sessions" && selected != "graph" {
					return fmt.Errorf("open supports sessions or graph")
				}
				path = "/" + selected
			}
			url := "http://127.0.0.1:4444" + path
			client := &http.Client{Timeout: 800 * time.Millisecond}
			response, err := client.Get("http://127.0.0.1:4444/api/meta")
			if err != nil {
				return fmt.Errorf("UI not reachable; run `so dev` first: %w", err)
			}
			_ = response.Body.Close()
			return openBrowser(url)
		},
	}
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		command = exec.Command("xdg-open", url)
	default:
		fmt.Println(url)
		return nil
	}
	return command.Start()
}

func findWebDir(repoRoot string) string {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("SUPEROPEN_WEB_DIR")); configured != "" {
		candidates = append(candidates, configured)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(dir, "web"), filepath.Join(dir, "..", "share", "superopen", "web"))
	}
	candidates = append(candidates, filepath.Join(repoRoot, "web"))
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "package.json")); err == nil {
			return candidate
		}
	}
	return ""
}

func npmCommand(args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", append([]string{"/d", "/s", "/c", "npm.cmd"}, args...)...)
	}
	return exec.Command("npm", args...)
}

func startNextUI(repoRoot string, port int) (*exec.Cmd, string, error) {
	webDir := findWebDir(repoRoot)
	if webDir == "" {
		return nil, "", fmt.Errorf("superopen web UI not found")
	}
	if _, err := os.Stat(filepath.Join(webDir, "node_modules")); err != nil {
		return nil, "", fmt.Errorf("web dependencies missing; run npm install --ignore-scripts in %s", webDir)
	}
	command := npmCommand("run", "dev", "--", "-p", strconv.Itoa(port), "-H", "127.0.0.1")
	command.Dir = webDir
	binary, _ := os.Executable()
	command.Env = append(os.Environ(), "SUPEROPEN_ROOT="+repoRoot, "SUPEROPEN_SO_BIN="+binary, "NODE_ENV=development")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Start(); err != nil {
		return nil, "", err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if response, err := http.Get(url + "/sessions"); err == nil {
			_ = response.Body.Close()
			if response.StatusCode < 500 {
				return command, url, nil
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	_ = command.Process.Kill()
	return nil, "", fmt.Errorf("timed out waiting for UI on %s", url)
}
