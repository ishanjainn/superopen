package engine

import "testing"

func TestDetectLanguagePinnedFilenameSurface(t *testing.T) {
	t.Parallel()
	tests := map[string]DetectedLanguage{
		"main.go":                  {Language: "go", Grammar: "go"},
		"src/view.blade.php":       {Language: "blade", Grammar: "blade"},
		"Dockerfile":               {Language: "dockerfile", Grammar: "dockerfile"},
		"requirements-dev.txt":     {Language: "requirements", Grammar: "requirements"},
		"config/.env.production":   {Language: "dotenv", Grammar: "dotenv"},
		"deploy/kustomization.yml": {Language: "kustomize", Grammar: "yaml", Flavor: "kustomize"},
		"home/.ssh/config":         {Language: "sshconfig", Grammar: "sshconfig"},
		"module.S":                 {Language: "assembly", Grammar: "assembly"},
		"module.R":                 {Language: "r", Grammar: "r"},
	}
	for name, want := range tests {
		got, ok := DetectLanguage(name, nil, nil)
		if !ok || got != want {
			t.Errorf("DetectLanguage(%q) = %#v, %v; want %#v, true", name, got, ok, want)
		}
	}
	if _, ok := DetectLanguage("README", nil, nil); ok {
		t.Fatal("README must remain unknown")
	}
	if _, ok := DetectLanguage("package.json", nil, nil); ok {
		t.Fatal("package.json must remain reserved for package/config scanning")
	}
}

func TestDetectLanguagePinnedDisambiguation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, content, language, grammar string
	}{
		{"sample.m", "#import <Foundation/Foundation.h>\n@interface Foo : NSObject\n", "objc", "objc"},
		{"sample.m", "function MyFunc(x)\nend function;\n", "magma", "magma"},
		{"sample.m", "function y = square(x)\nend\n", "matlab", "matlab"},
		{"sample.cls", "Class Demo.Person Extends %Persistent\n", "objectscript_udl", "objectscript_udl"},
		{"sample.cls", "public class Demo {}\n", "apex", "apex"},
		{"sample.inc", "#define VALUE 1\n", "objectscript_routine", "objectscript_routine"},
		{"sample.inc", "require recipes-core/base.bb\n", "bitbake", "bitbake"},
		{"sample.xml", "<?xml version=\"1.0\"?><Export generator=\"Cache\">", "objectscript_export", ""},
		{"deployment.yaml", "apiVersion: apps/v1\nkind: Deployment\n", "yaml", "yaml"},
		{"values.yaml", "replicaCount: 1\n", "yaml", "yaml"},
	}
	for _, test := range tests {
		got, ok := DetectLanguage(test.name, []byte(test.content), nil)
		if !ok || got.Language != test.language || got.Grammar != test.grammar {
			t.Errorf("DetectLanguage(%q) = %#v, %v", test.name, got, ok)
		}
	}
}

func TestDetectLanguageOverridesMatchPinnedPrecedence(t *testing.T) {
	t.Parallel()
	got, ok := DetectLanguage("module.custom", nil, map[string]string{".custom": "python"})
	if !ok || got.Grammar != "python" {
		t.Fatalf("single override = %#v, %v", got, ok)
	}
	got, ok = DetectLanguage("view.tpl.php", nil, map[string]string{".tpl.php": "php"})
	if !ok || got.Grammar != "php" {
		t.Fatalf("compound override = %#v, %v", got, ok)
	}
	got, ok = DetectLanguage("view.blade.php", nil, map[string]string{".blade.php": "php"})
	if !ok || got.Grammar != "blade" {
		t.Fatalf("built-in compound must precede override: %#v, %v", got, ok)
	}
	got, ok = DetectLanguage("main.go", nil, map[string]string{".go": "not_a_language"})
	if !ok || got.Grammar != "go" {
		t.Fatalf("invalid override must fall back to the built-in: %#v, %v", got, ok)
	}
}

func TestEveryDetectedGrammarExists(t *testing.T) {
	t.Parallel()
	for extension, language := range extensionLanguages {
		got, ok := DetectLanguage("file"+extension, nil, nil)
		if !ok {
			t.Errorf("extension %q (%s) was not detected", extension, language)
			continue
		}
		if got.Grammar == "" {
			continue
		}
		if _, ok := GrammarExport(got.Grammar); !ok {
			t.Errorf("extension %q routes to missing grammar %q", extension, got.Grammar)
		}
	}
}
