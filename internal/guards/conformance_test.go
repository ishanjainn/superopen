package guards

import (
	"path/filepath"
	"runtime"
	"testing"
)

func guardsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(thisFile)
}

func rulesDir(t *testing.T) string { return filepath.Join(guardsDir(t), "corpus") }

func specDir(t *testing.T) string { return guardsDir(t) }

// TestPackConformance is the keystone: it loads the real rule pack and, for every rule,
// validates it, enforces its maturity gate, and runs every embedded fixture against the
// reference evaluator. Adding a rule file automatically extends coverage.
func TestPackConformance(t *testing.T) {
	dir := rulesDir(t)
	rules, err := LoadDir(dir) // also asserts no duplicate ids and that every rule validates
	if err != nil {
		t.Fatalf("load rule pack: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("rule pack is empty")
	}

	for _, rule := range rules {
		rule := rule
		t.Run(rule.ID, func(t *testing.T) {
			results, err := CheckRule(rule)
			if err != nil {
				t.Fatalf("rule-level failure: %v", err)
			}
			for _, res := range results {
				res := res
				t.Run(res.Fixture, func(t *testing.T) {
					if !res.OK() {
						t.Fatalf("%s", res.String())
					}
				})
			}
		})
	}
}
