package repofile

import "testing"

func TestAcceptReadJSONAndMakefile(t *testing.T) {
	if got := Accept("src/app.ts", "Read", ""); got != "src/app.ts" {
		t.Fatalf("app.ts: %q", got)
	}
	if got := Accept("Makefile", "Read", ""); got != "Makefile" {
		t.Fatalf("Makefile: %q", got)
	}
	if got := Accept("go.mod", "Read", ""); got != "go.mod" {
		t.Fatalf("go.mod: %q", got)
	}
	if got := PathFromJSON(`{"file_path":"src/app.ts"}`); got != "src/app.ts" {
		t.Fatalf("json: %q", got)
	}
}

func TestAcceptRejectsShellCommand(t *testing.T) {
	cmd := `so graph query "who wraps app"`
	if got := Accept(cmd, "shell", ""); got != "" {
		t.Fatalf("shell tool stamped command: %q", got)
	}
	if got := Accept(cmd, "Read", ""); got != "" {
		t.Fatalf("command-shaped path: %q", got)
	}
	if got := PathFromJSON(cmd); got != "" {
		t.Fatalf("raw command parsed as json path: %q", got)
	}
	if got := Accept("ls -la", "Bash", ""); got != "" {
		t.Fatalf("bash flags: %q", got)
	}
}

func TestStateFromToolNameNotCommandText(t *testing.T) {
	if State("Read", "coding_agent.tool.call") != "read" {
		t.Fatal("Read should be read")
	}
	if State("shell", "coding_agent.tool.call") != "seen" {
		t.Fatal("shell must not become read because the command contains the word query")
	}
	if State("afterFileEdit", "") != "edited" {
		t.Fatal("afterFileEdit should be edited")
	}
}
