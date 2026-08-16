package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	PinnedVersion = "0.9.44"
)

var supportedExtras = []string{"mcp", "neo4j", "falkordb", "pdf", "watch", "svg", "leiden", "office", "google", "postgres", "video", "kimi", "ollama", "bedrock", "anthropic", "gemini", "openai", "chinese", "sql", "pascal", "terraform", "ocaml"}

var extraImports = map[string][]string{
	"mcp": {"mcp", "starlette"}, "neo4j": {"neo4j"}, "falkordb": {"falkordb"},
	"pdf": {"pypdf", "markdownify"}, "office": {"docx", "openpyxl"}, "watch": {"watchdog"},
	"svg": {"matplotlib"}, "leiden": {"graspologic"}, "google": {"openpyxl"},
	"postgres": {"psycopg"}, "video": {"faster_whisper", "yt_dlp"},
	"kimi": {"openai", "tiktoken"}, "ollama": {"openai"}, "gemini": {"openai", "tiktoken"},
	"openai": {"openai", "tiktoken"}, "bedrock": {"boto3"}, "anthropic": {"anthropic"},
	"chinese": {"jieba"}, "sql": {"tree_sitter_sql"}, "terraform": {"tree_sitter_hcl"},
	"pascal": {"tree_sitter_pascal"}, "ocaml": {"tree_sitter_ocaml"}, "dm": {"tree_sitter_dm"},
}

func packageSpec() string {
	if runtime.GOOS == "windows" {
		return "graphifyy[all]==" + PinnedVersion
	}
	return "graphifyy[" + strings.Join(supportedExtras, ",") + "]==" + PinnedVersion
}

type ToolchainStatus struct {
	Available       bool              `json:"available"`
	Binary          string            `json:"binary,omitempty"`
	Version         string            `json:"version,omitempty"`
	Interpreter     string            `json:"interpreter,omitempty"`
	Managed         bool              `json:"managed"`
	ModuleOK        bool              `json:"module_ok"`
	ConsoleOK       bool              `json:"console_ok"`
	ValidationError string            `json:"validation_error,omitempty"`
	Extras          map[string]string `json:"extras"`
}

func cacheRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(base, "superopen", "tools", "graphify", PinnedVersion), nil
}

func managedPaths() (root, python, graphify string, err error) {
	root, err = cacheRoot()
	if err != nil {
		return "", "", "", err
	}
	binDir := filepath.Join(root, "venv", "bin")
	python = filepath.Join(binDir, "python")
	graphify = filepath.Join(binDir, "graphify")
	if runtime.GOOS == "windows" {
		binDir = filepath.Join(root, "venv", "Scripts")
		python = filepath.Join(binDir, "python.exe")
		graphify = filepath.Join(binDir, "graphify.exe")
	}
	return root, python, graphify, nil
}

func binaryVersion(bin string) (string, error) {
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s --version: %w", bin, err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	for i := len(fields) - 1; i >= 0; i-- {
		v := strings.TrimPrefix(fields[i], "v")
		if v == PinnedVersion {
			return v, nil
		}
	}
	return "", fmt.Errorf("Graphify version mismatch: need %s, got %q", PinnedVersion, strings.TrimSpace(string(out)))
}

func moduleVersion(python string) (string, error) {
	out, err := exec.Command(python, "-m", "graphify", "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s -m graphify --version: %w", python, err)
	}
	if strings.Contains(string(out), PinnedVersion) {
		return PinnedVersion, nil
	}
	return "", fmt.Errorf("Graphify version mismatch: need %s, got %q", PinnedVersion, strings.TrimSpace(string(out)))
}

func validateManagedToolchain(python, graphify string) (map[string]string, error) {
	if _, err := moduleVersion(python); err != nil {
		return nil, err
	}
	if _, err := binaryVersion(graphify); err != nil {
		return nil, fmt.Errorf("validate final Graphify console script (possible non-relocatable shebang): %w", err)
	}
	if out, err := exec.Command(python, "-c", "import graphify; import importlib.metadata as m; assert m.version('graphifyy') == '"+PinnedVersion+"'").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("validate Graphify imports: %w (%s)", err, truncateOut(out, 400))
	}
	extras := map[string]string{}
	modules := []string{}
	for name, imports := range extraImports {
		if name == "dm" && runtime.GOOS != "windows" {
			extras[name] = "not_applicable"
			continue
		}
		extras[name] = "available"
		modules = append(modules, imports...)
	}
	if out, err := exec.Command(python, "-c", "import "+strings.Join(modules, ", ")).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("validate Graphify compatible extras: %w (%s)", err, truncateOut(out, 800))
	}
	return extras, nil
}

// resolveGraphifyBin accepts only the exact pin. The explicit override is
// useful for hermetic CI, but it is held to the same version contract.
func resolveGraphifyBin() (string, error) {
	if override := strings.TrimSpace(os.Getenv("SUPEROPEN_GRAPHIFY_BIN")); override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("SUPEROPEN_GRAPHIFY_BIN must be an absolute path")
		}
		if _, err := binaryVersion(override); err != nil {
			return "", err
		}
		return override, nil
	}
	_, python, _, err := managedPaths()
	if err == nil {
		if _, statErr := os.Stat(python); statErr == nil {
			if _, versionErr := moduleVersion(python); versionErr == nil {
				return python, nil
			}
		}
	}
	// An exact PATH install is accepted for development, but never preferred
	// over the managed environment and never used without version validation.
	if found, lookErr := exec.LookPath("graphify"); lookErr == nil {
		if _, versionErr := binaryVersion(found); versionErr == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("Graphify %s is not installed; run `so install`", PinnedVersion)
}

