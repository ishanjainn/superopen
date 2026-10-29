package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/blocks"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/compact"
)

func cmdBlocks() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocks",
		Short: "Reusable scripts staged until you apply them",
	}
	cmd.AddCommand(blocksScanCmd(), blocksInitCmd(), blocksApplyCmd(), blocksDeclineCmd(), blocksRunCmd())
	return cmd
}

func blocksScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Rank shell commands repeated across local sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			hits, err := blocks.Scan(root)
			if err != nil {
				return err
			}
			if len(hits) == 0 {
				out().Next("so blocks --help")
				out().Empty("scripts")
				return nil
			}
			rows := make([]map[string]any, 0, len(hits))
			hasBlock := false
			for _, h := range hits {
				block := h.Block
				if block == "" {
					block = "none"
				} else {
					hasBlock = true
				}
				rows = append(rows, map[string]any{"sessions": h.Count, "command": h.Command, "block": block})
			}
			if hasBlock {
				out().Next("so blocks init  (stage matched scripts only)")
			}
			out().Rows("scripts", []string{"sessions", "block", "command"}, rows)
			return nil
		},
	}
}

func blocksInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Stage an index of repeated matching scripts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			path, err := blocks.Init(root)
			if err != nil {
				return err
			}
			o := out()
			if path == "" {
				o.Next("so blocks scan")
				return o.HumanOrJSON("blocks", func() { fmt.Fprint(o.W, "status: none\nreason: no repeated matching scripts\n") }, map[string]any{"status": "none", "reason": "no repeated matching scripts"})
			}
			o.Next("so blocks apply", "so blocks decline")
			data := map[string]any{"status": "staged", "path": path}
			return o.HumanOrJSON("blocks", func() { fmt.Fprint(o.W, cli.Toon(data, o.Flags.Full)) }, data)
		},
	}
}

func blocksApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Write the staged script index into AGENTS.md",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			if err := blocks.Apply(root); err != nil {
				if os.IsNotExist(err) {
					return cli.Fail(cli.ExitNotFound, "no staged script index", "so blocks init")
				}
				return err
			}
			o := out()
			data := map[string]any{"status": "applied", "file": "AGENTS.md"}
			return o.HumanOrJSON("blocks", func() { fmt.Fprint(o.W, cli.Toon(data, o.Flags.Full)) }, data)
		},
	}
}

func blocksDeclineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "decline",
		Short: "Drop the staged script index",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			if err := blocks.Decline(root); err != nil {
				return err
			}
			o := out()
			data := map[string]any{"status": "declined"}
			return o.HumanOrJSON("blocks", func() { fmt.Fprint(o.W, cli.Toon(data, o.Flags.Full)) }, data)
		},
	}
}

func blocksRunCmd() *cobra.Command {
	var raw bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one built-in script",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			rows := make([]map[string]any, 0, len(blocks.Names))
			for _, name := range blocks.Names {
				rows = append(rows, map[string]any{"block": name, "purpose": blockDescription(name)})
			}
			out().Next("so blocks run --help")
			out().Rows("blocks", []string{"block", "purpose"}, rows)
			return nil
		},
	}
	cmd.PersistentFlags().BoolVar(&raw, "raw", false, "Print only the script output for a pipe")
	for _, name := range blocks.Names {
		name := name
		child := &cobra.Command{
			Use:   name,
			Short: blockDescription(name),
			RunE: func(cmd *cobra.Command, args []string) error {
				if raw && out().Flags.JSON {
					return cli.Usage("--raw and --json cannot be combined", "use --raw for bytes or --json for a structured result")
				}
				root := repoRoot()
				if err := failIfUnmanaged(root); err != nil {
					return err
				}
				return runBlock(cmd, root, name, args, raw)
			},
		}
		if name == "wait-for" {
			child.Flags().Duration("timeout", 30*time.Second, "Maximum wait time")
			child.Flags().String("file", "", "Wait for this file")
			child.Flags().String("pattern", "", "Required text inside --file")
		}
		cmd.AddCommand(child)
	}
	return cmd
}

func blockDescription(name string) string {
	switch name {
	case "json-peek":
		return "Show keys and types from one JSON file"
	case "jsonl-stats":
		return "Count JSONL rows and keys"
	case "test-summary":
		return "Summarize JUnit, pytest JSON, or Jest JSON reports"
	case "first-error":
		return "Show the first error and nearby lines"
	case "grep-defs":
		return "Show source signatures from one file"
	case "http-json":
		return "Fetch and summarize a JSON response"
	case "replace-in-file":
		return "Replace one literal in one file"
	case "wait-for":
		return "Wait for a file pattern or command success"
	default:
		return "Run a built-in script"
	}
}

