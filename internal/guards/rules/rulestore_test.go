package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/guards"
)

func TestBaselineValid(t *testing.T) {
	rules, err := Baseline()
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("embedded baseline is empty")
	}
	// Every embedded rule must pass full conformance (validate + maturity + fixtures).
	for _, r := range rules {
		if _, err := guards.CheckRule(r); err != nil {
			t.Errorf("baseline rule %q fails conformance: %v", r.ID, err)
		}
	}
}

func TestBaselineRulePathUsesEmbedFSSeparator(t *testing.T) {
	got := baselineRulePath("example.rule.yaml")
	if got != "baseline/example.rule.yaml" {
		t.Fatalf("embedded FS paths must use forward slashes, got %q", got)
	}
}

const aRule = `
id: %ID%
version: 1
title: T
severity: low
status: experimental
posture: detect
match: 'e.event.action == "file.read"'
emit:
  reason: ok
tests:
  - name: p
    verdict: match
    events:
      - event: { action: file.read }
`

func ruleWithID(id string) string { return strings.ReplaceAll(aRule, "%ID%", id) }

func TestLoadActiveFallsBackToBaseline(t *testing.T) {
	store := t.TempDir() // empty store -> baseline
	loaded, err := LoadActive(store, "")
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	if len(loaded) == 0 || loaded[0].Source != SourceBaseline {
		t.Fatalf("expected baseline fallback, got %d rules src=%v", len(loaded), srcOf(loaded))
	}
}

func TestLoadActiveRulesDirOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.rule.yaml"), []byte(ruleWithID("override-rule")), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadActive(t.TempDir(), dir)
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Source != SourceStore || loaded[0].Rule.ID != "override-rule" {
		t.Fatalf("expected explicit rulesDir override, got %+v", loaded)
	}
}

