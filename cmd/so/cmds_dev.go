package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/watch"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/projects"
)

func cmdDev() *cobra.Command {
	var uiPort int
	var detach bool
	var noOpen bool
	var hot bool

	c := &cobra.Command{
		Use:   "dev",
		Short: "Start the Superopen file-backed Sessions UI",
		Long: `Start the local Superopen UI from the installed prefix
(~/.superopen/share/superopen/web or Homebrew's share/superopen/web).
Release installs are a prebuilt Next standalone bundle; so dev runs
node server.js (Node.js required at runtime, not npm).

  so dev              # foreground (Ctrl+C to stop); opens the UI
  so dev -d           # detached (background); opens the UI when ready
  so dev --hot        # next dev (needs UI sources via SUPEROPEN_WEB_DIR)
  so dev stop         # stop a detached (or any tracked) UI
  so dev status       # show if the UI is running`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev(uiPort, detach, noOpen, hot)
		},
	}
	c.Flags().IntVar(&uiPort, "ui-port", 4444, "Superopen UI port")
	c.Flags().BoolVarP(&detach, "detach", "d", false, "Run in the background (like docker -d)")
	c.Flags().BoolVar(&noOpen, "no-open", false, "Do not open the UI in a browser when ready")
	c.Flags().BoolVar(&hot, "hot", false, "Serve with next dev (on-demand compile, HMR) for UI work")

	c.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop a detached Superopen UI started with so dev -d",
		RunE: func(cmd *cobra.Command, args []string) error {
			return stopDev()
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show whether the Superopen UI is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusDev(uiPort)
		},
	})
	return c
}

func runDir(layout paths.Paths) string {
	dir, err := paths.RuntimeDir(layout.RepoRoot)
	if err != nil {
		return filepath.Join(os.TempDir(), "superopen-runtime")
	}
	return dir
}

func pidFile(layout paths.Paths) string {
	return filepath.Join(runDir(layout), "dev.pid")
}

func logFile(layout paths.Paths) string {
	return filepath.Join(runDir(layout), "dev.log")
}

func runDev(uiPort int, detach, noOpen, hot bool) error {
	explicit := strings.TrimSpace(cliFlags.Root)
	if explicit == "" {
		explicit = strings.TrimSpace(os.Getenv("SUPEROPEN_ROOT"))
	}
	if explicit != "" {
		explicit = absPath(explicit)
	}
	wd, _ := os.Getwd()
	root, err := projects.ResolveDevRoot(explicit, wd)
	if err != nil {
		return err
	}
	layout := paths.Resolve(root)
	if err := os.MkdirAll(runDir(layout), 0o755); err != nil {
		return err
	}

	if detach {
		return startDevDetached(root, layout, uiPort, noOpen, hot)
	}
	return runDevForeground(root, layout, uiPort, noOpen, hot)
}

func maybeOpenUI(url string, noOpen bool) {
	if noOpen || os.Getenv("SO_DEV_NO_OPEN") == "1" {
		return
	}
	// Detached child re-execs `so dev` without -d; only the parent should open.
	if os.Getenv("SO_DEV_DAEMON") == "1" {
		return
	}
	if err := openBrowser(url); err != nil {
		fmt.Fprintf(os.Stderr, "note: could not open browser: %v\n", err)
	}
}

