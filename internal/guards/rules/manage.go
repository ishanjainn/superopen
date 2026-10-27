package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/guards"
)

// Entry is a rule as the CLI and UI show it: the rule, where it came from, and
// the YAML a person would edit.
type Entry struct {
	Rule   *guards.Rule
	Source Source
	// Path is the store file for a custom or override rule; empty for a
	// built-in that has not been changed.
	Path string
	YAML string
}

// Enabled reports whether scans run the rule.
func (e Entry) Enabled() bool { return !e.Rule.Disabled }

type baselineRule struct {
	rule *guards.Rule
	yaml string
}

// baselineRules decodes the embedded rules with their YAML, by id.
func baselineRules() (map[string]baselineRule, error) {
	entries, err := baselineFS.ReadDir("baseline")
	if err != nil {
		return nil, err
	}
	out := map[string]baselineRule{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), RuleFileSuffix) {
			continue
		}
		data, err := baselineFS.ReadFile(baselineRulePath(e.Name()))
		if err != nil {
			return nil, err
		}
		rule, err := guards.DecodeRule(data)
		if err != nil {
			return nil, fmt.Errorf("baseline %s: %w", e.Name(), err)
		}
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("baseline %s: %w", e.Name(), err)
		}
		if _, dup := out[rule.ID]; dup {
			return nil, fmt.Errorf("baseline duplicate rule id %q", rule.ID)
		}
		out[rule.ID] = baselineRule{rule: rule, yaml: string(data)}
	}
	return out, nil
}

// List returns every rule: the built-ins, then the custom rules, with a custom
// rule replacing the built-in of the same id. Sorted by id.
func List(storeDir string) ([]Entry, error) {
	base, err := baselineRules()
	if err != nil {
		return nil, err
	}
	merged := map[string]Entry{}
	for id, b := range base {
		merged[id] = Entry{Rule: b.rule, Source: SourceBaseline, YAML: b.yaml}
	}
	dirEntries, err := os.ReadDir(storeDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	seen := map[string]string{}
	for _, de := range dirEntries {
		if de.IsDir() || !isRuleFile(de.Name()) {
			continue
		}
		path := filepath.Join(storeDir, de.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rule, err := guards.DecodeRule(data)
		if err != nil {
			return nil, fmt.Errorf("%s: decode: %w", path, err)
		}
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("%s: validate: %w", path, err)
		}
		if prev, dup := seen[rule.ID]; dup {
			return nil, fmt.Errorf("rule id %q is in both %s and %s", rule.ID, prev, path)
		}
		seen[rule.ID] = path
		src := SourceStore
		if _, builtin := base[rule.ID]; builtin {
			src = SourceOverride
		}
		merged[rule.ID] = Entry{Rule: rule, Source: src, Path: path, YAML: string(data)}
	}
	out := make([]Entry, 0, len(merged))
	for _, e := range merged {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rule.ID < out[j].Rule.ID })
	return out, nil
}

// Get returns one rule by id.
func Get(storeDir, id string) (Entry, error) {
	entries, err := List(storeDir)
	if err != nil {
		return Entry{}, err
	}
	for _, e := range entries {
		if e.Rule.ID == id {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("no rule %q", id)
}

// Set validates a rule's YAML, including that every test fixture gives its
// stated verdict, and writes it to the store as <id>.rule.yaml, creating the
// rule or replacing the one with that id. A built-in id becomes an override.
func Set(storeDir string, data []byte) (Entry, error) {
	if strings.TrimSpace(string(data)) == "" {
		return Entry{}, &RuleError{Msg: "the rule is empty"}
	}
	rule, err := guards.DecodeRule(data)
	if err != nil {
		return Entry{}, decodeError(err)
	}
	results, err := guards.CheckRule(rule)
	if err != nil {
		return Entry{}, validationError(data, err)
	}
	for _, res := range results {
		if res.OK() {
			continue
		}
		detail := fmt.Sprintf("expected %s but got %s", res.Expected, res.Actual)
		if res.Err != nil {
			detail = "failed: " + res.Err.Error()
		}
		return Entry{}, fixtureError(data, res.Fixture, detail)
	}
	if !guards.ValidID(rule.ID) {
		return Entry{}, validationError(data, fmt.Errorf("id %q must match the id pattern", rule.ID))
	}
	if err := writeRuleFile(storeDir, rule.ID, data); err != nil {
		return Entry{}, err
	}
	return Get(storeDir, rule.ID)
}

func writeRuleFile(storeDir, id string, data []byte) error {
	if _, err := EnsureStore(storeDir); err != nil {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	tmp, err := os.CreateTemp(storeDir, ".set-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(storeDir, id+RuleFileSuffix))
}

var disabledLine = regexp.MustCompile(`(?m)^disabled:[ \t]*(true|false)?[ \t]*\r?\n?`)

// SetDisabled turns a rule off or on without deleting it. A built-in is turned
// off by writing an override; turning an unchanged override back on removes it,
// so the built-in applies again.
func SetDisabled(storeDir, id string, disabled bool) (Entry, error) {
	entry, err := Get(storeDir, id)
	if err != nil {
		return Entry{}, err
	}
	if entry.Rule.Disabled == disabled {
		return entry, nil
	}
	text := disabledLine.ReplaceAllString(entry.YAML, "")
	if disabled {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "disabled: true\n"
	}
	if !disabled && entry.Source == SourceOverride {
		base, err := baselineRules()
		if err != nil {
			return Entry{}, err
		}
		if b, ok := base[id]; ok && strings.TrimSpace(b.yaml) == strings.TrimSpace(text) {
			if _, err := Remove(storeDir, id); err != nil {
				return Entry{}, err
			}
			return Get(storeDir, id)
		}
	}
	rule, err := guards.DecodeRule([]byte(text))
	if err != nil {
		return Entry{}, fmt.Errorf("decode: %w", err)
	}
	if err := rule.Validate(); err != nil {
		return Entry{}, err
	}
	if err := writeRuleFile(storeDir, id, []byte(text)); err != nil {
		return Entry{}, err
	}
	return Get(storeDir, id)
}
