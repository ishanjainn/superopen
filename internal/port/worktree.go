package port

import (
	"os"
	"path/filepath"
)

// RemapCWD applies destination worktree authority.
// Never re-derive cwd from encoded vendor path names - use PortableSession.CWD,
// then remap to destRepoRoot when the recorded path is missing or intentionally
// relocating into this workspace.
func RemapCWD(sess *PortableSession, destRepoRoot string) {
	if sess == nil || destRepoRoot == "" {
		return
	}
	destRepoRoot = filepath.Clean(destRepoRoot)
	if sess.SourceMetadata == nil {
		sess.SourceMetadata = map[string]any{}
	}
	orig := sess.CWD
	if orig == "" {
		sess.CWD = destRepoRoot
		return
	}
	orig = filepath.Clean(orig)
	if orig == destRepoRoot {
		return
	}
	if _, err := os.Stat(orig); err != nil {
		sess.SourceMetadata["original_cwd"] = orig
		sess.CWD = destRepoRoot
		return
	}
	// Same basename (typical git worktree layout) → prefer dest authority.
	if filepath.Base(orig) == filepath.Base(destRepoRoot) {
		sess.SourceMetadata["original_cwd"] = orig
		sess.CWD = destRepoRoot
		return
	}
	// Exporting into this workspace: stamp dest as authority for writers.
	sess.SourceMetadata["original_cwd"] = orig
	sess.CWD = destRepoRoot
}