func startDevDetached(root string, layout paths.Paths, uiPort int, noOpen, hot bool) error {
	url := fmt.Sprintf("http://127.0.0.1:%d", uiPort)
	if st, err := readDevStatus(layout, uiPort); err == nil && st.alive {
		fmt.Printf("Already running (pid %d) at %s\n", st.pid, st.url)
		fmt.Printf("Stop with: so dev stop\n")
		maybeOpenUI(url+"/sessions", noOpen)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := logFile(layout)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	args := []string{"dev", "--ui-port", strconv.Itoa(uiPort), "--no-open"}
	if hot {
		args = append(args, "--hot")
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "SO_DEV_DAEMON=1")
	cmd.Stdout = f
	cmd.Stderr = f
	setDetachedProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return fmt.Errorf("start detached: %w", err)
	}
	_ = f.Close()

	if err := os.WriteFile(pidFile(layout), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644); err != nil {
		_ = cmd.Process.Kill()
		return err
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(cmd.Process.Pid) {
			return fmt.Errorf("dev process exited early; see %s", logPath)
		}
		resp, err := http.Get(url + "/sessions")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				fmt.Printf("Superopen UI %s (pid %d)\n", url, cmd.Process.Pid)
				fmt.Printf("Logs: %s\n", logPath)
				fmt.Printf("Stop: so dev stop\n")
				maybeOpenUI(url+"/sessions", noOpen)
				return nil
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for UI on %s; see %s", url, logPath)
}

func runDevForeground(root string, layout paths.Paths, uiPort int, noOpen, hot bool) error {
	_, _ = projects.Register(root, layout.Root, "")
	var runner *watch.Runner
	if graphClient, err := client.Resolve(); err == nil {
		runner = &watch.Runner{Root: root, Client: graphClient}
		runner.Start(context.Background())
		defer runner.Stop()
	}
	fmt.Println("Live graph refresh active (local git poll ~60s; no LLM).")
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			_ = memory.SleepRoot(root)
		}
	}()

	nextCmd, nextURL, err := startNextUI(root, uiPort, hot)
	if err != nil {
		return fmt.Errorf("Next.js UI: %w", err)
	}
	mode := "prebuilt"
	if hot {
		mode = "hot / Turbopack"
	}
	fmt.Printf("Superopen UI %s (%s)\n", nextURL, mode)
	maybeOpenUI(nextURL+"/sessions", noOpen)

	// Track foreground runs too so `so dev stop` works from another shell.
	if nextCmd.Process != nil {
		_ = os.WriteFile(pidFile(layout), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
	}

	defer func() {
		if nextCmd.Process != nil {
			_ = nextCmd.Process.Kill()
		}
		_ = os.Remove(pidFile(layout))
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("\nShutting down…")
	return nil
}

func stopDev() error {
	root := repoRoot()
	layout := paths.Resolve(root)
	pf := pidFile(layout)
	raw, err := os.ReadFile(pf)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No Superopen UI pid file - nothing to stop.")
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		_ = os.Remove(pf)
		return fmt.Errorf("invalid pid file %s", pf)
	}
	if !processAlive(pid) {
		_ = os.Remove(pf)
		fmt.Println("UI was not running (stale pid file removed).")
		return nil
	}
	if err := killProcessTree(pid); err != nil {
		return fmt.Errorf("stop pid %d: %w", pid, err)
	}
	_ = os.Remove(pf)
	fmt.Printf("Stopped Superopen UI (pid %d)\n", pid)
	return nil
}

type devStatus struct {
	pid   int
	url   string
	alive bool
}

func readDevStatus(layout paths.Paths, uiPort int) (devStatus, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d", uiPort)
	st := devStatus{url: url}
	raw, err := os.ReadFile(pidFile(layout))
	if err != nil {
		return st, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return st, fmt.Errorf("bad pid")
	}
	st.pid = pid
	st.alive = processAlive(pid)
	if !st.alive {
		return st, fmt.Errorf("not alive")
	}
	return st, nil
}

func statusDev(uiPort int) error {
	root := repoRoot()
	layout := paths.Resolve(root)
	url := fmt.Sprintf("http://127.0.0.1:%d", uiPort)

	st, err := readDevStatus(layout, uiPort)
	reachable := false
	if resp, herr := http.Get(url + "/sessions"); herr == nil {
		_ = resp.Body.Close()
		reachable = resp.StatusCode < 500
	}

	switch {
	case err == nil && st.alive && reachable:
		fmt.Printf("running  pid=%d  %s\n", st.pid, url)
	case reachable:
		fmt.Printf("reachable  %s  (no tracked pid)\n", url)
	case err == nil && st.alive:
		fmt.Printf("pid %d alive but UI not reachable yet (%s)\n", st.pid, url)
	default:
		fmt.Println("stopped")
	}
	return nil
}
