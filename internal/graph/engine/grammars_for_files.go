package engine

import (
	"os"
	"path/filepath"
	"sort"
)

var ambiguousGrammarExtensions = map[string][]string{
	".m":   {"matlab", "objc", "magma"},
	".cls": {"apex", "objectscript_udl"},
	".inc": {"bitbake", "objectscript_routine"},
}

func grammarsForFiles(root string, files []string, overrides map[string]string) []string {
	seen := map[string]bool{}
	for _, rel := range files {
		detection, ok := DetectLanguage(rel, nil, overrides)
		if !ok {
			ext := filepath.Ext(rel)
			if candidates := ambiguousGrammarExtensions[ext]; len(candidates) > 0 {
				body, err := readRepoFile(root, rel)
				if err != nil {
					for _, language := range candidates {
						addRepoGrammar(seen, language)
					}
					continue
				}
				detection, ok = DetectLanguage(rel, body, overrides)
			} else if ext == ".xml" {
				body, err := readRepoFile(root, rel)
				if err == nil {
					detection, ok = DetectLanguage(rel, body, overrides)
				}
			}
		}
		if !ok {
			continue
		}
		addRepoGrammar(seen, detection.Grammar)
		if detection.Language == "objectscript_export" {
			addRepoGrammar(seen, "objectscript_udl")
		}
	}
	if len(seen) == 0 {
		return nil
	}
	result := make([]string, 0, len(seen))
	for language := range seen {
		result = append(result, language)
	}
	sort.Strings(result)
	return result
}

func addRepoGrammar(seen map[string]bool, language string) {
	if language == "" || !knownLanguage(language) || seen[language] {
		return
	}
	seen[language] = true
}

func databaseExists(root string) bool {
	paths, err := CachePaths(root)
	if err != nil {
		return false
	}
	_, err = os.Stat(paths.Database)
	return err == nil
}
