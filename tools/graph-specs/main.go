// Command graph-specs validates Superopen language extraction specs JSON and
// optionally regenerates a checked-in copy. Development-only; not in releases.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/langspec"
)

func main() {
	var input, check string
	flag.StringVar(&input, "in", "", "path to lang_specs.json (default: module langspec assets)")
	flag.StringVar(&check, "check", "", "if set, require this many languages (e.g. 159)")
	flag.Parse()
	if err := run(input, check); err != nil {
		fmt.Fprintln(os.Stderr, "graph-specs:", err)
		os.Exit(1)
	}
}

func run(input, check string) error {
	var raw []byte
	var err error
	if input == "" {
		// Round-trip through Lookup/All to ensure embed loads.
		all := langspec.All()
		keys := make([]string, 0, len(all))
		for k := range all {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("langspec ok: %d languages\n", len(keys))
		if check != "" {
			var want int
			if _, err := fmt.Sscanf(check, "%d", &want); err != nil {
				return err
			}
			if len(keys) != want {
				return fmt.Errorf("want %d languages, got %d", want, len(keys))
			}
		}
		return nil
	}
	raw, err = os.ReadFile(input)
	if err != nil {
		return err
	}
	var specs map[string]langspec.Spec
	if err := json.Unmarshal(raw, &specs); err != nil {
		return err
	}
	if len(specs) == 0 {
		return fmt.Errorf("%s: empty specs", input)
	}
	abs, _ := filepath.Abs(input)
	fmt.Printf("validated %s (%d languages)\n", abs, len(specs))
	return nil
}
