package discover

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StackSignals are filesystem/dependency facts used to rank automation candidates.
type StackSignals struct {
	Manifests    []string `json:"manifests,omitempty"`
	Deps         []string `json:"deps,omitempty"`
	Configs      []string `json:"configs,omitempty"`
	Dirs         []string `json:"dirs,omitempty"`
	GitRemote    string   `json:"git_remote,omitempty"`
	HasEnvFiles  bool     `json:"has_env_files,omitempty"`
	HasLockfiles bool     `json:"has_lockfiles,omitempty"`
}

// DetectSignals scans the repo for manifests, deps, tool configs, and remotes.
func DetectSignals(repoRoot string) StackSignals {
	sig := StackSignals{}
	for _, name := range []string{"go.mod", "package.json", "pyproject.toml", "Cargo.toml", "pom.xml"} {
		if fileExists(filepath.Join(repoRoot, name)) {
			sig.Manifests = append(sig.Manifests, name)
		}
	}
	sig.Deps = detectDeps(repoRoot)
	for _, name := range []string{
		".prettierrc", ".prettierrc.json", "prettier.config.js", "prettier.config.mjs",
		".eslintrc", ".eslintrc.json", ".eslintrc.js", "eslint.config.js", "eslint.config.mjs",
		"tsconfig.json", "ruff.toml", "pytest.ini", "jest.config.js", "jest.config.ts",
		"playwright.config.ts", "playwright.config.js", "vitest.config.ts", "vitest.config.js",
		"docker-compose.yml", "docker-compose.yaml", "Dockerfile",
	} {
		if fileExists(filepath.Join(repoRoot, name)) {
			sig.Configs = append(sig.Configs, name)
		}
	}
	if pyHasTool(repoRoot, "ruff") {
		sig.Configs = appendUnique(sig.Configs, "pyproject.toml[tool.ruff]")
	}
	if pyHasTool(repoRoot, "black") {
		sig.Configs = appendUnique(sig.Configs, "pyproject.toml[tool.black]")
	}
	for _, name := range []string{"src", "app", "web", "frontend", "backend", "api", "tests", "test", "__tests__", "components", "pages", "convex", "migrations"} {
		if dirExists(filepath.Join(repoRoot, name)) {
			sig.Dirs = append(sig.Dirs, name)
		}
	}
	sig.GitRemote = detectGitRemote(repoRoot)
	for _, name := range []string{".env", ".env.local", ".env.production", ".env.development", "credentials.json", "secrets.yaml"} {
		if fileExists(filepath.Join(repoRoot, name)) || fileExists(filepath.Join(repoRoot, name+".example")) {
			sig.HasEnvFiles = true
			break
		}
	}
	for _, name := range []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "Cargo.lock", "poetry.lock", "Pipfile.lock", "go.sum"} {
		if fileExists(filepath.Join(repoRoot, name)) {
			sig.HasLockfiles = true
			break
		}
	}
	return sig
}

func detectDeps(repoRoot string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(names ...string) {
		for _, n := range names {
			n = strings.TrimSpace(strings.ToLower(n))
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}

	if data, err := os.ReadFile(filepath.Join(repoRoot, "package.json")); err == nil {
		var pkg struct {
			Dependencies    map[string]any `json:"dependencies"`
			DevDependencies map[string]any `json:"devDependencies"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			for k := range pkg.Dependencies {
				add(k, depFamily(k))
			}
			for k := range pkg.DevDependencies {
				add(k, depFamily(k))
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					add(fields[1], depFamily(fields[1]))
				}
			} else if strings.Contains(line, "/") && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "module ") {
				fields := strings.Fields(line)
				if len(fields) >= 1 && strings.Contains(fields[0], ".") {
					add(fields[0], depFamily(fields[0]))
				}
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(repoRoot, "pyproject.toml")); err == nil {
		lower := strings.ToLower(string(data))
		for _, name := range []string{"django", "fastapi", "flask", "pytest", "ruff", "black", "mypy", "sqlalchemy", "alembic", "sentry-sdk", "playwright", "prisma"} {
			if strings.Contains(lower, name) {
				add(name)
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(repoRoot, "Cargo.toml")); err == nil {
		lower := strings.ToLower(string(data))
		for _, name := range []string{"tokio", "serde", "axum", "actix", "diesel"} {
			if strings.Contains(lower, name) {
				add(name)
			}
		}
	}
	return out
}

func depFamily(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "react"):
		return "react"
	case strings.Contains(lower, "next"):
		return "next"
	case strings.Contains(lower, "vue"):
		return "vue"
	case strings.Contains(lower, "angular"):
		return "angular"
	case strings.Contains(lower, "playwright"):
		return "playwright"
	case strings.Contains(lower, "prisma"):
		return "prisma"
	case strings.Contains(lower, "supabase"):
		return "supabase"
	case strings.Contains(lower, "convex"):
		return "convex"
	case strings.Contains(lower, "sentry"):
		return "sentry"
	case strings.Contains(lower, "stripe"):
		return "stripe"
	case strings.Contains(lower, "express"):
		return "express"
	case strings.Contains(lower, "fastify"):
		return "fastify"
	case strings.Contains(lower, "django"):
		return "django"
	case strings.Contains(lower, "fastapi"):
		return "fastapi"
	case strings.Contains(lower, "jwt") || strings.Contains(lower, "passport") || strings.Contains(lower, "auth0") || strings.Contains(lower, "oauth"):
		return "auth"
	default:
		return ""
	}
}

func detectGitRemote(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func pyHasTool(repoRoot, tool string) bool {
	data, err := os.ReadFile(filepath.Join(repoRoot, "pyproject.toml"))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "[tool."+tool+"]")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func appendUnique(in []string, v string) []string {
	for _, s := range in {
		if s == v {
			return in
		}
	}
	return append(in, v)
}

// HasDep reports whether any detected dependency matches name (substring ok).
func (s StackSignals) HasDep(name string) bool {
	name = strings.ToLower(name)
	for _, d := range s.Deps {
		if d == name || strings.Contains(d, name) {
			return true
		}
	}
	return false
}

// HasConfig reports whether a config file name was detected.
func (s StackSignals) HasConfig(name string) bool {
	for _, c := range s.Configs {
		if c == name || strings.Contains(c, name) {
			return true
		}
	}
	return false
}

// HasDir reports whether a top-level directory was detected.
func (s StackSignals) HasDir(name string) bool {
	for _, d := range s.Dirs {
		if d == name {
			return true
		}
	}
	return false
}

// HasManifest reports whether a manifest file was detected.
func (s StackSignals) HasManifest(name string) bool {
	for _, m := range s.Manifests {
		if m == name {
			return true
		}
	}
	return false
}

// IsGitHub reports whether the origin remote looks like GitHub.
func (s StackSignals) IsGitHub() bool {
	r := strings.ToLower(s.GitRemote)
	return strings.Contains(r, "github.com")
}

// IsFrontend reports UI-oriented stack signals.
func (s StackSignals) IsFrontend() bool {
	return s.HasDep("react") || s.HasDep("next") || s.HasDep("vue") || s.HasDep("angular") ||
		s.HasDir("components") || s.HasDir("frontend") || s.HasDir("web") || s.HasDir("pages")
}
