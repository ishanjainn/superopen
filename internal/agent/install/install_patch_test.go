package install

import (
	"encoding/json"
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

// Windows paths are the reason patchManifestBytes has to JSON-escape: a raw
// `C:\Users\...` yields the invalid escape `\U`, and a shell-quoted path's
// leading `"` closes the JSON string early ("invalid character 'C' after
// object key:value pair"). Both broke every Windows install.
func TestPatchManifestProducesValidJSONForWindowsPaths(t *testing.T) {
	for _, bin := range []string{
		`C:\Users\RUNNER~1\AppData\Local\Temp\TestInstall001\bin\so.exe`,
		`C:\Program Files\Superopen\so.exe`,
		`/tmp/bin/so`,
		`/tmp/dir with space/so`,
	} {
		t.Run(bin, func(t *testing.T) {
			raw, err := marketplaceFS.ReadFile("marketplace/plugins/cursor/hooks/hooks.json")
			if err != nil {
				t.Fatal(err)
			}
			patched := patchManifestBytes("hooks/hooks.json", raw, bin)

			var manifest struct {
				Hooks map[string][]struct {
					Command string `json:"command"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(patched, &manifest); err != nil {
				t.Fatalf("patched manifest is not valid JSON: %v\n%s", err, patched)
			}
			if len(manifest.Hooks) == 0 {
				t.Fatal("patched manifest decoded no hooks")
			}
			// The decoded command must still name the real binary, and must no
			// longer rely on a bare `so` resolved from PATH.
			for event, entries := range manifest.Hooks {
				for _, e := range entries {
					if !strings.Contains(e.Command, bin) {
						t.Fatalf("%s: command %q lost the binary path %q", event, e.Command, bin)
					}
					if strings.HasPrefix(e.Command, "so ") {
						t.Fatalf("%s: command %q still uses bare so", event, e.Command)
					}
				}
			}
		})
	}
}

func TestCodexStopHookUsesSessionsRefresh(t *testing.T) {
	// Codex has no SessionEnd; Stop is turn-end and must refresh (keep active)
	// rather than finalizing the session. Both the repo plugins/ tree and the
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

func TestCodexMarketplaceAndPluginUseSupportedSchema(t *testing.T) {
	marketplaceBody, err := marketplaceFS.ReadFile("marketplace/plugins/codex/.agents/plugins/marketplace.json")
	if err != nil {
		t.Fatalf("read embedded Codex marketplace: %v", err)
	}
	var marketplace struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Kind string `json:"source"`
				Path string `json:"path"`
			} `json:"source"`
			Policy struct {
				Installation   string `json:"installation"`
				Authentication string `json:"authentication"`
			} `json:"policy"`
			Category string `json:"category"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(marketplaceBody, &marketplace); err != nil {
		t.Fatalf("decode Codex marketplace: %v", err)
	}
	if marketplace.Name != "superopen" || len(marketplace.Plugins) != 1 {
		t.Fatalf("unexpected Codex marketplace: %+v", marketplace)
	}
	plugin := marketplace.Plugins[0]
	if plugin.Name != "superopen" || plugin.Source.Kind != "local" || plugin.Source.Path != "./plugins/superopen" {
		t.Fatalf("unexpected Codex plugin source: %+v", plugin)
	}
	if plugin.Policy.Installation == "" || plugin.Policy.Authentication == "" || plugin.Category == "" {
		t.Fatalf("Codex marketplace policy metadata is incomplete: %+v", plugin)
	}

	manifestBody, err := marketplaceFS.ReadFile("marketplace/plugins/codex/plugins/superopen/.codex-plugin/plugin.json")
	if err != nil {
		t.Fatalf("read embedded Codex plugin manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		t.Fatalf("decode Codex plugin manifest: %v", err)
	}
	for _, unsupported := range []string{"displayName", "hooks"} {
		if _, ok := manifest[unsupported]; ok {
			t.Fatalf("Codex plugin manifest contains unsupported field %q", unsupported)
		}
	}
	iface, ok := manifest["interface"].(map[string]any)
	if !ok || iface["longDescription"] == "" {
		t.Fatalf("Codex plugin manifest is missing interface.longDescription")
	}
	if prompts, ok := iface["defaultPrompt"].([]any); !ok || len(prompts) == 0 {
		t.Fatalf("Codex plugin manifest is missing interface.defaultPrompt")
	}
}
