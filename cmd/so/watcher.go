package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/sync"
)

// startRefreshWatcher polls git HEAD + shared .so dirs and runs sync.Refresh on change.
func startRefreshWatcher(root string) {
	go func() {
		paths := harness.Resolve(root)
		statusPath := filepath.Join(paths.MemoryDir, "refresh-status.json")
		var lastSHA string
		var lastMtime int64
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var pending bool
		var lastFire time.Time
		for range ticker.C {
			sha := ""
			if out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
				sha = strings.TrimSpace(string(out))
			}
			mt := maxSharedMtime(paths)
			changed := (sha != "" && sha != lastSHA) || mt > lastMtime
			if lastSHA == "" && lastMtime == 0 {
				lastSHA, lastMtime = sha, mt
				continue
			}
			if changed {
				pending = true
				lastSHA, lastMtime = sha, mt
			}
			if pending && time.Since(lastFire) >= 2*time.Second {
				pending = false
				lastFire = time.Now()
				// Never rewrite tracked injectors from the watcher — a HEAD
				// bump after commit would otherwise dirty AGENTS.md / skills
				// while `so dev` is running. Explicit `so sync` still injects.
				err := sync.Refresh(sync.RefreshOptions{RepoRoot: root, SkipInject: true})
				writeRefreshStatus(statusPath, err)
			}
		}
	}()
}

func maxSharedMtime(paths harness.Paths) int64 {
	var best int64
	touch := func(path string) {
		if info, err := os.Stat(path); err == nil {
			if t := info.ModTime().Unix(); t > best {
				best = t
			}
		}
	}
	touch(paths.AgentsMD)
	for _, d := range []string{paths.RulesDir, paths.SkillsDir, paths.GuardrailsDir, paths.EvalsDir} {
		_ = filepath.Walk(d, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if t := info.ModTime().Unix(); t > best {
				best = t
			}
			return nil
		})
	}
	return best
}

func writeRefreshStatus(path string, err error) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	msg := "ok"
	if err != nil {
		msg = err.Error()
	}
	data, _ := json.Marshal(map[string]any{
		"at":      time.Now().UTC().Format(time.RFC3339),
		"ok":      err == nil,
		"message": msg,
	})
	_ = os.WriteFile(path, data, 0o644)
}
