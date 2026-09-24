package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyPromptMemoryCueFirstPerson(t *testing.T) {
	cases := []string{
		"how long is my commute to work",
		"how many siblings do I have",
		"where did I buy the coffee maker",
		"what did I name my playlist",
		"where do I shop for groceries",
	}
	for _, p := range cases {
		if got := classifyPrompt(p); got != routeMemory {
			t.Errorf("classifyPrompt(%q)=%q want memory", p, got)
		}
	}
}

func TestClassifyPromptCodeStillWins(t *testing.T) {
	cases := []string{
		"how does foo.go architecture work",
		"implement the handler in internal/api/foo.go",
		"where is the queryset for User",
		"how many endpoints does the architecture expose",
		"How does Django's ORM build and evaluate a QuerySet?",
	}
	for _, p := range cases {
		if got := classifyPrompt(p); got != routeCode {
			t.Errorf("classifyPrompt(%q)=%q want code", p, got)
		}
	}
}

func TestClassifyPromptDoesNotStealSourceCounts(t *testing.T) {
	// Bare "how many" / "playlist" must not divert a source question to diary.
	for _, p := range []string{
		"how many tests fail",
		"how many files in the tree",
		"add a playlist feature",
	} {
		if got := classifyPrompt(p); got == routeMemory {
			t.Errorf("classifyPrompt(%q)=memory; source-shaped counts must not take the diary path", p)
		}
	}
}

func TestClassifyPromptCaptureVsRecall(t *testing.T) {
	for _, p := range []string{
		"remember this: login timeout is 30s",
		"please remember this for later",
		"don't forget we use jsonData for the Prometheus UID",
		"save this",
		"jot this down",
		"keep this: never put credentials in jsonData",
	} {
		if got := classifyPrompt(p); got != routeCapture {
			t.Errorf("classifyPrompt(%q)=%q want capture", p, got)
		}
	}
	if got := classifyPrompt("do you remember where we left the login timeout"); got != routeMemory {
		t.Errorf("question remember must stay recall, got %q", got)
	}
	if got := classifyPrompt("do you remember this timeout"); got != routeMemory {
		t.Errorf("do you remember this must stay recall, got %q", got)
	}
}

func TestClassifyPromptContributorNotesAreCapture(t *testing.T) {
	p := "Code-shaped questions beat notes-shaped ones. That rule belongs in the contributor notes, not as a comment on the function. Leave the source as it is."
	if got := classifyPrompt(p); got != routeCapture {
		t.Fatalf("classifyPrompt(%q)=%q want capture", p, got)
	}
	if got := classifyPrompt("how does this function work"); got != routeCode {
		t.Fatalf("question about a function must stay code, got %q", got)
	}
	if got := classifyPrompt("where is the class in internal/foo.go"); got != routeCode {
		t.Fatalf("path question must stay code, got %q", got)
	}
	if got := classifyPrompt("add a comment on the function"); got == routeCode {
		t.Fatal("bare function must not force code")
	}
}

func TestWeakDeclDoesNotSteerGraph(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"session_id": "weak-decl",
		"cwd":        root,
		"prompt":     "add a comment on the function",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("cursor", "beforeSubmitPrompt", payload)
	if ok && strings.Contains(text, "graph query") {
		t.Fatalf("a bare function mention must not steer the graph, got %q", text)
	}
	notes := "Code-shaped questions beat notes-shaped ones. That rule belongs in the contributor notes, not as a comment on the function. Leave the source as it is."
	payload, err = json.Marshal(map[string]any{
		"session_id": "notes-capture",
		"cwd":        root,
		"prompt":     notes,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok = steerTextFor("cursor", "beforeSubmitPrompt", payload)
	if !ok || !strings.Contains(text, "memory capture") {
		t.Fatalf("contributor notes must steer memory capture, ok=%v text=%q", ok, text)
	}
}