func resolveGraphify() (string, []string, error) {
	bin, err := resolveGraphifyBin()
	if err != nil {
		return "", nil, err
	}
	_, python, _, _ := managedPaths()
	if filepath.Clean(bin) == filepath.Clean(python) {
		return bin, []string{"-m", "graphify"}, nil
	}
	return bin, nil, nil
}

// EnsureTool installs Graphify into an isolated, versioned Python 3.12 venv.
// Installation is assembled and validated in a sibling directory, then
// published with a rename so a failed upgrade cannot corrupt the prior tool.
func EnsureTool() error {
	if override := strings.TrimSpace(os.Getenv("SUPEROPEN_GRAPHIFY_BIN")); override != "" {
		if !filepath.IsAbs(override) {
			return fmt.Errorf("SUPEROPEN_GRAPHIFY_BIN must be an absolute path")
		}
		if _, err := binaryVersion(override); err != nil {
			return err
		}
		return nil
	}
	_, managedPython, managedGraphify, managedErr := managedPaths()
	if managedErr == nil {
		if _, statErr := os.Stat(managedPython); statErr == nil {
			if _, versionErr := validateManagedToolchain(managedPython, managedGraphify); versionErr == nil {
				return nil
			}
		}
	}
	uv, err := exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("Graphify %s missing: install uv, then rerun `so install`", PinnedVersion)
	}
	root, _, _, err := managedPaths()
	if err != nil {
		return err
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	venv := filepath.Join(tmp, "venv")
	if out, runErr := exec.Command(uv, "venv", "--relocatable", "--python", "3.12", venv).CombinedOutput(); runErr != nil {
		return fmt.Errorf("create Graphify Python 3.12 environment: %w (%s)", runErr, truncateOut(out, 400))
	}
	python := filepath.Join(venv, "bin", "python")
	if runtime.GOOS == "windows" {
		python = filepath.Join(venv, "Scripts", "python.exe")
	}
	spec := packageSpec()
	if out, runErr := exec.Command(uv, "pip", "install", "--python", python, spec).CombinedOutput(); runErr != nil {
		return fmt.Errorf("install %s: %w (%s)", spec, runErr, truncateOut(out, 800))
	}
	tmpGraphify := filepath.Join(venv, "bin", "graphify")
	if runtime.GOOS == "windows" {
		tmpGraphify = filepath.Join(venv, "Scripts", "graphify.exe")
	}
	extras, err := validateManagedToolchain(python, tmpGraphify)
	if err != nil {
		return err
	}
	manifest, _ := json.MarshalIndent(map[string]any{"version": PinnedVersion, "package": spec, "python": "3.12", "capabilities": CapabilityState(), "extras": extras}, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, "manifest.json"), append(manifest, '\n'), 0o644); err != nil {
		return err
	}
	old := root + ".previous"
	_ = os.RemoveAll(old)
	if _, statErr := os.Stat(root); statErr == nil {
		if err := os.Rename(root, old); err != nil {
			return fmt.Errorf("preserve previous Graphify toolchain: %w", err)
		}
	}
	if err := os.Rename(tmp, root); err != nil {
		_ = os.Rename(old, root)
		return fmt.Errorf("publish Graphify toolchain: %w", err)
	}
	_, finalPython, finalGraphify, pathErr := managedPaths()
	if pathErr != nil {
		_ = os.RemoveAll(root)
		_ = os.Rename(old, root)
		return pathErr
	}
	if _, validateErr := validateManagedToolchain(finalPython, finalGraphify); validateErr != nil {
		_ = os.RemoveAll(root)
		_ = os.Rename(old, root)
		return fmt.Errorf("published Graphify toolchain is invalid; previous runtime restored: %w", validateErr)
	}
	_ = os.RemoveAll(old)
	return nil
}

func Status() ToolchainStatus {
	st := ToolchainStatus{Extras: map[string]string{"all": "missing"}}
	bin, err := resolveGraphifyBin()
	if err != nil {
		return st
	}
	st.Available, st.Binary, st.Version, st.Extras["all"] = true, bin, PinnedVersion, "available"
	root, python, _, _ := managedPaths()
	if rel, relErr := filepath.Rel(root, bin); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		st.Managed, st.Interpreter = true, python
		_, managedPython, managedGraphify, _ := managedPaths()
		_, moduleErr := moduleVersion(managedPython)
		_, consoleErr := binaryVersion(managedGraphify)
		st.ModuleOK, st.ConsoleOK = moduleErr == nil, consoleErr == nil
		_, validateErr := validateManagedToolchain(managedPython, managedGraphify)
		if data, err := os.ReadFile(filepath.Join(root, "manifest.json")); err == nil {
			var manifest struct {
				Extras map[string]string `json:"extras"`
			}
			if json.Unmarshal(data, &manifest) == nil && len(manifest.Extras) > 0 {
				st.Extras = manifest.Extras
			}
		}
		if validateErr != nil {
			st.ValidationError = validateErr.Error()
			st.Extras["all"] = "incomplete"
		}
	}
	return st
}
