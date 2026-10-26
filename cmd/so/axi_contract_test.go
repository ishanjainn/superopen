package main

import (
	"bytes"
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
