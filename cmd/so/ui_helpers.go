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
			path := "/graph"
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

func findWebDir(_ string) string {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("SUPEROPEN_WEB_DIR")); configured != "" {
		candidates = append(candidates, configured)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(dir, "..", "share", "superopen", "web"),
			filepath.Join(dir, "web"),
		)
	}
	for _, candidate := range candidates {
		if isStandaloneWeb(candidate) || webFileExists(candidate, "package.json") {
			return candidate
		}
	}
	return ""
}

func expectedWebDir() string {
	if executable, err := os.Executable(); err == nil {
		return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "share", "superopen", "web"))
	}
	return "~/.superopen/share/superopen/web"
}

func webFileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func isStandaloneWeb(webDir string) bool {
	return webFileExists(webDir, "server.js")
}

func hasWebSources(webDir string) bool {
	return webFileExists(webDir, "next.config.mjs") || webFileExists(webDir, "next.config.ts") || webFileExists(webDir, "next.config.js")
}

func npmCommand(args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", append([]string{"/d", "/s", "/c", "npm.cmd"}, args...)...)
	}
	return exec.Command("npm", args...)
}

// newestModTime is the latest mtime under dir, ignoring build and dep output.
func newestModTime(dir string) time.Time {
	var newest time.Time
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == "node_modules" || name == ".next" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// buildStale reports whether webDir has sources newer than its last build.
func buildStale(webDir string) bool {
	built, err := os.Stat(filepath.Join(webDir, ".next", "BUILD_ID"))
	if err != nil {
		return true
	}
	return newestModTime(webDir).After(built.ModTime())
}

func buildNextUI(webDir, repoRoot string) error {
	command := npmCommand("run", "build")
	command.Dir = webDir
	binary, _ := os.Executable()
	command.Env = append(os.Environ(), "SUPEROPEN_ROOT="+repoRoot, "SUPEROPEN_SO_BIN="+binary, "NODE_ENV=production")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func uiServeCommand(webDir, repoRoot, binary string, port int, hot bool) (*exec.Cmd, error) {
	if hot {
		if !hasWebSources(webDir) {
			return nil, fmt.Errorf("so dev --hot needs UI sources; set SUPEROPEN_WEB_DIR to a Superopen checkout's web/ directory")
		}
		if !webFileExists(webDir, "node_modules") {
			return nil, fmt.Errorf("web dependencies missing; run npm install --ignore-scripts in %s", webDir)
		}
		command := npmCommand("run", "dev", "--", "-p", strconv.Itoa(port), "-H", "127.0.0.1")
		command.Dir = webDir
		command.Env = append(os.Environ(), "SUPEROPEN_ROOT="+repoRoot, "SUPEROPEN_SO_BIN="+binary, "NODE_ENV=development")
		return command, nil
	}
	if isStandaloneWeb(webDir) {
		command := exec.Command("node", "server.js")
		command.Dir = webDir
		command.Env = append(os.Environ(),
			"SUPEROPEN_ROOT="+repoRoot,
			"SUPEROPEN_SO_BIN="+binary,
			"NODE_ENV=production",
			"PORT="+strconv.Itoa(port),
			"HOSTNAME=127.0.0.1",
		)
		return command, nil
	}
	if !webFileExists(webDir, "node_modules") {
		return nil, fmt.Errorf("web dependencies missing; run npm install --ignore-scripts in %s", webDir)
	}
	command := npmCommand("run", "start", "--", "-p", strconv.Itoa(port), "-H", "127.0.0.1")
	command.Dir = webDir
	command.Env = append(os.Environ(), "SUPEROPEN_ROOT="+repoRoot, "SUPEROPEN_SO_BIN="+binary, "NODE_ENV=production")
	return command, nil
}

// startNextUI serves the installed UI. Curl/brew prefixes contain a Next
// standalone bundle (`node server.js`). A source checkout can still use
// `next start` or `so dev --hot` (next dev).
func startNextUI(repoRoot string, port int, hot bool) (*exec.Cmd, string, error) {
	webDir := findWebDir(repoRoot)
	if webDir == "" {
		return nil, "", fmt.Errorf("web UI missing from the Superopen prefix (%s); re-run the installer", expectedWebDir())
	}
	if !hot && !isStandaloneWeb(webDir) && buildStale(webDir) {
		fmt.Println("Building the UI once (first run after a change)…")
		if err := buildNextUI(webDir, repoRoot); err != nil {
			return nil, "", fmt.Errorf("build UI: %w", err)
		}
	}
	if isStandaloneWeb(webDir) && !hot {
		if _, err := exec.LookPath("node"); err != nil {
			return nil, "", fmt.Errorf("node is required to run the Superopen UI; install Node.js then retry")
		}
	}
	binary, _ := os.Executable()
	command, err := uiServeCommand(webDir, repoRoot, binary, port, hot)
	if err != nil {
		return nil, "", err
	}
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Start(); err != nil {
		return nil, "", err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if response, err := http.Get(url + "/graph"); err == nil {
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
