package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/skills"
)

func TestInstallAllWritesSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("COPILOT_HOME", "")

	written, err := skills.InstallAll("/usr/local/bin/so")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("expected skill paths")
	}
	for _, path := range written {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		if !bytes.Contains(body, []byte("/usr/local/bin/so")) {
			t.Fatalf("%s missing absolute so path", path)
		}
		if bytes.Contains(body, []byte("__SO_BIN__")) {
			t.Fatalf("%s still has placeholder", path)
		}
		if filepath.Base(path) == "SKILL.md" {
			if !bytes.Contains(body, []byte("any question about a codebase")) {
				t.Fatalf("%s description must trip on any codebase question: %s", path, body[:300])
			}
			if !bytes.Contains(body, []byte("graph query first")) {
				t.Fatalf("%s description must treat codebase questions as graph query first", path)
			}
			if bytes.Contains(body, []byte("memory search first")) {
				t.Fatalf("%s description must not lead with memory search: %s", path, body[:400])
			}
			if bytes.Contains(bytes.ToLower(body), []byte("so init")) &&
				!bytes.Contains(body, []byte("unless the user explicitly")) {
				t.Fatalf("%s must not tell the agent to so init unprompted", path)
			}
		}
	}
}

// Reference files keep SKILL.md small: the agent loads a recipe on demand
// instead of carrying every query example in each session.
func TestInstallAllShipsReferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("COPILOT_HOME", "")

	written, err := skills.InstallAll("/usr/local/bin/so")
	if err != nil {
		t.Fatal(err)
	}
	var sawSkill, sawReference bool
	for _, path := range written {
		switch {
		case filepath.Base(path) == "SKILL.md":
			sawSkill = true
		case filepath.Base(filepath.Dir(path)) == "references":
			sawReference = true
		}
	}
	if !sawSkill {
		t.Fatal("SKILL.md was not installed")
	}
	if !sawReference {
		t.Fatalf("no reference files installed: %v", written)
	}

	body, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "so", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("references/query.md")) {
		t.Fatal("SKILL.md does not point at the reference file")
	}
	if !bytes.Contains(body, []byte("references/memory.md")) {
		t.Fatal("SKILL.md does not point at the memory reference")
	}
}
