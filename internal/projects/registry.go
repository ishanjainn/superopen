// Package projects maintains a global registry of Superopen roots
// so one UI/CLI can browse sessions across multiple local clones.
package projects

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const fileName = "projects.json"

// Project is one registered repo/.so pair.
type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	RepoRoot   string    `json:"repo_root"`
	SoRoot     string    `json:"so_root"`
	RemoteURL  string    `json:"remote_url,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type fileShape struct {
	Projects        []Project `json:"projects"`
	ActiveProjectID string    `json:"active_project_id,omitempty"`
}

var (
	mu       sync.Mutex
	override string // test hook: absolute path to projects.json
)

// SetPathForTest redirects the registry file (tests only).
func SetPathForTest(path string) {
	mu.Lock()
	defer mu.Unlock()
	override = path
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

// Path returns the absolute path to projects.json.
func Path() (string, error) {
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

func load() (fileShape, error) {
	path, err := Path()
	if err != nil {
		return fileShape{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileShape{}, nil
		}
		return fileShape{}, err
	}
	var f fileShape
	if err := json.Unmarshal(data, &f); err != nil {
		return fileShape{}, err
	}
	return f, nil
}

func save(f fileShape) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func idFor(repoRoot string) string {
	sum := sha1.Sum([]byte(filepath.Clean(repoRoot)))
	return hex.EncodeToString(sum[:8])
}

// Register upserts a project by repo root. soRoot defaults to repoRoot/.so.
func Register(repoRoot, soRoot, remoteURL string) (Project, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Project{}, err
	}
	if soRoot == "" {
		soRoot = filepath.Join(repoRoot, ".so")
	} else {
		soRoot, err = filepath.Abs(soRoot)
		if err != nil {
			return Project{}, err
		}
	}
	f, err := load()
	if err != nil {
		return Project{}, err
	}
	now := time.Now().UTC()
	id := idFor(repoRoot)
	name := filepath.Base(repoRoot)
	p := Project{
		ID:         id,
		Name:       name,
		RepoRoot:   repoRoot,
		SoRoot:     soRoot,
		RemoteURL:  remoteURL,
		LastSeenAt: now,
	}
	found := false
	for i := range f.Projects {
		if f.Projects[i].RepoRoot == repoRoot || f.Projects[i].ID == id {
			if f.Projects[i].RemoteURL != "" && remoteURL == "" {
				p.RemoteURL = f.Projects[i].RemoteURL
			}
			f.Projects[i] = p
			found = true
			break
		}
	}
	if !found {
		f.Projects = append(f.Projects, p)
	}
	if f.ActiveProjectID == "" {
		f.ActiveProjectID = id
	}
	if err := save(f); err != nil {
		return Project{}, err
	}
	return p, nil
}

// Unregister removes a project by id or repo root.
func Unregister(idOrRoot string) error {
	_, err := Remove(idOrRoot, RemoveOptions{})
	return err
}

// RemoveOptions controls destructive cleanup when removing a project.
type RemoveOptions struct {
	// PurgeSO deletes the project's .so directory after unregistering.
	PurgeSO bool
}

// RemoveResult describes what Remove did.
type RemoveResult struct {
	Project      Project `json:"project"`
	Unregistered bool    `json:"unregistered"`
	PurgedSO     bool    `json:"purged_so"`
	SOPath       string  `json:"so_path,omitempty"`
	RepoMissing  bool    `json:"repo_missing"`
}

// Remove unregisters a project and optionally deletes its .so data.
// Safe when the repo directory is already gone - still drops projects.json.
func Remove(idOrRoot string, opts RemoveOptions) (RemoveResult, error) {
	f, err := load()
	if err != nil {
		return RemoveResult{}, err
	}
	var found *Project
	out := make([]Project, 0, len(f.Projects))
	for _, p := range f.Projects {
		if p.ID == idOrRoot || p.RepoRoot == idOrRoot || p.Name == idOrRoot {
			cp := p
			found = &cp
			continue
		}
		out = append(out, p)
	}
	if found == nil {
		return RemoveResult{}, fmt.Errorf("project not found: %s", idOrRoot)
	}

	res := RemoveResult{
		Project:      *found,
		Unregistered: true,
		SOPath:       found.SoRoot,
		RepoMissing:  !pathExists(found.RepoRoot),
	}

	f.Projects = out
	if f.ActiveProjectID == found.ID || f.ActiveProjectID == idOrRoot {
		f.ActiveProjectID = ""
		if len(f.Projects) > 0 {
			f.ActiveProjectID = f.Projects[0].ID
		}
	}
	if err := save(f); err != nil {
		return RemoveResult{}, err
	}

	if opts.PurgeSO {
		soRoot := strings.TrimSpace(found.SoRoot)
		if soRoot == "" {
			soRoot = filepath.Join(found.RepoRoot, ".so")
		}
		if err := purgeSODir(soRoot); err != nil {
			return res, fmt.Errorf("unregistered, but failed to purge .so: %w", err)
		}
		res.PurgedSO = true
		res.SOPath = soRoot
	}
	return res, nil
}

// PruneMissing unregisters every project whose repo_root no longer exists.
// With purgeSO, also deletes leftover .so dirs when still present.
func PruneMissing(purgeSO bool) ([]RemoveResult, error) {
	list, err := List()
	if err != nil {
		return nil, err
	}
	var out []RemoveResult
	for _, p := range list {
		if pathExists(p.RepoRoot) {
			continue
		}
		res, err := Remove(p.ID, RemoveOptions{PurgeSO: purgeSO})
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

func pathExists(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// purgeSODir deletes a Superopen .so directory. Refuses paths that are not
// clearly a ".so" folder to avoid wiping an unrelated tree.
func purgeSODir(soRoot string) error {
	soRoot = filepath.Clean(soRoot)
	base := filepath.Base(soRoot)
	if base != ".so" {
		return fmt.Errorf("refusing to delete non-.so path: %s", soRoot)
	}
	if _, err := os.Stat(soRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(soRoot)
}

// List returns all registered projects (most recently seen first).
func List() ([]Project, error) {
	f, err := load()
	if err != nil {
		return nil, err
	}
	sort.Slice(f.Projects, func(i, j int) bool {
		return f.Projects[i].LastSeenAt.After(f.Projects[j].LastSeenAt)
	})
	return f.Projects, nil
}

// Get returns a project by id or repo root.
func Get(idOrRoot string) (Project, error) {
	f, err := load()
	if err != nil {
		return Project{}, err
	}
	for _, p := range f.Projects {
		if p.ID == idOrRoot || p.RepoRoot == idOrRoot || p.Name == idOrRoot {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("project not found: %s", idOrRoot)
}

// Active returns the active project, or empty if none.
func Active() (Project, error) {
	f, err := load()
	if err != nil {
		return Project{}, err
	}
	if f.ActiveProjectID == "" {
		if len(f.Projects) == 0 {
			return Project{}, fmt.Errorf("no projects registered")
		}
		return f.Projects[0], nil
	}
	for _, p := range f.Projects {
		if p.ID == f.ActiveProjectID {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("active project missing")
}

// Use sets the active project id.
func Use(idOrRoot string) (Project, error) {
	p, err := Get(idOrRoot)
	if err != nil {
		return Project{}, err
	}
	f, err := load()
	if err != nil {
		return Project{}, err
	}
	f.ActiveProjectID = p.ID
	if err := save(f); err != nil {
		return Project{}, err
	}
	return p, nil
}

// ResolvePaths returns projects matching filter.
// empty / "all" → all; otherwise id/name/root match.
func ResolveFilter(filter string) ([]Project, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == "all" {
		return all, nil
	}
	p, err := Get(filter)
	if err != nil {
		return nil, err
	}
	return []Project{p}, nil
}
