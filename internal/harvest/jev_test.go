package harvest

import "testing"

func TestPromoteGate(t *testing.T) {
	if !PromoteGate("promote", 2.7, 0.96) {
		t.Fatal("expected promote")
	}
	if PromoteGate("discard", 3, 0.99) || PromoteGate("promote", 2.4, 0.96) || PromoteGate("promote", 2.7, 0.79) {
		t.Fatal("expected discard")
	}
}

func TestParseJevAnswersScalesUnitEvidence(t *testing.T) {
	raw := []byte(`{"answers":{"decision":{"choice":"promote"},"evidence":{"score":0.675},"correction":{"noul":0.96}}}`)
	choice, evidence, corr, err := ParseJevAnswers(raw)
	if err != nil {
		t.Fatal(err)
	}
	if choice != "promote" || evidence < 2.69 || evidence > 2.71 || corr != 0.96 {
		t.Fatalf("got %s %.3f %.2f", choice, evidence, corr)
	}
}

func TestJevModeOffByDefault(t *testing.T) {
	t.Setenv("SUPEROPEN_HARVEST_JEV", "")
	if JevMode() {
		t.Fatal("unset must be off")
	}
	t.Setenv("SUPEROPEN_HARVEST_JEV", "1")
	if !JevMode() {
		t.Fatal("1 must be on")
	}
}