func runBlock(cmd *cobra.Command, root, name string, args []string, raw bool) error {
	switch name {
	case "json-peek":
		if len(args) < 1 {
			return cli.Usage("json-peek needs a file", "so blocks run json-peek <file> [path]")
		}
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		path := ""
		if len(args) > 1 {
			path = args[1]
		}
		text, err := blocks.JSONPeek(f, path)
		if err != nil {
			return err
		}
		return printBlockResult(cmd, name, text, raw)
	case "jsonl-stats":
		r, closeFn, err := openArgOrStdin(args)
		if err != nil {
			return err
		}
		defer closeFn()
		text, err := blocks.JSONLStats(r)
		if err != nil {
			return err
		}
		return printBlockResult(cmd, name, text, raw)
	case "test-summary":
		body, err := readArgOrStdin(args)
		if err != nil {
			return err
		}
		return printBlockResult(cmd, name, blocks.TestSummary(root, body), raw)
	case "first-error":
		body, err := readArgOrStdin(args)
		if err != nil {
			return err
		}
		return printBlockResult(cmd, name, blocks.FirstError(string(body)), raw)
	case "grep-defs":
		if len(args) < 1 {
			return cli.Usage("grep-defs needs a file", "so blocks run grep-defs <file>")
		}
		body, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		text, note := blocks.GrepDefs(root, body)
		if !raw {
			out().Note(note)
		}
		return printBlockResult(cmd, name, text, raw)
	case "http-json":
		if len(args) < 1 {
			return cli.Usage("http-json needs a url", "so blocks run http-json <url>")
		}
		text, err := blocks.HTTPJSON(root, args[0])
		if err != nil {
			return err
		}
		return printBlockResult(cmd, name, text, raw)
	case "replace-in-file":
		if len(args) < 3 {
			return cli.Usage("replace-in-file needs file, old, and new", "so blocks run replace-in-file <file> <old> <new>")
		}
		changed, err := blocks.ReplaceInFile(args[0], args[1], args[2])
		if err != nil {
			return err
		}
		if !changed {
			return printBlockResult(cmd, name, "already_applied "+args[0], raw)
		}
		return printBlockResult(cmd, name, "updated "+args[0], raw)
	case "wait-for":
		if err := runWait(cmd, args); err != nil {
			return err
		}
		return printBlockResult(cmd, name, "ready", raw)
	default:
		return cli.Usage("unknown script "+name, "so blocks run json-peek|jsonl-stats|test-summary|first-error|wait-for|grep-defs|http-json|replace-in-file")
	}
}

func printBlockResult(cmd *cobra.Command, name, content string, raw bool) error {
	if raw {
		_, err := fmt.Fprint(cmd.OutOrStdout(), content)
		return err
	}
	o := out()
	if content == "" {
		data := map[string]any{"block": name, "count": 0, "content": "none"}
		o.Next("so blocks run " + name + " --help")
		return o.HumanOrJSON("block_result", func() { fmt.Fprint(o.W, cli.Toon(data, o.Flags.Full)) }, data)
	}
	data := map[string]any{"block": name, "content": content}
	o.Next("so blocks run " + name + " --help")
	// A block may return an unrecognized input unchanged. Keep its evidence
	// visible; a clipped first screen can omit every failure at the end.
	return o.HumanOrJSON("block_result", func() { fmt.Fprint(o.W, cli.Toon(data, true)) }, data)
}

func runWait(cmd *cobra.Command, args []string) error {
	timeout, _ := cmd.Flags().GetDuration("timeout")
	file, _ := cmd.Flags().GetString("file")
	pattern, _ := cmd.Flags().GetString("pattern")
	if timeout <= 0 {
		return cli.Usage("wait-for --timeout must be positive", "so blocks run wait-for --timeout 30s --file path --pattern text")
	}
	if file != "" {
		if len(args) != 0 {
			return cli.Usage("wait-for accepts either --file or a command", "so blocks run wait-for --file path --pattern text")
		}
		return blocks.WaitFor(timeout, func() (bool, error) {
			return blocks.FileContains(file, pattern)
		})
	}
	if pattern != "" {
		return cli.Usage("wait-for --pattern requires --file", "so blocks run wait-for --file path --pattern text")
	}
	if len(args) == 0 {
		return cli.Usage("wait-for needs a file or a command", "so blocks run wait-for --timeout 30s -- <command>")
	}
	return blocks.WaitFor(timeout, func() (bool, error) {
		c := exec.Command(args[0], args[1:]...)
		c.Stdout = io.Discard
		c.Stderr = io.Discard
		err := c.Run()
		if err == nil {
			return true, nil
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return false, nil
		}
		return false, err
	})
}

func openArgOrStdin(args []string) (io.ReadCloser, func(), error) {
	if len(args) == 0 {
		if stdinIsTerminal() {
			return nil, nil, cli.Usage("block needs a file or stdin", "so blocks run --help")
		}
		return io.NopCloser(os.Stdin), func() {}, nil
	}
	f, err := os.Open(args[0])
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func readArgOrStdin(args []string) ([]byte, error) {
	if len(args) == 0 {
		if stdinIsTerminal() {
			return nil, cli.Usage("block needs a file or stdin", "so blocks run --help")
		}
		return readLimited(os.Stdin)
	}
	body, err := os.ReadFile(args[0])
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > compact.MaxInput {
		return nil, compact.ErrTooLarge
	}
	return body, nil
}
