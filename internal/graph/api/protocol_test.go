package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearchDegreeFilterPreservesExplicitZero(t *testing.T) {
	zero := 0
	body, err := json.Marshal(SearchRequest{MaxDegree: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"max_degree":0`) {
		t.Fatalf("explicit zero was lost: %s", body)
	}
	var decoded SearchRequest
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.MaxDegree == nil || *decoded.MaxDegree != 0 {
		t.Fatalf("decoded max degree=%v", decoded.MaxDegree)
	}
	omitted, err := json.Marshal(SearchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(omitted), "max_degree") {
		t.Fatalf("unset max degree was serialized: %s", omitted)
	}
}
