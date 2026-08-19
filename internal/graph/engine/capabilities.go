package engine

import "github.com/ishanjainn/superopen/internal/graph/api"

// Languages is the Superopen asset grammar inventory. The pinned commit
// contains 159 grammar translation units; this corrects the earlier planning
// estimate of 158 and prevents an off-by-one readiness claim.
var Languages = []string{
	"ada", "agda", "apex", "assembly", "astro", "awk", "bash", "beancount",
	"bibtex", "bicep", "bitbake", "blade", "c", "c_sharp", "cairo", "capnp",
	"cfml", "cfscript", "clojure", "cmake", "cobol", "commonlisp", "cpp", "crystal",
	"css", "csv", "cuda", "d", "dart", "devicetree", "diff", "dockerfile",
	"dotenv", "elisp", "elixir", "elm", "erlang", "fennel", "fish", "form",
	"fortran", "fsharp", "func", "gdscript", "gitattributes", "gitignore", "gleam", "glsl",
	"gn", "go", "gomod", "gotemplate", "graphql", "groovy", "hare", "haskell",
	"hcl", "hlsl", "html", "hyprlang", "ini", "ispc", "janet", "java",
	"javascript", "jinja2", "jsdoc", "json", "json5", "jsonnet", "julia", "just",
	"kconfig", "kdl", "kotlin", "lean", "linkerscript", "liquid", "llvm", "lua",
	"luau", "magma", "makefile", "markdown", "matlab", "mermaid", "meson", "mojo",
	"move", "nasm", "nickel", "nix", "objc", "objectscript_routine", "objectscript_udl", "ocaml",
	"odin", "pascal", "perl", "php", "pine", "pkl", "po", "pony",
	"powershell", "prisma", "properties", "protobuf", "puppet", "purescript", "python", "qml",
	"r", "racket", "regex", "requirements", "rescript", "ron", "rst", "ruby",
	"rust", "scala", "scheme", "scss", "slang", "smali", "smithy", "solidity",
	"soql", "sosl", "sql", "squirrel", "sshconfig", "starlark", "svelte", "sway",
	"swift", "systemverilog", "tablegen", "tcl", "teal", "templ", "thrift", "tlaplus",
	"toml", "tsx", "typescript", "typst", "verilog", "vhdl", "vim", "vue",
	"wgsl", "wit", "wolfram", "xml", "yaml", "zig", "zsh",
}

func Capabilities() api.CapabilitySet {
	gates := readinessGates()
	summary := readinessSummary(gates)
	return api.CapabilitySet{
		AssetRevision: AssetRevision,
		Complete:       summary.Verified == summary.Total,
		Languages:      append([]string(nil), Languages...),
		Readiness:         summary,
		Gates:          gates,
		Capabilities: []api.Capability{
			{Name: "provider_neutral_protocol", State: "in_progress", Notes: "schema and Cypher dispatch implemented; full operation differential pending"},
			{Name: "sqlite_store", State: "in_progress", Notes: "storage snapshot differential now includes properties, locations, vectors, coverage, and file metadata"},
			{Name: "fts5_bm25", State: "in_progress", Notes: "pinned ranking differential pending"},
			{Name: "atomic_publication", State: "in_progress", Notes: "Unix validated; Windows concurrent-reader acceptance remains"},
			{Name: "unified_file_result_pipeline", State: "in_progress", Notes: "all languages, including Go, use the Tree-sitter FileResult boundary; family resolver readiness pending", Languages: append([]string(nil), Languages...)},
			{Name: "grammar_runtime", State: "in_progress", Notes: "159/159 ABI2 WASM modules embedded and bundle-verified; golden extraction differentials pending", Languages: append([]string(nil), Languages...)},
			{Name: "semantic_resolvers", State: "pending"},
			{Name: "semantic_vectors", State: "in_progress", Notes: "pinned vectors and SIMILAR_TO publication implemented; SEMANTICALLY_RELATED differential pending"},
			{Name: "graph_analysis", State: "pending"},
			{Name: "cypher", State: "pending"},
			{Name: "incremental_indexing", State: "pending"},
			{Name: "cross_repository", State: "pending"},
			{Name: "visualization_asset", State: "pending"},
		},
	}
}
