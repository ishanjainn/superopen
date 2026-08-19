package engine

import "testing"

func TestReadinessManifestIsUniqueAndCoversEveryGrammar(t *testing.T) {
	capabilities := Capabilities()
	seen := make(map[string]bool, len(capabilities.Gates))
	grammars := make(map[string]bool, len(Languages))
	for _, item := range capabilities.Gates {
		if item.ID == "" || item.Area == "" || item.Requirement == "" {
			t.Fatalf("incomplete readiness gate: %+v", item)
		}
		if seen[item.ID] {
			t.Fatalf("duplicate readiness gate %q", item.ID)
		}
		seen[item.ID] = true
		if item.Area == "grammar" {
			if len(item.Languages) != 1 {
				t.Fatalf("grammar gate must name one language: %+v", item)
			}
			grammars[item.Languages[0]] = true
		}
	}
	for _, language := range Languages {
		if !grammars[language] {
			t.Errorf("missing grammar gate for %s", language)
		}
	}
	if len(grammars) != len(Languages) {
		t.Fatalf("grammar gate count=%d language count=%d", len(grammars), len(Languages))
	}
	if capabilities.Readiness.Total != len(capabilities.Gates) {
		t.Fatalf("summary total=%d gates=%d", capabilities.Readiness.Total, len(capabilities.Gates))
	}
	if capabilities.Complete != (capabilities.Readiness.Verified == capabilities.Readiness.Total) {
		t.Fatal("complete flag is not derived from the readiness ledger")
	}
}

func TestPinnedSchemaInventoriesHaveReadinessGates(t *testing.T) {
	capabilities := Capabilities()
	gateIDs := map[string]bool{}
	for _, item := range capabilities.Gates {
		gateIDs[item.ID] = true
	}
	for name, values := range map[string][]string{"node": NodeLabels, "edge": EdgeTypes} {
		seen := map[string]bool{}
		for _, value := range values {
			if seen[value] {
				t.Errorf("duplicate %s type %s", name, value)
			}
			seen[value] = true
			if !gateIDs[name+"."+value] {
				t.Errorf("missing readiness gate %s.%s", name, value)
			}
		}
	}
	if capabilities.Readiness.Verified+capabilities.Readiness.InProgress+capabilities.Readiness.Pending != capabilities.Readiness.Total {
		t.Fatalf("parity summary does not sum: %+v", capabilities.Readiness)
	}
}
