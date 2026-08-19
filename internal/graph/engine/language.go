package engine

import (
	"path/filepath"
	"strings"
)

// DetectedLanguage separates the semantic language from the Tree-sitter
// grammar used to parse it. A few Superopen languages are flavors of a shared
// grammar (for example Kustomize uses YAML), while ObjectScript export XML is
// a transform-only container and therefore has no direct grammar.
type DetectedLanguage struct {
	Language string
	Grammar  string
	Flavor   string
}

var extensionLanguages = map[string]string{
	".bash": "bash", ".sh": "bash", ".c": "c",
	".cc": "cpp", ".ccm": "cpp", ".cpp": "cpp", ".cppm": "cpp", ".cxx": "cpp",
	".h": "cpp", ".hh": "cpp", ".hpp": "cpp", ".hxx": "cpp", ".ixx": "cpp",
	".cs": "c_sharp", ".clj": "clojure", ".cljc": "clojure", ".cljs": "clojure",
	".cmake": "cmake", ".cbl": "cobol", ".cob": "cobol",
	".cl": "commonlisp", ".lisp": "commonlisp", ".lsp": "commonlisp",
	".css": "css", ".cu": "cuda", ".cuh": "cuda", ".dart": "dart",
	".dockerfile": "dockerfile", ".ex": "elixir", ".exs": "elixir", ".env": "dotenv",
	".elm": "elm", ".el": "elisp", ".erl": "erlang",
	".fs": "fsharp", ".fsi": "fsharp", ".fsx": "fsharp", ".frm": "form", ".prc": "form",
	".f03": "fortran", ".f08": "fortran", ".f90": "fortran", ".f95": "fortran",
	".frag": "glsl", ".glsl": "glsl", ".vert": "glsl", ".go": "go",
	".gql": "graphql", ".graphql": "graphql", ".gradle": "groovy", ".groovy": "groovy",
	".hs": "haskell", ".hcl": "hcl", ".tf": "hcl", ".htm": "html", ".html": "html",
	".cfg": "ini", ".conf": "ini", ".ini": "ini", ".java": "java",
	".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	".json": "json", ".jl": "julia", ".kt": "kotlin", ".kts": "kotlin", ".lean": "lean",
	".lua": "lua", ".mag": "magma", ".magma": "magma", ".mk": "makefile",
	".md": "markdown", ".mdx": "markdown", ".m": "matlab", ".matlab": "matlab", ".mlx": "matlab",
	".meson": "meson", ".mojo": "mojo", ".nix": "nix", ".ml": "ocaml", ".mli": "ocaml",
	".pl": "perl", ".pm": "perl", ".php": "php", ".proto": "protobuf", ".py": "python",
	".R": "r", ".r": "r", ".gemspec": "ruby", ".rake": "ruby", ".rb": "ruby",
	".rs": "rust", ".sc": "scala", ".scala": "scala", ".scss": "scss", ".sql": "sql",
	".svelte": "svelte", ".swift": "swift", ".sv": "verilog", ".v": "verilog",
	".toml": "toml", ".tsx": "tsx", ".ts": "typescript", ".mts": "typescript", ".cts": "typescript",
	".vim": "vim", ".vimrc": "vim", ".justfile": "just", ".just": "just",
	".inc": "bitbake", ".mac": "objectscript_routine", ".int": "objectscript_routine", ".rtn": "objectscript_routine",
	".vue": "vue", ".wl": "wolfram", ".wls": "wolfram",
	".xml": "xml", ".xsd": "xml", ".xsl": "xml", ".svg": "xml",
	".yaml": "yaml", ".yml": "yaml", ".adb": "ada", ".ads": "ada", ".agda": "agda",
	".astro": "astro", ".awk": "awk", ".bb": "bitbake", ".bbappend": "bitbake", ".bbclass": "bitbake",
	".beancount": "beancount", ".bib": "bibtex", ".bicep": "bicep", ".bzl": "starlark",
	".cairo": "cairo", ".capnp": "capnp", ".cls": "apex", ".cr": "crystal", ".csv": "csv",
	".d": "d", ".diff": "diff", ".dpr": "pascal", ".dts": "devicetree", ".dtsi": "devicetree",
	".fc": "func", ".fish": "fish", ".fnl": "fennel", ".fx": "hlsl", ".gd": "gdscript",
	".gleam": "gleam", ".gn": "gn", ".gni": "gn", ".gotmpl": "gotemplate", ".tpl": "gotemplate",
	".ha": "hare", ".hl": "hyprlang", ".hlsl": "hlsl", ".hlsli": "hlsl", ".ispc": "ispc",
	".j2": "jinja2", ".janet": "janet", ".jinja": "jinja2", ".jinja2": "jinja2",
	".json5": "json5", ".jsonnet": "jsonnet", ".kdl": "kdl", ".ld": "linkerscript", ".lds": "linkerscript",
	".libsonnet": "jsonnet", ".liquid": "liquid", ".ll": "llvm", ".lpr": "pascal", ".luau": "luau",
	".qml": "qml", ".cfc": "cfscript", ".cfm": "cfml", ".mermaid": "mermaid", ".mmd": "mermaid",
	".move": "move", ".nasm": "nasm", ".ncl": "nickel", ".nut": "squirrel", ".odin": "odin",
	".overlay": "devicetree", ".pas": "pascal", ".patch": "diff", ".pine": "pine", ".pkl": "pkl",
	".po": "po", ".pony": "pony", ".pot": "po", ".pp": "puppet", ".prisma": "prisma",
	".properties": "properties", ".ps1": "powershell", ".psd1": "powershell", ".psm1": "powershell",
	".purs": "purescript", ".res": "rescript", ".resi": "rescript", ".re": "regex", ".rkt": "racket",
	".ron": "ron", ".rst": "rst", ".s": "assembly", ".S": "assembly", ".scm": "scheme",
	".slang": "slang", ".smali": "smali", ".smithy": "smithy", ".sol": "solidity",
	".soql": "soql", ".sosl": "sosl", ".ss": "scheme", ".star": "starlark", ".sw": "sway",
	".tcl": "tcl", ".td": "tablegen", ".templ": "templ", ".thrift": "thrift", ".tl": "teal",
	".tla": "tlaplus", ".tmpl": "gotemplate", ".trigger": "apex", ".typ": "typst",
	".vhd": "vhdl", ".vhdl": "vhdl", ".wgsl": "wgsl", ".wit": "wit", ".zsh": "zsh", ".zig": "zig",
}

