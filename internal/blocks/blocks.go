// Package blocks keeps scripts the agent would otherwise retype. Nothing here
// writes a playbook until the user applies a proposal. Commands are never
// denied or rewritten.
package blocks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/compact"
	"github.com/ishanjainn/superopen/third_party/caveman/engine"
)

// Names are the built-in scripts, in catalog order.
var Names = []string{
	"json-peek",
	"jsonl-stats",
	"test-summary",
	"first-error",
	"wait-for",
	"grep-defs",
	"http-json",
	"replace-in-file",
}

// GrepDefsNote stays visible because this block can cost more than a plain grep.
const GrepDefsNote = "grep-defs can be larger than a plain grep"

const (
	markerBegin = "<!-- BEGIN SUPEROPEN BLOCKS -->"
	markerEnd   = "<!-- END SUPEROPEN BLOCKS -->"
)

// ProposalPath is the staged index. It is under .so/harvest, which is gitignored.
func ProposalPath(root string) string {
	return filepath.Join(root, ".so", "harvest", "blocks.proposal")
}

// IndexText is the playbook section apply writes. grep-defs stays a red row.
func IndexText() string {
	return strings.TrimSpace(`
`+markerBegin+`
Reusable scripts. Run so blocks run <name>. Do not retype these.

- json-peek <file> [path] — keys and types, not the whole document
- jsonl-stats <file> — line count and keys
- test-summary — counts and every failure
- first-error — the first error line and the two lines after it
- wait-for — poll a file or a command until it matches
- grep-defs <file> — signatures only. `+GrepDefsNote+`
- http-json <url> — fetch JSON and shrink it
- replace-in-file <file> <old> <new> — one literal replacement
`+markerEnd) + "\n"
}

// JSONPeek prints the shape of a JSON value, or of one dotted path inside it.
func JSONPeek(r io.Reader, dotted string) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r, compact.MaxInput+1))
	if err != nil {
		return "", err
	}
	if len(raw) > compact.MaxInput {
		return "", compact.ErrTooLarge
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	if dotted != "" {
		cur := v
		for _, part := range strings.Split(dotted, ".") {
			obj, ok := cur.(map[string]any)
			if !ok {
				return "", fmt.Errorf("path %q is not an object", dotted)
			}
			next, ok := obj[part]
			if !ok {
				return "", fmt.Errorf("path %q missing %q", dotted, part)
			}
			cur = next
		}
		v = cur
	}
	return summarizeJSON(v), nil
}

func summarizeJSON(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sortStrings(keys)
		var b strings.Builder
		fmt.Fprintf(&b, "object keys=%d\n", len(keys))
		for _, k := range keys {
			fmt.Fprintf(&b, "%s %s\n", k, jsonKind(t[k]))
		}
		return b.String()
	case []any:
		kind := "empty"
		if len(t) > 0 {
			kind = jsonKind(t[0])
		}
		return fmt.Sprintf("array len=%d first=%s\n", len(t), kind)
	default:
		buf, err := json.Marshal(v)
		if err != nil {
			return jsonKind(v) + "\n"
		}
		return string(buf) + "\n"
	}
}

func jsonKind(v any) string {
	switch t := v.(type) {
	case map[string]any:
		return fmt.Sprintf("object(%d)", len(t))
	case []any:
		return fmt.Sprintf("array(%d)", len(t))
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return "value"
	}
}

// JSONLStats counts lines and the keys that appear.
func JSONLStats(r io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r, compact.MaxInput+1))
	if err != nil {
		return "", err
	}
	if len(raw) > compact.MaxInput {
		return "", compact.ErrTooLarge
	}
	lines := 0
	bad := 0
	blank := 0
	keys := map[string]int{}
	sc := bytes.Split(raw, []byte("\n"))
	for _, line := range sc {
		if len(bytes.TrimSpace(line)) == 0 {
			blank++
			continue
		}
		lines++
		var obj map[string]any
		if json.Unmarshal(line, &obj) != nil {
			bad++
			continue
		}
		for k := range obj {
			keys[k]++
		}
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sortStrings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "lines=%d bad=%d blank=%d keys=%d\n", lines, bad, blank, len(names))
	for _, k := range names {
		fmt.Fprintf(&b, "%s %d\n", k, keys[k])
	}
	return b.String(), nil
}

// TestSummary keeps counts and failures from a test report.
func TestSummary(root string, input []byte) string {
	out := compact.CompressAny(root, input, engine.TypeTestReport)
	if out.PassedThrough {
		return string(input)
	}
	return "recover so retrieve " + out.Handle + "\n" + string(out.Output)
}

// FirstError returns the first error-like line and the two lines after it.
func FirstError(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		low := strings.ToLower(line)
		if strings.Contains(low, "error") || strings.Contains(low, "fail") || strings.Contains(low, "panic") {
			end := i + 3
			if end > len(lines) {
				end = len(lines)
			}
			return strings.Join(lines[i:end], "\n") + "\n"
		}
	}
	return ""
}

// GrepDefs keeps imports, signatures, and type declarations.
func GrepDefs(root string, input []byte) (string, string) {
	out := compact.CompressAny(root, input, engine.TypeCode)
	if out.PassedThrough {
		return string(input), GrepDefsNote
	}
	return "recover so retrieve " + out.Handle + "\n" + string(out.Output), GrepDefsNote
}

// ReplaceInFile replaces the first literal old with new and writes the file back.
func ReplaceInFile(path, old, next string) error {
	if old == "" {
		return fmt.Errorf("old text is empty")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Contains(body, []byte(old)) {
		return fmt.Errorf("old text not found")
	}
	updated := bytes.Replace(body, []byte(old), []byte(next), 1)
	return os.WriteFile(path, updated, 0o644)
}

// WaitFor polls until match returns true or the timeout elapses.
func WaitFor(timeout time.Duration, match func() (bool, error)) error {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		ok, err := match()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait-for timed out")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j] < s[j-1] {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}
