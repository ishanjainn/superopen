package install

import "testing"

func TestVSCodeSharesCopilotHarnessInstall(t *testing.T) {
	for _, name := range []string{"vscode", "copilot-cli"} {
		got, err := vendorsFromArg(name)
		if err != nil || len(got) != 1 || got[0] != "copilot-cli" {
			t.Fatalf("%s resolves to %v, err=%v", name, got, err)
		}
	}
	all, err := vendorsFromArg("all")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, name := range all {
		if name == "copilot-cli" {
			count++
		}
		if name == "vscode" {
			t.Fatal("all installs duplicate VS Code hooks")
		}
	}
	if count != 1 {
		t.Fatalf("Copilot harness installs = %d", count)
	}
}
