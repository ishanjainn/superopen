package engine

import "testing"

func TestPlanIncrementalChangesAddModifyDeleteRename(t *testing.T) {
	prior := incrementalSnapshot{revision: "old", files: map[string]string{
		"same.go": "same", "modified.go": "old", "deleted.go": "deleted", "old/name.go": "rename",
	}}
	current := map[string]string{
		"same.go": "same", "modified.go": "new", "added.go": "added", "new/name.go": "rename",
	}
	got := planIncrementalChanges(prior, current, "new")
	if got.Unchanged != 1 || !got.RevisionChanged || got.RequiresFull || len(got.Added) != 1 ||
		len(got.Modified) != 1 || len(got.Deleted) != 1 || len(got.Renamed) != 1 ||
		got.Renamed[0].OldPath != "old/name.go" || got.Renamed[0].Path != "new/name.go" {
		t.Fatalf("changes = %#v", got)
	}
}

func TestPlanIncrementalChangesDoesNotGuessDuplicateRename(t *testing.T) {
	prior := incrementalSnapshot{files: map[string]string{"old-a": "same", "old-b": "same"}}
	current := map[string]string{"new-a": "same", "new-b": "same"}
	got := planIncrementalChanges(prior, current, "")
	if len(got.Renamed) != 0 || len(got.Added) != 2 || len(got.Deleted) != 2 {
		t.Fatalf("duplicate content was guessed as rename: %#v", got)
	}
}
