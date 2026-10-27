package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
		Short: "Rank shell commands retyped in local session history",
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
				fmt.Fprintln(cmd.OutOrStdout(), "0 retyped scripts")
				fmt.Fprintln(cmd.OutOrStdout(), blocks.GrepDefsNote)
				return nil
			}
			rows := make([]map[string]any, 0, len(hits))
			for _, h := range hits {
				rows = append(rows, map[string]any{"count": h.Count, "command": h.Command})
			}
			out().Rows("scripts", []string{"count", "command"}, rows)
			fmt.Fprintln(cmd.OutOrStdout(), blocks.GrepDefsNote)
			return nil
		},
	}
}

func blocksInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Stage a script index. Does not edit AGENTS.md",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			path, err := blocks.Init(root)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "staged %s\n", path)
			fmt.Fprintln(cmd.OutOrStdout(), "so blocks apply")
			return nil
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
			fmt.Fprintln(cmd.OutOrStdout(), "applied AGENTS.md")
			return nil
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
			fmt.Fprintln(cmd.OutOrStdout(), "declined")
			return nil
		},
	}
}

func blocksRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run one built-in script",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			return runBlock(cmd, root, args[0], args[1:])
		},
	}
	return cmd
}

func runBlock(cmd *cobra.Command, root, name string, args []string) error {
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
		fmt.Fprint(cmd.OutOrStdout(), text)
		return nil
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
		fmt.Fprint(cmd.OutOrStdout(), text)
		return nil
	case "test-summary":
		body, err := readArgOrStdin(args)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), blocks.TestSummary(root, body))
		return nil
	case "first-error":
		body, err := readArgOrStdin(args)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), blocks.FirstError(string(body)))
		return nil
	case "grep-defs":
		if len(args) < 1 {
			return cli.Usage("grep-defs needs a file", "so blocks run grep-defs <file>")
		}
		body, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		text, note := blocks.GrepDefs(root, body)
		fmt.Fprintln(os.Stderr, note)
		fmt.Fprint(cmd.OutOrStdout(), text)
		return nil
	case "http-json":
		if len(args) < 1 {
			return cli.Usage("http-json needs a url", "so blocks run http-json <url>")
		}
		text, err := blocks.HTTPJSON(root, args[0])
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), text)
		return nil
	case "replace-in-file":
		if len(args) < 3 {
			return cli.Usage("replace-in-file needs file, old, and new", "so blocks run replace-in-file <file> <old> <new>")
		}
		return blocks.ReplaceInFile(args[0], args[1], args[2])
	case "wait-for":
		return runWait(args)
	default:
		return cli.Usage("unknown script "+name, "so blocks run json-peek|jsonl-stats|test-summary|first-error|wait-for|grep-defs|http-json|replace-in-file")
	}
}

func runWait(args []string) error {
	timeout := 30 * time.Second
	file := ""
	pattern := ""
	var cmdArgs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--timeout":
			if i+1 >= len(args) {
				return cli.Usage("wait-for --timeout needs a duration", "so blocks run wait-for --timeout 30s --file path --pattern text")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				if n, nerr := strconv.Atoi(args[i+1]); nerr == nil {
					d = time.Duration(n) * time.Second
				} else {
					return err
				}
			}
			timeout = d
			i++
		case "--file":
			if i+1 >= len(args) {
				return cli.Usage("wait-for --file needs a path", "so blocks run wait-for --file path --pattern text")
			}
			file = args[i+1]
			i++
		case "--pattern":
			if i+1 >= len(args) {
				return cli.Usage("wait-for --pattern needs text", "so blocks run wait-for --file path --pattern text")
			}
			pattern = args[i+1]
			i++
		case "--":
			cmdArgs = args[i+1:]
			i = len(args)
		default:
			if strings.HasPrefix(args[i], "--") {
				return cli.Usage("unknown wait-for flag", "so blocks run wait-for --timeout 30s --file path --pattern text")
			}
			cmdArgs = args[i:]
			i = len(args)
		}
	}
	if file != "" {
		return blocks.WaitFor(timeout, func() (bool, error) {
			return blocks.FileContains(file, pattern)
		})
	}
	if len(cmdArgs) == 0 {
		return cli.Usage("wait-for needs a file or a command", "so blocks run wait-for --timeout 30s -- <command>")
	}
	return blocks.WaitFor(timeout, func() (bool, error) {
		c := exec.Command(cmdArgs[0], cmdArgs[1:]...)
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
