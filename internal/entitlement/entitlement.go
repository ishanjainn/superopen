// Package entitlement gates paid features (OTLP cloud sync) behind CLI auth.
package entitlement

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Status describes paid unlock state.
type Status struct {
	Authenticated bool      `json:"authenticated"`
	Paid          bool      `json:"paid"`
	Email         string    `json:"email,omitempty"`
	Token         string    `json:"token,omitempty"` // persisted locally; never returned by public APIs
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	OTLPEndpoint  string    `json:"otlp_endpoint,omitempty"`
	QueryEndpoint string    `json:"query_endpoint,omitempty"`
}

// Public returns a copy safe for API responses (no token).
func (s Status) Public() Status {
	out := s
	out.Token = ""
	return out
}

const fileName = "auth.json"

var (
	mu       sync.Mutex
	override string
)

func SetPathForTest(p string) {
	mu.Lock()
	defer mu.Unlock()
	override = p
}

func configDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "superopen"), nil
	}
	if runtime.GOOS == "windows" {
		if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
			return filepath.Join(cfg, "superopen"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "superopen"), nil
}

func path() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if override != "" {
		return override, nil
	}
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Load reads auth state from disk.
func Load() (Status, error) {
	p, err := path()
	if err != nil {
		return Status{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Status{}, nil
		}
		return Status{}, err
	}
	var st Status
	if err := json.Unmarshal(data, &st); err != nil {
		return Status{}, err
	}
	if !st.ExpiresAt.IsZero() && time.Now().After(st.ExpiresAt) {
		st.Paid = false
		st.Authenticated = false
	}
	return st, nil
}

// Save persists auth state (0600).
func Save(st Status) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Clear removes auth.
func Clear() error {
	p, err := path()
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// CloudOTLPEnabled is true only with paid+authenticated entitlement.
func CloudOTLPEnabled() bool {
	st, err := Load()
	if err != nil {
		return false
	}
	return st.Authenticated && st.Paid && (st.OTLPEndpoint != "" || st.QueryEndpoint != "")
}

// LoginPaid is used by `so login` when connecting to a paid UI.
// token/endpoints come from the paid product; free CLI never invents them.
func LoginPaid(email, token, otlpEndpoint, queryEndpoint string, expires time.Time) error {
	return Save(Status{
		Authenticated: true,
		Paid:          true,
		Email:         email,
		Token:         token,
		ExpiresAt:     expires,
		OTLPEndpoint:  otlpEndpoint,
		QueryEndpoint: queryEndpoint,
	})
}
