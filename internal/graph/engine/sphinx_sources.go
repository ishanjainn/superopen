package engine

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var sphinxQuotedSuffix = regexp.MustCompile(`['"](\.[A-Za-z0-9_-]+)['"]`)
var sphinxRSTValue = regexp.MustCompile(`^\s*:\s*['"]restructuredtext['"]`)

func specialDocumentExtension(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".dot", ".gv", ".puml", ".plantuml", ".iuml", ".wsd", ".pdf":
		return true
	}
	return false
}

func detectRepositoryLanguage(root, rel string, body []byte, overrides map[string]string) (DetectedLanguage, bool) {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".dot", ".gv", ".puml", ".plantuml", ".iuml", ".wsd":
		return DetectedLanguage{Language: "diagram"}, true
	case ".pdf":
		if bytes.Contains(body[:min(len(body), 1024)], []byte("%PDF-")) {
			return DetectedLanguage{Language: "pdf"}, true
		}
	}
	if detection, ok := DetectLanguage(rel, body, overrides); ok {
		return detection, true
	}
	if sphinxSourceSuffix(root, rel) {
		return DetectedLanguage{Language: "rst", Grammar: "rst"}, true
	}
	return DetectedLanguage{}, false
}

// A nested conf.py replaces its parent's source suffixes. Config is read as
// text; indexing never executes it.
func sphinxSourceSuffix(root, rel string) bool {
	ext := filepath.Ext(rel)
	if ext == "" {
		return false
	}
	for dir := filepath.Dir(filepath.FromSlash(rel)); ; dir = filepath.Dir(dir) {
		file, err := os.Open(filepath.Join(root, dir, "conf.py"))
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(file, 65536))
			_ = file.Close()
			return readErr == nil && sphinxConfigHasSuffix(string(body), ext)
		}
		if dir == "." {
			return false
		}
	}
}

func sphinxConfigHasSuffix(body, ext string) bool {
	const key = "source_suffix"
	for position := 0; position < len(body); {
		end := strings.IndexByte(body[position:], '\n')
		if end < 0 {
			end = len(body)
		} else {
			end += position
		}
		line := body[position:end]
		if !strings.HasPrefix(line, key) {
			position = min(end+1, len(body))
			continue
		}
		cursor := position + len(key)
		for cursor < end && (body[cursor] == ' ' || body[cursor] == '\t') {
			cursor++
		}
		if cursor >= end || body[cursor] != '=' {
			position = min(end+1, len(body))
			continue
		}
		cursor++
		for cursor < end && (body[cursor] == ' ' || body[cursor] == '\t') {
			cursor++
		}
		if cursor >= end {
			return false
		}
		stop := end
		closing := byte(0)
		switch body[cursor] {
		case '[':
			closing = ']'
		case '(':
			closing = ')'
		case '{':
			closing = '}'
		}
		if closing != 0 {
			if index := strings.IndexByte(body[cursor:], closing); index >= 0 {
				stop = cursor + index
			}
		}
		value := body[cursor:stop]
		for _, match := range sphinxQuotedSuffix.FindAllStringSubmatchIndex(value, -1) {
			if value[match[2]:match[3]] != ext {
				continue
			}
			if strings.HasPrefix(value, "{") && !sphinxRSTValue.MatchString(value[match[1]:]) {
				continue
			}
			return true
		}
		return false
	}
	return false
}