var specialFilenames = map[string]string{
	"CMakeLists.txt": "cmake", "Dockerfile": "dockerfile", "GNUmakefile": "makefile",
	"Makefile": "makefile", "makefile": "makefile", "meson.build": "meson",
	"meson.options": "meson", "meson_options.txt": "meson", "kustomization.yaml": "kustomize",
	"kustomization.yml": "kustomize", ".vimrc": "vim", ".zshrc": "zsh", ".zshenv": "zsh",
	".zprofile": "zsh", "justfile": "just", "Justfile": "just", ".justfile": "just",
	"hyprland.conf": "hyprlang", "ssh_config": "sshconfig", "sshd_config": "sshconfig",
	".ssh/config": "sshconfig", "BUILD": "starlark", "BUILD.bazel": "starlark",
	"WORKSPACE": "starlark", "WORKSPACE.bazel": "starlark", "requirements.txt": "requirements",
	"requirements-dev.txt": "requirements", "requirements-test.txt": "requirements", "Kconfig": "kconfig",
	"go.mod": "gomod", ".env": "dotenv", ".env.local": "dotenv", ".gitattributes": "gitattributes",
}

var ignoredJSONFilenames = map[string]bool{
	"package.json": true, "package-lock.json": true, "tsconfig.json": true, "jsconfig.json": true,
	"composer.json": true, "composer.lock": true, "yarn.lock": true, "openapi.json": true,
	"swagger.json": true, "jest.config.json": true, ".eslintrc.json": true, ".prettierrc.json": true,
	".babelrc.json": true, "tslint.json": true, "angular.json": true, "firebase.json": true,
	"renovate.json": true, "lerna.json": true, "turbo.json": true, ".stylelintrc.json": true,
	"pnpm-lock.json": true, "deno.json": true, "biome.json": true, "devcontainer.json": true,
	".devcontainer.json": true, "launch.json": true, "settings.json": true, "extensions.json": true,
	"tasks.json": true,
}

