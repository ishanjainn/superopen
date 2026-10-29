package main

import (
	"fmt"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/spf13/cobra"
)

// groupHome is what a command group with no data of its own prints when run
// bare (AXI: content first, never help text): its subcommands as a table, and
// each one's usage line as a next step.
func groupHome(cmd *cobra.Command, _ []string) error {
	var rows []map[string]any
	var next []string
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		rows = append(rows, map[string]any{"command": c.Name(), "description": c.Short})
	}
	// Point agents at inspection commands before actions that need staged state.
	for _, name := range []string{"list", "scan", "search", "query", "show", "status", "run"} {
		if len(next) == 2 {
			break
		}
		if c, _, err := cmd.Find([]string{name}); err == nil && c != nil && c.Parent() == cmd && !c.Hidden {
			next = append(next, c.UseLine())
		}
	}
	for _, c := range cmd.Commands() {
		if len(next) == 2 {
			break
		}
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		found := false
		for _, hint := range next {
			if hint == c.UseLine() {
				found = true
				break
			}
		}
		if !found {
			next = append(next, c.UseLine())
		}
	}
	next = append(next, cmd.CommandPath()+" --help")
	out().Next(next...)
	return out().HumanOrJSON("group", func() {
		w := out().W
		fmt.Fprintf(w, "bin: %s\n", cli.BinPath())
		fmt.Fprintf(w, "description: %s\n", cmd.Short)
		fmt.Fprintf(w, "group: %s\n", cmd.CommandPath())
		o := out()
		o.Rows("commands", []string{"command", "description"}, rows)
	}, map[string]any{"bin": cli.BinPath(), "description": cmd.Short, "group": cmd.CommandPath(), "commands": rows})
}

// wireGroupHomes gives every command group that has no behavior of its own the
// shared home, so a new group can never fall back to printing help.
func wireGroupHomes(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c != root && c.HasSubCommands() && c.Run == nil && c.RunE == nil {
			c.RunE = groupHome
			c.Args = cobra.NoArgs
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
}
