package harnessvalid

import "testing"

func TestValidatePreferences(t *testing.T) {
	if err := ValidatePreferences("# Preferences\n\nHow agents work.\n\n- a\n"); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePreferences("# Wrong\n"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeProjects(t *testing.T) {
	got := NormalizeProjects("# Projects\n\n## Current focus\n\n- ship X\n")
	if err := ValidateProjects(got); err != nil {
		t.Fatal(err, got)
	}
	if !contains(got, "## Notes") {
		t.Fatal("missing Notes", got)
	}
}

func TestAppendToProjectsSection(t *testing.T) {
	md := NormalizeProjects("")
	md = AppendToProjectsSection(md, "Notes", "learned foo")
	if err := ValidateProjects(md); err != nil {
		t.Fatal(err)
	}
	if !contains(md, "learned foo") {
		t.Fatal(md)
	}
}

func TestValidateGuardrailsBody(t *testing.T) {
	_, err := ValidateGuardrailsBody("# harvest note\n# only comments\n")
	if err == nil {
		t.Fatal("expected comment-only reject")
	}
	_, err = ValidateGuardrailsBody("approval: auto\ndenied_commands:\n  - rm -rf /\n")
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplyable(t *testing.T) {
	if err := Applyable("docs", "x.md", "", []string{"score"}); err == nil {
		t.Fatal("empty body docs")
	}
	if err := Applyable("skill", "x.md", "# hi\n", []string{"score"}); err != nil {
		t.Fatal(err)
	}
}

func TestTier(t *testing.T) {
	if Tier("guardrail") != "policy" {
		t.Fatal(Tier("guardrail"))
	}
	if Tier("skill") != "soft" {
		t.Fatal(Tier("skill"))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
