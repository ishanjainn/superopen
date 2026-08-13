package mcp

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestContainedInRejectsSiblingPrefix(t *testing.T) {
	root := filepath.FromSlash("/Users/a/work")
	child := filepath.Join(root, ".cursor", "mcp.json")
	if !containedIn(root, child) {
		t.Fatalf("expected %s inside %s", child, root)
	}
	sibling := filepath.FromSlash("/Users/a/workother/.mcp.json")
	if containedIn(root, sibling) {
		t.Fatalf("sibling prefix must not count as contained: %s vs %s", root, sibling)
	}
	escaped := filepath.Clean(filepath.Join(root, "..", "secret", ".mcp.json"))
	if containedIn(root, escaped) {
		t.Fatalf("escaped path counted as contained: %s", escaped)
	}
}

func TestSamePathWindowsFold(t *testing.T) {
	if runtime.GOOS != "windows" {
		if samePath(`C:\Users\A`, `c:\users\a`) {
			t.Fatal("unix must be case-sensitive")
		}
		return
	}
	if !samePath(`C:\Users\A`, `c:\users\a`) {
		t.Fatal("windows paths should compare case-insensitively")
	}
}
