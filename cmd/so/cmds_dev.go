package main

import (
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

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/userpaths"
)

func cmdDev() *cobra.Command {
	var uiPort int
	var detach bool
	var noOpen bool

	c := &cobra.Command{
		Use:   "dev",
		Short: "Start the Superopen file-backed Sessions UI",
		Long: `Start the local Superopen UI with Next.js in development mode
(Turbopack by default on Next 16) so pages compile on demand with fast HMR.

  so dev              # foreground (Ctrl+C to stop); opens the UI
  so dev -d           # detached (background); opens the UI when ready
  so dev stop         # stop a detached (or any tracked) UI
  so dev status       # show if the UI is running

End-user / release installs can later ship a prebuilt UI; local work
uses next dev (Turbopack).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev(uiPort, detach, noOpen)
		},
	}
	c.Flags().IntVar(&uiPort, "ui-port", 4444, "Superopen UI port")
	c.Flags().BoolVarP(&detach, "detach", "d", false, "Run in the background (like docker -d)")
	c.Flags().BoolVar(&noOpen, "no-open", false, "Do not open the UI in a browser when ready")

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

func runDir(paths harness.Paths) string {
	dir, err := userpaths.RuntimeDir(paths.RepoRoot)
	if err != nil {
		return filepath.Join(os.TempDir(), "superopen-runtime")
	}
	return dir
}

func pidFile(paths harness.Paths) string {
	return filepath.Join(runDir(paths), "dev.pid")
}

func logFile(paths harness.Paths) string {
	return filepath.Join(runDir(paths), "dev.log")
}

func runDev(uiPort int, detach, noOpen bool) error {
	root := repoRoot()
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return fmt.Errorf("run `so init` first")
	}
	if err := os.MkdirAll(runDir(paths), 0o755); err != nil {
		return err
	}

	if detach {
		return startDevDetached(root, paths, uiPort, noOpen)
	}
	return runDevForeground(root, paths, uiPort, noOpen)
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

func startDevDetached(root string, paths harness.Paths, uiPort int, noOpen bool) error {
	url := fmt.Sprintf("http://127.0.0.1:%d", uiPort)
	if st, err := readDevStatus(paths, uiPort); err == nil && st.alive {
		fmt.Printf("Already running (pid %d) at %s\n", st.pid, st.url)
		fmt.Printf("Stop with: so dev stop\n")
		maybeOpenUI(url+"/sessions", noOpen)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := logFile(paths)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	args := []string{"dev", "--ui-port", strconv.Itoa(uiPort), "--no-open"}
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

	if err := os.WriteFile(pidFile(paths), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644); err != nil {
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

func runDevForeground(root string, paths harness.Paths, uiPort int, noOpen bool) error {
	nextCmd, nextURL, err := startNextUI(root, uiPort)
	if err != nil {
		return fmt.Errorf("Next.js UI: %w", err)
	}
	fmt.Printf("Superopen UI %s (dev / Turbopack)\n", nextURL)
	maybeOpenUI(nextURL+"/sessions", noOpen)

	// Track foreground runs too so `so dev stop` works from another shell.
	if nextCmd.Process != nil {
		_ = os.WriteFile(pidFile(paths), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
	}

	defer func() {
		if nextCmd.Process != nil {
			_ = nextCmd.Process.Kill()
		}
		_ = os.Remove(pidFile(paths))
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("\nShutting down…")
	return nil
}

func stopDev() error {
	root := repoRoot()
	paths := harness.Resolve(root)
	pf := pidFile(paths)
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

func readDevStatus(paths harness.Paths, uiPort int) (devStatus, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d", uiPort)
	st := devStatus{url: url}
	raw, err := os.ReadFile(pidFile(paths))
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
	paths := harness.Resolve(root)
	url := fmt.Sprintf("http://127.0.0.1:%d", uiPort)

	st, err := readDevStatus(paths, uiPort)
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
