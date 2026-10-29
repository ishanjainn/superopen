package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
)

// walkCommands visits every command below root except help and completion.
func walkCommands(root *cobra.Command, visit func(path []string, cmd *cobra.Command)) {
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		visit(path, cmd)
		for _, child := range cmd.Commands() {
			if child.Name() == "help" || child.Name() == "completion" {
				continue
			}
			walk(child, append(append([]string(nil), path...), child.Name()))
		}
	}
	walk(root, nil)
}

func TestEveryCommandRejectsUnknownFlagsJSON(t *testing.T) {
	walkCommands(newRootCommand(), func(path []string, _ *cobra.Command) {
		root := newRootCommand()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(append(append([]string{"--json"}, path...), "--definitely-not-a-flag"))
		cmd, err := root.ExecuteC()
		if err == nil {
			t.Errorf("so %s: unknown flag succeeded", strings.Join(path, " "))
			return
		}
		err = cli.WithUsageHint(cli.QuietUnmanaged(cli.NormalizeExit(err)), cmd)
		var output bytes.Buffer
		(&cli.Out{Flags: cli.Flags{JSON: true}, W: &output, ErrW: io.Discard}).WriteError(err)
		var response struct {
			OK    bool   `json:"ok"`
			Code  int    `json:"code"`
			Error string `json:"error"`
			Hint  string `json:"hint"`
		}
		if json.Unmarshal(output.Bytes(), &response) != nil || response.OK || response.Code != cli.ExitUsage || response.Error == "" || response.Hint == "" {
			t.Errorf("so %s: unstructured JSON usage error: %s", strings.Join(path, " "), output.String())
		}
	})
}

// An agent that invents a flag must learn it did nothing (AXI §6): exit 2,
// `error:` on stdout, and a help[] line naming the usage and valid flags.
func TestEveryCommandRejectsUnknownFlagsStructured(t *testing.T) {
	probe := newRootCommand()
	var paths [][]string
	walkCommands(probe, func(path []string, _ *cobra.Command) { paths = append(paths, path) })
	if len(paths) < 60 {
		t.Fatalf("command tree looks truncated: %d commands", len(paths))
	}
	for _, path := range paths {
		name := "so " + strings.Join(path, " ")
		root := newRootCommand()
		var stdout bytes.Buffer
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(append(append([]string(nil), path...), "--definitely-not-a-flag"))
		cmd, err := root.ExecuteC()
		if err == nil {
			t.Fatalf("%s: an unknown flag must fail", name)
		}
		err = cli.WithUsageHint(cli.QuietUnmanaged(cli.NormalizeExit(err)), cmd)
		if code := cli.ExitCode(err); code != cli.ExitUsage {
			t.Fatalf("%s: exit %d, want %d (%v)", name, code, cli.ExitUsage, err)
		}
		o := &cli.Out{W: &stdout, ErrW: io.Discard}
		o.WriteError(err)
		got := stdout.String()
		if !strings.HasPrefix(got, "error: ") || !strings.Contains(got, "help[1]:\n  usage: ") {
			t.Fatalf("%s: structured error missing:\n%s", name, got)
		}
	}
}

// Every command documents itself for the one-turn fallback (AXI §10).
func TestEveryCommandHasShortHelp(t *testing.T) {
	walkCommands(newRootCommand(), func(path []string, cmd *cobra.Command) {
		if len(path) > 0 && strings.TrimSpace(cmd.Short) == "" {
			t.Errorf("so %s has no Short description", strings.Join(path, " "))
		}
	})
}

func TestEveryCommandOffersConciseHelp(t *testing.T) {
	audited := 0
	walkCommands(newRootCommand(), func(path []string, _ *cobra.Command) {
		audited++
		root := newRootCommand()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs(append(append([]string(nil), path...), "--help"))
		if err := root.Execute(); err != nil {
			t.Errorf("so %s --help: %v", strings.Join(path, " "), err)
			return
		}
		got := output.String()
		for _, want := range []string{"Usage:", "--help", "--json", "--full"} {
			if !strings.Contains(got, want) {
				t.Errorf("so %s --help lacks %q: %q", strings.Join(path, " "), want, got)
			}
		}
		if lines := strings.Count(got, "\n"); lines > 60 {
			t.Errorf("so %s --help is %d lines; keep the fallback concise", strings.Join(path, " "), lines)
		}
	})
	t.Logf("AXI help contract audited %d commands", audited)
}

// Run bare, a command group shows data or a table of its subcommands, never
// cobra's help text (AXI: content first).
func TestEveryCommandGroupHasABareBehavior(t *testing.T) {
	walkCommands(newRootCommand(), func(path []string, cmd *cobra.Command) {
		if len(path) == 0 || !cmd.HasSubCommands() {
			return
		}
		if cmd.RunE == nil && cmd.Run == nil {
			t.Errorf("so %s is a group with no bare behavior; cobra would print help", strings.Join(path, " "))
		}
	})
}

func TestBlocksHomeSuggestsCommandsUsableWithoutStagedScripts(t *testing.T) {
	root := newRootCommand()
	root.SetErr(io.Discard)
	root.SetArgs([]string{"blocks"})
	got := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(got, "  so blocks scan\n") || !strings.Contains(got, "  so blocks run\n") || strings.Contains(got, "  so blocks apply\n") {
		t.Fatalf("blocks home suggested a staged-only action: %s", got)
	}
}

// An unknown subcommand names the valid ones (AXI §6).
func TestUnknownSubcommandListsTheValidOnes(t *testing.T) {
	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"graph", "artifact", "nope"})
	cmd, err := root.ExecuteC()
	err = cli.WithUsageHint(cli.NormalizeExit(err), cmd)
	var buf bytes.Buffer
	(&cli.Out{W: &buf, ErrW: io.Discard}).WriteError(err)
	for _, want := range []string{"error: unknown command", "help[1]:\n  commands: export, import, verify"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, buf.String())
		}
	}
	if cli.ExitCode(err) != cli.ExitUsage {
		t.Fatalf("exit %d", cli.ExitCode(err))
	}
}

// --json is honored by every command, version included.
func TestVersionHonorsJSON(t *testing.T) {
	root := newRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"kind":"version"`) || !strings.Contains(buf.String(), `"ok":true`) {
		t.Fatalf("version --json must print the shared envelope: %q", buf.String())
	}
}
