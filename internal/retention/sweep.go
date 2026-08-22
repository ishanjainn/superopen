package retention

import (
	"strconv"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/config"
	"github.com/ishanjainn/superopen/internal/agent/sessionstate"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

// Result is the AXI payload for `so gc`.
type Result struct {
	SessionHours       int      `json:"session_hours"`
	MemoryHours        int      `json:"memory_hours"`
	SessionsDeleted    []string `json:"sessions_deleted"`
	MemoriesDeleted    int      `json:"memories_deleted"`
	SessionKeepForever bool     `json:"session_keep_forever"`
	MemoryKeepForever  bool     `json:"memory_keep_forever"`
}

// Settings is the current retention policy (hours; 0 = forever).
type Settings struct {
	SessionHours int `json:"session_hours"`
	MemoryHours  int `json:"memory_hours"`
}

func LoadSettings() (Settings, error) {
	_ = config.PromoteFileToEnv()
	cfg, err := config.Load(nil)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		SessionHours: cfg.SessionRetentionHours,
		MemoryHours:  cfg.MemoryRetentionHours,
	}, nil
}

func SaveSettings(next Settings) (Settings, error) {
	if next.SessionHours < 0 {
		next.SessionHours = config.DefaultRetentionHours
	}
	if next.MemoryHours < 0 {
		next.MemoryHours = config.DefaultRetentionHours
	}
	if _, err := config.Save(map[string]string{
		config.EnvSessionRetentionHours: strconv.Itoa(next.SessionHours),
		config.EnvMemoryRetentionHours:  strconv.Itoa(next.MemoryHours),
	}); err != nil {
		return Settings{}, err
	}
	return LoadSettings()
}

// Sweep deletes expired sessions and unpinned memories for one managed repo.
func Sweep(root string) (Result, error) {
	settings, err := LoadSettings()
	if err != nil {
		return Result{}, err
	}
	out := Result{
		SessionHours:       settings.SessionHours,
		MemoryHours:        settings.MemoryHours,
		SessionKeepForever: settings.SessionHours == 0,
		MemoryKeepForever:  settings.MemoryHours == 0,
		SessionsDeleted:    []string{},
	}
	layout := paths.Resolve(root)
	if !layout.Exists() {
		return out, nil
	}
	now := time.Now().UTC()
	if d := config.HoursDuration(settings.SessionHours); d > 0 {
		deleted, err := session.NewStore(layout).DeleteOlderThan(now.Add(-d))
		if err != nil {
			return out, err
		}
		out.SessionsDeleted = deleted
		sessionstate.GC(d)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		return out, nil
	}
	defer store.Close()
	n, err := store.DeleteUnprotectedForSessions(out.SessionsDeleted)
	if err != nil {
		return out, err
	}
	out.MemoriesDeleted += n
	if d := config.HoursDuration(settings.MemoryHours); d > 0 {
		n, err := store.DeleteExpired(now.Add(-d))
		if err != nil {
			return out, err
		}
		out.MemoriesDeleted += n
	}
	return out, nil
}
