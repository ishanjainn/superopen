package engine

import (
	"os"
	"path/filepath"
	"strings"
)

func fileMayHaveShebang(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var prefix [2]byte
	n, _ := file.Read(prefix[:])
	return n == 2 && prefix[0] == '#' && prefix[1] == '!'
}

func languageFromShebang(content []byte) string {
	if len(content) < 2 || content[0] != '#' || content[1] != '!' {
		return ""
	}
	end := len(content)
	if newline := strings.IndexByte(string(content[:min(end, 256)]), '\n'); newline >= 0 {
		end = newline
	} else if end > 255 {
		return ""
	}
	line := strings.TrimSuffix(string(content[:end]), "\r")
	if strings.ContainsRune(line, 0) {
		return ""
	}
	words := strings.Fields(line[2:])
	if len(words) == 0 {
		return ""
	}
	interpreter := filepath.Base(words[0])
	if interpreter == "env" {
		words = words[1:]
		if len(words) > 0 && (words[0] == "-S" || words[0] == "--split-string") {
			words = words[1:]
		}
		if len(words) == 0 || strings.HasPrefix(words[0], "-") || strings.Contains(words[0], "=") {
			return ""
		}
		interpreter = filepath.Base(words[0])
	}
	if strings.HasPrefix(interpreter, "python") {
		version := strings.TrimPrefix(interpreter, "python")
		if version == "" {
			return "python"
		}
		for _, part := range strings.Split(version, ".") {
			if part == "" {
				return ""
			}
			for _, digit := range part {
				if digit < '0' || digit > '9' {
					return ""
				}
			}
		}
		return "python"
	}
	return map[string]string{
		"sh": "bash", "bash": "bash", "dash": "bash", "ksh": "bash", "zsh": "bash",
		"node": "javascript", "nodejs": "javascript", "ruby": "ruby", "perl": "perl",
		"php": "php", "lua": "lua",
	}[interpreter]
}
