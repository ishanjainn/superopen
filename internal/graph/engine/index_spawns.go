package engine

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Python process APIs use the same qualified spellings and positional program
// argument as the source-language spawn table. Bare names require an import
// from one of these modules.
var pythonSpawnAPIs = map[string]int{
	"subprocess.run": 0, "subprocess.call": 0, "subprocess.check_call": 0,
	"subprocess.check_output": 0, "subprocess.Popen": 0,
	"subprocess.getoutput": 0, "subprocess.getstatusoutput": 0,
	"os.system": 0, "os.popen": 0, "os.startfile": 0,
	"os.execl": 0, "os.execle": 0, "os.execlp": 0, "os.execlpe": 0,
	"os.execv": 0, "os.execve": 0, "os.execvp": 0, "os.execvpe": 0,
	"os.posix_spawn": 0, "os.posix_spawnp": 0,
	"os.spawnl": 1, "os.spawnle": 1, "os.spawnlp": 1, "os.spawnlpe": 1,
	"os.spawnv": 1, "os.spawnve": 1, "os.spawnvp": 1, "os.spawnvpe": 1,
	"asyncio.create_subprocess_exec": 0, "asyncio.create_subprocess_shell": 0,
	"pexpect.spawn": 0, "pexpect.run": 0,
}

// Process APIs follow qualified source-language spellings. A proven
// local implementation still wins over an apparent process API.
func syntaxSpawnEdge(parsed ParsedSyntaxFile, call SyntaxFact, source string, resolution symbolResolution, registry symbolRegistry) (pendingEdge, bool) {
	apiName := call.Name
	imports := make(map[string]string, len(parsed.Extraction.Imports))
	for _, fact := range parsed.Extraction.Imports {
		if fact.LocalName != "" {
			imports[fact.LocalName] = fact.Name
		}
	}
	separator := "."
	if parsed.File.Language == "rust" {
		separator = "::"
	}
	if cut := strings.Index(apiName, separator); cut >= 0 {
		if module := imports[apiName[:cut]]; module != "" {
			apiName = module + apiName[cut:]
		}
	} else if imported := imports[apiName]; imported != "" {
		apiName = imported
	}
	argIndex, ok := spawnAPIArgument(parsed.File.Language, apiName)
	if !ok {
		return pendingEdge{}, false
	}
	if target, ok := registry.exact[resolution.qn]; ok && target.Location.File != "" && !strings.HasPrefix(target.Location.File, "<") &&
		!guessStrategy(resolution.strategy) && (target.Label == "Function" || target.Label == "Method" || target.Label == "Class") &&
		!(strings.Contains(call.Name, ".") && resolution.strategy == "same_module") {
		return pendingEdge{}, false
	}
	program := spawnProgram(call, argIndex)
	return pendingEdge{source: source, target: "__process__" + program, kind: "SPAWNS",
		properties: api.Properties{"api": apiName, "program": program, "line": call.StartLine, "confidence": 1.0},
		evidence:   syntaxEvidence(parsed.File.Path, call, "process_spawn", 1)}, true
}

func spawnAPIArgument(language, apiName string) (int, bool) {
	switch language {
	case "python":
		arg, ok := pythonSpawnAPIs[apiName]
		return arg, ok
	case "go":
		switch apiName {
		case "os/exec.Command", "golang.org/x/sys/execabs.Command", "os.StartProcess", "syscall.Exec", "syscall.ForkExec", "syscall.StartProcess":
			return 0, true
		case "os/exec.CommandContext", "golang.org/x/sys/execabs.CommandContext":
			return 1, true
		}
	case "rust":
		switch apiName {
		case "std::process::Command::new", "tokio::process::Command::new", "async_process::Command::new", "async_std::process::Command::new", "smol::process::Command::new", "duct::cmd":
			return 0, true
		}
	}
	return 0, false
}

func spawnProgram(call SyntaxFact, index int) string {
	literal := ""
	for _, arg := range call.Arguments {
		if arg.Index != index || arg.Keyword != "" {
			continue
		}
		literal = firstSpawnLiteral(arg.Expr)
		break
	}
	if literal == "" && index == 0 && len(call.Arguments) == 0 {
		literal = call.FirstStringArg
	}
	if literal == "" {
		return "<dynamic>"
	}
	words := strings.Fields(literal)
	for len(words) > 1 && (strings.Contains(words[0], "=") || filepath.Base(words[0]) == "env") {
		words = words[1:]
	}
	if len(words) == 0 {
		return "<dynamic>"
	}
	program := filepath.Base(strings.ReplaceAll(words[0], `\`, "/"))
	if program == "" || len(program) >= 64 || strings.ContainsAny(program, "$%{}`()<>|;&*?[]\"'") {
		return "<dynamic>"
	}
	return program
}

func firstSpawnLiteral(expr string) string {
	for depth := 0; depth < 3; depth++ {
		expr = strings.TrimSpace(expr)
		for prefix := 0; prefix < 2 && len(expr) > 0 && strings.ContainsRune("rRbBuUfFL@$", rune(expr[0])); prefix++ {
			expr = expr[1:]
		}
		if len(expr) == 0 {
			return ""
		}
		if quote := expr[0]; quote == '\'' || quote == '"' || quote == '`' {
			for i := 1; i < len(expr); i++ {
				if expr[i] == '\\' && i+1 < len(expr) {
					i++
					continue
				}
				if expr[i] == quote {
					return expr[1:i]
				}
			}
			return ""
		}
		if expr[0] != '[' && expr[0] != '(' && expr[0] != '{' {
			return ""
		}
		first := strings.TrimSpace(expr[1:])
		end := len(first)
		level := 0
		for i := 0; i < len(first); i++ {
			if first[i] == '\'' || first[i] == '"' {
				quote := first[i]
				for i++; i < len(first); i++ {
					if first[i] == '\\' && i+1 < len(first) {
						i++
					} else if first[i] == quote {
						break
					}
				}
			} else if strings.ContainsRune("([{", rune(first[i])) {
				level++
			} else if strings.ContainsRune(")]}", rune(first[i])) {
				if level == 0 {
					end = i
					break
				}
				level--
			} else if first[i] == ',' && level == 0 {
				end = i
				break
			}
		}
		expr = first[:end]
	}
	return ""
}

func indexSpawnProcessNodes(project string, graph *goGraph) {
	seen := map[string]bool{}
	for _, node := range graph.nodes {
		seen[node.QualifiedName] = true
	}
	for _, edge := range graph.edges {
		if edge.kind != "SPAWNS" || seen[edge.target] {
			continue
		}
		program := strings.TrimPrefix(edge.target, "__process__")
		graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Process", Name: program,
			QualifiedName: edge.target, Properties: api.Properties{"program": program}})
		seen[edge.target] = true
	}
}
