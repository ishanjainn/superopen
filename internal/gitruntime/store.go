// Package gitruntime stores Superopen session blobs in git side refs
// (refs/so/sessions/<id>) without checking out or dirtying the feature branch.
// Live session state lives under .git/so-sessions/.
package gitruntime

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// SessionsRefPrefix is the git ref namespace for durable session blobs.
	SessionsRefPrefix = "refs/so/sessions/"
	// StateDirName is the on-disk live-state directory under .git/.
	StateDirName = "so-sessions"
)

var safeRef = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// RefName returns refs/so/sessions/<sanitized-id>.
func RefName(sessionID string) string {
	id := strings.TrimSpace(sessionID)
	id = safeRef.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")
	if id == "" {
		id = "unknown"
	}
	return SessionsRefPrefix + id
}

// StateDir returns .git/so-sessions for live phase/branch state.
func StateDir(repoRoot string) string {
	gitDir := resolveGitDir(repoRoot)
	if gitDir == "" {
		return filepath.Join(repoRoot, ".git", StateDirName)
	}
	return filepath.Join(gitDir, StateDirName)
}

func resolveGitDir(repoRoot string) string {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-dir").Output()
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoRoot, p)
	}
	return p
}

// WriteSession commits files into refs/so/sessions/<id> without touching HEAD.
// files keys are paths inside the session tree (e.g. meta.json, transcript.jsonl).
func WriteSession(repoRoot, sessionID string, files map[string][]byte) (string, error) {
	if repoRoot == "" || sessionID == "" {
		return "", fmt.Errorf("repo root and session id required")
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no session files")
	}
	ref := RefName(sessionID)
	tmp, err := os.MkdirTemp("", "so-session-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	var treeLines []string
	for name, body := range files {
		name = strings.TrimPrefix(filepath.ToSlash(name), "/")
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		full := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, body, 0o644); err != nil {
			return "", err
		}
		hash, err := hashObject(repoRoot, full)
		if err != nil {
			return "", err
		}
		treeLines = append(treeLines, fmt.Sprintf("100644 blob %s\t%s", hash, name))
	}
	if len(treeLines) == 0 {
		return "", fmt.Errorf("no valid session files")
	}
	treeHash, err := mktree(repoRoot, strings.Join(treeLines, "\n")+"\n")
	if err != nil {
		return "", err
	}
	parent := ""
	if out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--verify", ref).Output(); err == nil {
		parent = strings.TrimSpace(string(out))
	}
	commit, err := commitTree(repoRoot, treeHash, parent, fmt.Sprintf("so session %s", sessionID))
	if err != nil {
		return "", err
	}
	if err := updateRefCAS(repoRoot, ref, commit, parent); err != nil {
		return "", err
	}
	return commit, nil
}

// ReadFile returns one path from refs/so/sessions/<id>.
func ReadFile(repoRoot, sessionID, path string) ([]byte, error) {
	ref := RefName(sessionID)
	spec := fmt.Sprintf("%s:%s", ref, filepath.ToSlash(path))
	out, err := exec.Command("git", "-C", repoRoot, "show", spec).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s: %w", spec, err)
	}
	return out, nil
}

// ListSessionIDs returns session ids that have side refs.
func ListSessionIDs(repoRoot string) ([]string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "for-each-ref", "--format=%(refname)", SessionsRefPrefix).Output()
	if err != nil {
		return nil, nil
	}
	var ids []string
	prefix := SessionsRefPrefix
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		ids = append(ids, strings.TrimPrefix(line, prefix))
	}
	return ids, nil
}

// PushSessionsFF pushes refs/so/sessions/* to remote with fast-forward only (never force).
func PushSessionsFF(repoRoot, remote string) error {
	if remote == "" {
		remote = "origin"
	}
	ids, _ := ListSessionIDs(repoRoot)
	if len(ids) == 0 {
		return nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "push", "--atomic", remote, "refs/so/sessions/*:refs/so/sessions/*")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "src refspec") || strings.Contains(msg, "does not match any") {
			return nil
		}
		return fmt.Errorf("push session refs (FF only): %s", msg)
	}
	return nil
}

// SnapshotSessionDir packs meta/transcript/footprint from a filesystem session
// directory into the side ref.
func SnapshotSessionDir(repoRoot, sessionDir, sessionID string) (string, error) {
	files := map[string][]byte{}
	for _, name := range []string{"meta.json", "transcript.jsonl", "footprint.json"} {
		data, err := os.ReadFile(filepath.Join(sessionDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		files[name] = data
	}
	if len(files) == 0 {
		return "", fmt.Errorf("empty session dir")
	}
	return WriteSession(repoRoot, sessionID, files)
}

func hashObject(repoRoot, path string) (string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "hash-object", "-w", path).Output()
	if err != nil {
		return "", fmt.Errorf("hash-object: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func mktree(repoRoot, treeInput string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "mktree")
	cmd.Stdin = strings.NewReader(treeInput)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mktree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func commitTree(repoRoot, tree, parent, msg string) (string, error) {
	args := []string{"-C", repoRoot, "commit-tree", tree}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, "-m", msg)
	now := time.Now().UTC().Format(time.RFC3339)
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=superopen",
		"GIT_AUTHOR_EMAIL=so@localhost",
		"GIT_COMMITTER_NAME=superopen",
		"GIT_COMMITTER_EMAIL=so@localhost",
		"GIT_AUTHOR_DATE="+now,
		"GIT_COMMITTER_DATE="+now,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("commit-tree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func updateRefCAS(repoRoot, ref, newSHA, oldSHA string) error {
	args := []string{"-C", repoRoot, "update-ref", ref, newSHA}
	if oldSHA != "" {
		args = append(args, oldSHA)
	}
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("update-ref: %s", msg)
	}
	return nil
}