func TestInstallThenLoadActiveMergesWithBuiltIns(t *testing.T) {
	store := t.TempDir()

	src := filepath.Join(t.TempDir(), "my.rule.yaml")
	if err := os.WriteFile(src, []byte(ruleWithID("custom-rule")), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallFiles(store, src, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(installed) != 1 || installed[0].ID != "custom-rule" {
		t.Fatalf("unexpected install result: %+v", installed)
	}

	loaded, err := LoadActive(store, "")
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	base, err := Baseline()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(base)+1 {
		t.Fatalf("a custom rule must add to the built-ins, not replace them: %d rules, %d built-in", len(loaded), len(base))
	}
	found := false
	for _, l := range loaded {
		if l.Rule.ID == "custom-rule" {
			found = l.Source == SourceStore
		}
	}
	if !found {
		t.Fatalf("custom rule missing or mis-sourced: %v", srcOf(loaded))
	}
}

func TestInstallFilesIgnoresAppleDoubleRuleFiles(t *testing.T) {
	store := t.TempDir()
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "ok.rule.yaml"), []byte(ruleWithID("custom-rule")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "._ok.rule.yaml"), []byte("\x00\x05metadata"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallFiles(store, srcDir, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(installed) != 1 || installed[0].ID != "custom-rule" {
		t.Fatalf("unexpected install result: %+v", installed)
	}
}

func TestInstallRejectsInvalidRule(t *testing.T) {
	store := t.TempDir()
	src := filepath.Join(t.TempDir(), "bad.rule.yaml")
	// valid YAML, invalid CEL field -> CheckRule fails
	bad := strings.Replace(ruleWithID("bad-rule"), `e.event.action == "file.read"`, `e.nope.field == 1`, 1)
	if err := os.WriteFile(src, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFiles(store, src, false); err == nil {
		t.Fatal("expected install to reject an invalid rule")
	}
	// Nothing should have been written.
	if HasRuleFiles(store) {
		t.Fatal("invalid rule must not be written to the store")
	}
}

func TestInstallRejectsFailingFixture(t *testing.T) {
	store := t.TempDir()
	src := filepath.Join(t.TempDir(), "mismatch.rule.yaml")
	// Valid, compilable rule whose embedded fixture asserts the wrong verdict:
	// the match condition only fires on file.read, but the fixture feeds a
	// file.write event while expecting a match. CheckRule returns no error here,
	// only a failing FixtureResult.
	bad := strings.Replace(ruleWithID("mismatch-rule"), "action: file.read }", "action: file.write }", 1)
	if err := os.WriteFile(src, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFiles(store, src, false); err == nil {
		t.Fatal("expected install to reject a rule with a failing fixture")
	}
	if HasRuleFiles(store) {
		t.Fatal("rule with a failing fixture must not be written to the store")
	}
}

func TestInstallRejectsDuplicateWithoutForce(t *testing.T) {
	store := t.TempDir()
	src := filepath.Join(t.TempDir(), "dup.rule.yaml")
	if err := os.WriteFile(src, []byte(ruleWithID("dup-rule")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFiles(store, src, false); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if _, err := InstallFiles(store, src, false); err == nil {
		t.Fatal("expected duplicate-id rejection without --force")
	}
	if _, err := InstallFiles(store, src, true); err != nil {
		t.Fatalf("force re-install should succeed: %v", err)
	}
}

func TestInstallRollsBackWhenLaterWriteFails(t *testing.T) {
	store := t.TempDir()
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.rule.yaml"), []byte(ruleWithID("a-rule")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.rule.yaml"), []byte(ruleWithID("b-rule")), 0o644); err != nil {
		t.Fatal(err)
	}

	blockedDest := filepath.Join(store, "b-rule"+RuleFileSuffix)
	if err := os.Mkdir(blockedDest, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := InstallFiles(store, srcDir, false); err == nil {
		t.Fatal("expected install to fail on blocked destination")
	}
	if _, err := os.Stat(filepath.Join(store, "a-rule"+RuleFileSuffix)); !os.IsNotExist(err) {
		t.Fatalf("first rule should have been rolled back, stat err=%v", err)
	}
	if info, err := os.Stat(blockedDest); err != nil || !info.IsDir() {
		t.Fatalf("blocked destination directory should remain, info=%v err=%v", info, err)
	}
}

func TestForceInstallRestoresOverwrittenRuleWhenLaterWriteFails(t *testing.T) {
	store := t.TempDir()
	initial := strings.Replace(ruleWithID("a-rule"), "title: T", "title: Old", 1)
	initialSrc := filepath.Join(t.TempDir(), "a.rule.yaml")
	if err := os.WriteFile(initialSrc, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFiles(store, initialSrc, false); err != nil {
		t.Fatalf("initial install: %v", err)
	}

	srcDir := t.TempDir()
	updated := strings.Replace(ruleWithID("a-rule"), "title: T", "title: New", 1)
	if err := os.WriteFile(filepath.Join(srcDir, "a.rule.yaml"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.rule.yaml"), []byte(ruleWithID("b-rule")), 0o644); err != nil {
		t.Fatal(err)
	}

	blockedDest := filepath.Join(store, "b-rule"+RuleFileSuffix)
	if err := os.Mkdir(blockedDest, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := InstallFiles(store, srcDir, true); err == nil {
		t.Fatal("expected force install to fail on blocked destination")
	}
	got, err := os.ReadFile(filepath.Join(store, "a-rule"+RuleFileSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != initial {
		t.Fatalf("overwritten rule was not restored:\n%s", got)
	}
}

func TestRemove(t *testing.T) {
	store := t.TempDir()
	src := filepath.Join(t.TempDir(), "r.rule.yaml")
	if err := os.WriteFile(src, []byte(ruleWithID("removable")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFiles(store, src, false); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := Remove(store, "removable"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := Remove(store, "removable"); err == nil {
		t.Fatal("removing a missing rule should error")
	}
}

func TestRemoveRejectsPathTraversal(t *testing.T) {
	store := t.TempDir()

	// A rule file outside the store that a traversal id could resolve to.
	victimDir := t.TempDir()
	victim := filepath.Join(victimDir, "victim.rule.yaml")
	if err := os.WriteFile(victim, []byte("id: victim\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// id crafted so id+suffix == "<victimDir>/victim.rule.yaml" after Join+Clean.
	rel, err := filepath.Rel(store, filepath.Join(victimDir, "victim"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{rel, "../../etc/passwd", "a/b", "."} {
		if _, err := Remove(store, id); err == nil {
			t.Fatalf("expected Remove to reject unsafe id %q", id)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("victim file must not be deleted: %v", err)
	}
}

func srcOf(l []LoadedRule) []Source {
	s := make([]Source, len(l))
	for i := range l {
		s[i] = l[i].Source
	}
	return s
}

func TestSetOverridesABuiltInAndRemoveRestoresIt(t *testing.T) {
	store := t.TempDir()
	base, err := Baseline()
	if err != nil {
		t.Fatal(err)
	}
	id := base[0].ID
	builtin, err := Get(store, id)
	if err != nil || builtin.Source != SourceBaseline {
		t.Fatalf("a fresh store shows the built-in: %+v %v", builtin, err)
	}
	edited := strings.Replace(builtin.YAML, "title:", "title: EDITED ", 1)
	got, err := Set(store, []byte(edited))
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if got.Source != SourceOverride || !strings.Contains(got.Rule.Title, "EDITED") {
		t.Fatalf("an edited built-in becomes an override: %+v", got)
	}
	if _, err := Remove(store, id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	back, _ := Get(store, id)
	if back.Source != SourceBaseline || strings.Contains(back.Rule.Title, "EDITED") {
		t.Fatalf("removing the override restores the built-in: %+v", back)
	}
	if _, err := Remove(store, id); err == nil || !strings.Contains(err.Error(), "built in") {
		t.Fatalf("a built-in cannot be removed, only disabled: %v", err)
	}
}

func TestSetRejectsBadRulesWithoutWriting(t *testing.T) {
	store := t.TempDir()
	if _, err := Set(store, []byte("id: Bad ID\n")); err == nil {
		t.Fatal("an invalid rule must be rejected")
	}
	failing := strings.Replace(ruleWithID("fixture-lies"), "verdict: match", "verdict: no_match", 1)
	if _, err := Set(store, []byte(failing)); err == nil {
		t.Fatal("a fixture that disagrees with its verdict must be rejected")
	}
	if entries, _ := os.ReadDir(store); len(entries) != 0 {
		t.Fatalf("a rejected rule must leave no file behind: %v", entries)
	}
}

func TestSetDisabledDisablesBuiltInAndCustomRules(t *testing.T) {
	store := t.TempDir()
	base, err := Baseline()
	if err != nil {
		t.Fatal(err)
	}
	id := base[0].ID

	off, err := SetDisabled(store, id, true)
	if err != nil || off.Enabled() || off.Source != SourceOverride {
		t.Fatalf("disabling a built-in writes an override: %+v %v", off, err)
	}
	active, _ := LoadActive(store, "")
	if len(active) != len(base)-1 {
		t.Fatalf("a disabled rule must not run: %d active of %d", len(active), len(base))
	}
	on, err := SetDisabled(store, id, false)
	if err != nil || !on.Enabled() || on.Source != SourceBaseline {
		t.Fatalf("enabling an unchanged override restores the built-in: %+v %v", on, err)
	}
	if entries, _ := os.ReadDir(store); len(entries) != 0 {
		t.Fatalf("no override file should remain: %v", entries)
	}

	if _, err := Set(store, []byte(ruleWithID("mine"))); err != nil {
		t.Fatal(err)
	}
	custom, err := SetDisabled(store, "mine", true)
	if err != nil || custom.Enabled() || custom.Source != SourceStore {
		t.Fatalf("disabling a custom rule: %+v %v", custom, err)
	}
	custom, err = SetDisabled(store, "mine", false)
	if err != nil || !custom.Enabled() || strings.Contains(custom.YAML, "disabled") {
		t.Fatalf("enabling a custom rule drops the flag: %+v %v", custom, err)
	}
}
