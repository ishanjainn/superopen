package engine

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ignoreRule struct {
	pattern *regexp.Regexp
	include bool
}

type graphIgnore struct{ rules []ignoreRule }

func loadGraphIgnore(root string, explicit []string) (graphIgnore, error) {
	var lines []string
	file, err := os.Open(filepath.Join(root, ".soignore"))
	if err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		closeErr := file.Close()
		if scanErr := scanner.Err(); scanErr != nil {
			return graphIgnore{}, scanErr
		}
		if closeErr != nil {
			return graphIgnore{}, closeErr
		}
	} else if !os.IsNotExist(err) {
		return graphIgnore{}, err
	}
	lines = append(lines, explicit...)
	result := graphIgnore{}
	for _, line := range lines {
		rule, ok := compileIgnoreRule(line)
		if ok {
			result.rules = append(result.rules, rule)
		}
	}
	return result, nil
}

func (g graphIgnore) Match(rel string) bool {
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	excluded := false
	for _, rule := range g.rules {
		if rule.pattern.MatchString(rel) {
			excluded = !rule.include
		}
	}
	return excluded
}

func compileIgnoreRule(line string) (ignoreRule, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ignoreRule{}, false
	}
	include := strings.HasPrefix(line, "!")
	if include {
		line = strings.TrimPrefix(line, "!")
	}
	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")
	directory := strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")
	if line == "" {
		return ignoreRule{}, false
	}
	var expression strings.Builder
	if anchored {
		expression.WriteByte('^')
	} else if !strings.Contains(line, "/") {
		expression.WriteString(`(^|.*/)`)
	} else {
		expression.WriteString(`(^|.*/)`)
	}
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case '*':
			if index+1 < len(line) && line[index+1] == '*' {
				expression.WriteString(`.*`)
				index++
			} else {
				expression.WriteString(`[^/]*`)
			}
		case '?':
			expression.WriteString(`[^/]`)
		default:
			expression.WriteString(regexp.QuoteMeta(string(line[index])))
		}
	}
	if directory {
		expression.WriteString(`(/.*)?$`)
	} else {
		expression.WriteByte('$')
	}
	return ignoreRule{pattern: regexp.MustCompile(expression.String()), include: include}, true
}
