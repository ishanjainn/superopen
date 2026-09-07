package hook

import "testing"

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