// DetectLanguage mirrors the Superopen asset filename and content routing.
// Overrides use exact extensions (including the leading dot), as Superopen's
// extra_extensions configuration does.
func DetectLanguage(name string, content []byte, overrides map[string]string) (DetectedLanguage, bool) {
	clean := filepath.ToSlash(filepath.Clean(name))
	base := filepath.Base(clean)
	language, ok := specialFilenames[clean]
	if !ok {
		language, ok = specialFilenames[base]
	}
	if !ok && strings.HasSuffix(clean, "/.ssh/config") {
		language, ok = "sshconfig", true
	}
	if !ok && strings.HasPrefix(base, ".env.") {
		language, ok = "dotenv", true
	}
	if !ok {
		lastDot := strings.LastIndexByte(base, '.')
		if lastDot < 0 {
			return DetectedLanguage{}, false
		}
		for dot := strings.IndexByte(base, '.'); dot >= 0 && dot < lastDot; {
			compound := base[dot:]
			if compound == ".blade.php" {
				language, ok = "blade", true
				break
			}
			if override, exists := overrides[compound]; exists && configuredLanguageValid(override) {
				language, ok = override, true
				break
			}
			next := strings.IndexByte(base[dot+1:], '.')
			if next < 0 {
				break
			}
			dot += next + 1
		}
		if !ok {
			ext := base[lastDot:]
			if override, exists := overrides[ext]; exists && configuredLanguageValid(override) {
				language, ok = override, true
			} else {
				language, ok = extensionLanguages[ext]
			}
		}
	}
	if !ok || language == "" {
		return DetectedLanguage{}, false
	}
	// These files are consumed by Superopen's package/configuration passes but
	// deliberately excluded from its normal JSON syntax-file index.
	if language == "json" && ignoredJSONFilenames[base] {
		return DetectedLanguage{}, false
	}

	prefix := content
	if len(prefix) > 4096 {
		prefix = prefix[:4096]
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".m":
		language = disambiguateM(string(prefix))
	case ".cls":
		language = disambiguateCLS(string(prefix))
	case ".inc":
		language = disambiguateINC(string(prefix))
	}
	if language == "xml" {
		xmlPrefix := content
		if len(xmlPrefix) > 255 {
			xmlPrefix = xmlPrefix[:255]
		}
		if strings.Contains(string(xmlPrefix), "<Export generator=") {
			return DetectedLanguage{Language: "objectscript_export", Flavor: "objectscript_export"}, true
		}
	}
	if language == "kustomize" {
		return DetectedLanguage{Language: language, Grammar: "yaml", Flavor: language}, true
	}
	// Superopen discovery keeps apiVersion YAML as YAML. Kubernetes Resource
	// nodes are emitted later by pass_k8s after engine helper(), so
	// premature k8s reclassification here would drop YAML Variable extraction.
	return DetectedLanguage{Language: language, Grammar: language}, true
}

func configuredLanguageValid(language string) bool {
	if language == "kustomize" || language == "k8s" || language == "objectscript_export" {
		return true
	}
	_, ok := GrammarExport(language)
	return ok
}

func disambiguateM(content string) string {
	for _, marker := range []string{"@interface", "@implementation", "@protocol", "@property", "#import", "@selector", "@encode", "@synthesize", "@dynamic"} {
		if strings.Contains(content, marker) {
			return "objc"
		}
	}
	for _, marker := range []string{"end function;", "end procedure;", "end intrinsic;", "end if;", "end for;", "end while;"} {
		if strings.Contains(content, marker) {
			return "magma"
		}
	}
	for _, marker := range []string{"intrinsic ", "procedure "} {
		for rest := content; ; {
			index := strings.Index(rest, marker)
			if index < 0 {
				break
			}
			rest = rest[index+len(marker):]
			end := 0
			for end < len(rest) && ((rest[end] >= 'A' && rest[end] <= 'Z') || (rest[end] >= 'a' && rest[end] <= 'z')) {
				end++
			}
			if end < len(rest) && rest[end] == '(' {
				return "magma"
			}
			if len(rest) == 0 {
				break
			}
		}
	}
	return "matlab"
}

func disambiguateCLS(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "Class ") && len(line) > len("Class ") {
			first := line[len("Class ")]
			if first >= 'A' && first <= 'Z' {
				return "objectscript_udl"
			}
		}
	}
	return "apex"
}

func disambiguateINC(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "ROUTINE ") && len(line) > len("ROUTINE ") {
			first := line[len("ROUTINE ")]
			if first >= 'A' && first <= 'Z' {
				return "objectscript_routine"
			}
		}
		if strings.HasPrefix(line, "#define") || strings.HasPrefix(line, "#def1arg") || strings.HasPrefix(line, "#;") {
			return "objectscript_routine"
		}
	}
	return "bitbake"
}
