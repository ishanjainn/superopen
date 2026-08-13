package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

func retrievalStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	s := NewStore(paths)
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestConcurrentMemoryWritesRemainValid(t *testing.T) {
	s := retrievalStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.UpsertPattern(Pattern{Fingerprint: "fp_concurrent", Vendor: "codex", Kind: "workflow", Summary: "Use focused tests", Confidence: .8}, fmt.Sprintf("s%d", i), i%2 == 0)
			if err != nil {
				t.Errorf("write %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	patterns, err := s.ListPatterns()
	if err != nil || len(patterns) != 1 || patterns[0].Occurrences != 20 {
		t.Fatalf("patterns=%+v err=%v", patterns, err)
	}
	if _, err := os.Stat(filepath.Join(s.Paths.MemoryDir, "state.json.lock")); !os.IsNotExist(err) {
		t.Fatalf("repository-local lock should not exist: %v", err)
	}
}

func TestRetrieveEligibilityIsolationAndDeduplication(t *testing.T) {
	s := retrievalStore(t)
	p := Pattern{Fingerprint: "fp_auth", Vendor: "codex", Kind: "workflow", Summary: "Run focused authentication tests after editing auth middleware", Keywords: []string{"authentication", "middleware", "tests"}, Confidence: .8}
	if _, err := s.UpsertPattern(p, "s1", false); err != nil {
		t.Fatal(err)
	}
	query := "run focused authentication tests after editing auth middleware"
	if hits, err := s.Retrieve(RetrievalQuery{Text: query, Vendor: "codex"}); err != nil || len(hits) != 0 {
		t.Fatalf("unverified first occurrence injected: hits=%v err=%v", hits, err)
	}
	if _, err := s.UpsertPattern(p, "s2", false); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Retrieve(RetrievalQuery{Text: query, Vendor: "codex"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("eligible pattern missing: hits=%v err=%v", hits, err)
	}
	if other, _ := s.Retrieve(RetrievalQuery{Text: query, Vendor: "cursor"}); len(other) != 0 {
		t.Fatalf("cross-vendor retrieval: %v", other)
	}
	seen := map[string]string{hits[0].Fingerprint: hits[0].ContentID}
	if again, _ := s.Retrieve(RetrievalQuery{Text: query, Vendor: "codex", Seen: seen}); len(again) != 0 {
		t.Fatalf("duplicate retrieval: %v", again)
	}
}

func TestManualRetrieveIncludesFirstOccurrence(t *testing.T) {
	s := retrievalStore(t)
	p := Pattern{Fingerprint: "fp_first", Vendor: "cursor", Kind: "workflow", Summary: "Run focused authentication tests after editing authentication middleware", Keywords: []string{"authentication", "middleware", "tests"}, Confidence: .6}
	if _, err := s.UpsertPattern(p, "s1", false); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Retrieve(RetrievalQuery{Text: p.Summary, Vendor: "cursor", Mode: RetrievalManual})
	if err != nil || len(hits) != 1 || !containsString(hits[0].Reasons, "unverified") {
		t.Fatalf("manual first occurrence: hits=%+v err=%v", hits, err)
	}
	if automatic, err := s.Retrieve(RetrievalQuery{Text: p.Summary, Vendor: "cursor"}); err != nil || len(automatic) != 0 {
		t.Fatalf("automatic first occurrence: hits=%+v err=%v", automatic, err)
	}
}

func TestRetrieveReturnsRankedPartialHitsWhenBudgetExpires(t *testing.T) {
	s := retrievalStore(t)
	for _, id := range []string{"one", "two"} {
		p := Pattern{Fingerprint: id, Vendor: "codex", Kind: "workflow", Summary: "Run focused tests for authentication", Confidence: .9, ExplicitWorkflow: true}
		if _, err := s.UpsertPattern(p, "s-"+id, true); err != nil {
			t.Fatal(err)
		}
	}
	checks := 0
	hits, err := s.Retrieve(RetrievalQuery{Text: "focused authentication tests", Vendor: "codex", expired: func() bool {
		checks++
		return checks > 1
	}})
	if err != nil || len(hits) != 1 {
		t.Fatalf("partial retrieval: hits=%+v err=%v", hits, err)
	}
}

func TestRetrieveFailsOpenForCorruptState(t *testing.T) {
	s := retrievalStore(t)
	if err := os.WriteFile(s.statePath(), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Retrieve(RetrievalQuery{Text: "anything", Vendor: "codex"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("corrupt memory must fail open: hits=%v err=%v", hits, err)
	}
}

func TestFileRecallRequiresCurrentExactPath(t *testing.T) {
	s := retrievalStore(t)
	target := filepath.Join(s.Paths.RepoRoot, "internal", "auth.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("package auth\n"))
	p := Pattern{Fingerprint: "fp_file", Vendor: "codex", Kind: "success", Summary: "Preserve auth validation order", TargetPath: "internal/auth.go", Paths: []string{"internal/auth.go"}, SourceSHA256: hex.EncodeToString(sum[:]), Confidence: .9, ExplicitWorkflow: true}
	if _, err := s.UpsertPattern(p, "s1", true); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Retrieve(RetrievalQuery{Vendor: "codex", Paths: []string{"internal/auth.go"}, FileOnly: true, MaxTokens: 250})
	if err != nil || len(hits) != 1 {
		t.Fatalf("exact file recall missing: %v %v", hits, err)
	}
	if err := os.WriteFile(target, []byte("package auth\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if stale, _ := s.Retrieve(RetrievalQuery{Vendor: "codex", Paths: []string{"internal/auth.go"}, FileOnly: true}); len(stale) != 0 {
		t.Fatalf("stale file pattern injected: %v", stale)
	}
}

func TestFeedbackConfidenceAndDismissal(t *testing.T) {
	s := retrievalStore(t)
	p := Pattern{Fingerprint: "fp_feedback", Vendor: "codex", Kind: "workflow", Summary: "Use focused tests", Confidence: .5, ExplicitWorkflow: true}
	if _, err := s.UpsertPattern(p, "s1", false); err != nil {
		t.Fatal(err)
	}
	helpful, err := s.FeedbackPattern("fp_feedback", "codex", "helpful", "")
	if err != nil || helpful.Confidence != .55 {
		t.Fatalf("helpful confidence=%v err=%v", helpful.Confidence, err)
	}
	incorrect, err := s.FeedbackPattern("fp_feedback", "codex", "incorrect", "bad advice")
	if err != nil || incorrect.Status != "dismissed" || incorrect.Confidence != 0 {
		t.Fatalf("incorrect=%+v err=%v", incorrect, err)
	}
	if hits, _ := s.Retrieve(RetrievalQuery{Text: "focused tests", Vendor: "codex"}); len(hits) != 0 {
		t.Fatalf("dismissed pattern returned: %v", hits)
	}
}

func TestFormatRetrievalIsCompactAndAdvisory(t *testing.T) {
	text := FormatRetrieval([]RetrievalHit{{Fingerprint: "fp", Summary: "Use focused tests", Occurrences: 2, Verified: 1, Confidence: .8}})
	if !strings.Contains(text, "Advisory historical evidence") || !strings.Contains(text, "[fp]") {
		t.Fatal(text)
	}
}

func TestPathNormalizationIsCrossPlatform(t *testing.T) {
	if got := normalizePathKey(`C:\Repo\Internal\Auth.go`); got != "c:/repo/internal/auth.go" {
		t.Fatalf("drive path = %q", got)
	}
	if got := normalizePathKey(`\\Server\Share\Repo\Auth.go`); got != "//server/share/repo/auth.go" {
		t.Fatalf("UNC path = %q", got)
	}
	if pathScore(normalizePaths([]string{`internal\Auth.go`}), []string{"internal/auth.go"}) != 1 {
		t.Fatal("separator normalization failed")
	}
}
