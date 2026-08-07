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
