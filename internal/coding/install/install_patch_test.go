package install

import (
	"strings"
	"testing"
)

func TestPatchManifestUsesAbsoluteBinaryForRefresh(t *testing.T) {
	body := []byte(`{"command":"so sessions refresh"}`)
	got := string(patchManifestBytes("hooks.json", body, "/tmp/bin/so"))
	if !strings.Contains(got, `"/tmp/bin/so sessions refresh"`) {
		t.Fatalf("refresh command was not patched: %s", got)
	}
}

func TestCodexStopHookUsesSessionsRefresh(t *testing.T) {
	// Codex has no SessionEnd; Stop is turn-end and must refresh (keep active)
	// rather than finalize (close + eval). Both the repo plugins/ tree and the
	// embedded install marketplace must agree.
	raw, err := marketplaceFS.ReadFile("marketplace/plugins/codex/plugins/superopen/hooks/hooks.json")
	if err != nil {
		t.Fatalf("read embedded codex hooks: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"so sessions refresh"`) {
		t.Fatalf("Codex Stop companion hook must invoke sessions refresh; hooks.json:\n%s", body)
	}
	if strings.Contains(body, `"so sessions finalize"`) {
		t.Fatalf("Codex hooks must not finalize on Stop (that closes every turn); hooks.json:\n%s", body)
	}
}

func TestSessionEndFinalizeIsDetached(t *testing.T) {
	raw, err := marketplaceFS.ReadFile("marketplace/plugins/claude-code/hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `so sessions finalize --detach`) {
		t.Fatalf("Claude SessionEnd must detach finalize; hooks.json:\n%s", body)
	}
	raw, err = marketplaceFS.ReadFile("marketplace/plugins/cursor/hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	body = string(raw)
	if !strings.Contains(body, `so sessions finalize --detach`) {
		t.Fatalf("Cursor sessionEnd must detach finalize; hooks.json:\n%s", body)
	}
}
