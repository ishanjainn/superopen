// Package langspec holds Superopen language extraction specifications.
package langspec

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed assets/lang_specs.json
var langSpecsJSON []byte

// Spec describes Tree-sitter node kinds used for extraction for one language.
type Spec struct {
	Functions         []string `json:"Functions,omitempty"`
	Classes           []string `json:"Classes,omitempty"`
	Fields            []string `json:"Fields,omitempty"`
	Modules           []string `json:"Modules,omitempty"`
	Calls             []string `json:"Calls,omitempty"`
	Imports           []string `json:"Imports,omitempty"`
	ImportsFrom       []string `json:"ImportsFrom,omitempty"`
	Branches          []string `json:"Branches,omitempty"`
	Variables         []string `json:"Variables,omitempty"`
	Assignments       []string `json:"Assignments,omitempty"`
	Throws            []string `json:"Throws,omitempty"`
	ThrowsClause      string   `json:"ThrowsClause,omitempty"`
	Decorators        []string `json:"Decorators,omitempty"`
	EnvFunctions      []string `json:"EnvFunctions,omitempty"`
	EnvMemberPatterns []string `json:"EnvMemberPatterns,omitempty"`
}

var byLanguage map[string]Spec

func init() {
	if err := json.Unmarshal(langSpecsJSON, &byLanguage); err != nil {
		panic(fmt.Sprintf("langspec: load assets/lang_specs.json: %v", err))
	}
}

// Lookup returns a defensive copy of the extraction spec for language.
func Lookup(language string) (Spec, bool) {
	spec, ok := byLanguage[language]
	if !ok {
		return Spec{}, false
	}
	return clone(spec), true
}

// All returns a map of defensive copies for every language with a spec.
func All() map[string]Spec {
	out := make(map[string]Spec, len(byLanguage))
	for k, v := range byLanguage {
		out[k] = clone(v)
	}
	return out
}

func clone(spec Spec) Spec {
	spec.Functions = append([]string(nil), spec.Functions...)
	spec.Classes = append([]string(nil), spec.Classes...)
	spec.Fields = append([]string(nil), spec.Fields...)
	spec.Modules = append([]string(nil), spec.Modules...)
	spec.Calls = append([]string(nil), spec.Calls...)
	spec.Imports = append([]string(nil), spec.Imports...)
	spec.ImportsFrom = append([]string(nil), spec.ImportsFrom...)
	spec.Branches = append([]string(nil), spec.Branches...)
	spec.Variables = append([]string(nil), spec.Variables...)
	spec.Assignments = append([]string(nil), spec.Assignments...)
	spec.Throws = append([]string(nil), spec.Throws...)
	spec.Decorators = append([]string(nil), spec.Decorators...)
	spec.EnvFunctions = append([]string(nil), spec.EnvFunctions...)
	spec.EnvMemberPatterns = append([]string(nil), spec.EnvMemberPatterns...)
	return spec
}
