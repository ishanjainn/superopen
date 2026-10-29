package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/compact"
	"github.com/ishanjainn/superopen/third_party/caveman/engine/ccr"
)

func cmdShrink() *cobra.Command {
	var contentType string
	var raw bool
	cmd := &cobra.Command{
		Use:   "shrink [-- command]",
		Short: "Shrink noisy bytes and keep the original locally",
		Long: `Read stdin, or the stdout of the command after --, and show a compact
structured result when one exists. The result shows all content; --raw prints
only the transformed bytes. Compressed originals are stored under .so/db/.

Input above 64 MiB fails with cave_input_too_large and is not truncated.
so graph, so memory, and so sessions output is printed unchanged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if raw && out().Flags.JSON {
				return cli.Usage("--raw and --json cannot be combined", "use --raw for bytes or --json for a structured result")
			}
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			var input []byte
			var runErr error
			if len(args) > 0 {
				if compact.ProtectCommand(strings.Join(args, " ")) {
					input, runErr = runCapture(args)
					if runErr != nil && len(input) == 0 {
						return runErr
					}
					return printShrink(cmd, input, compact.Outcome{Output: input, PassedThrough: true}, raw, runErr)
				}
				input, runErr = runCapture(args)
			} else {
				if stdinIsTerminal() {
					return cli.Usage("shrink needs stdin or a command", "so shrink -- <command>  or pipe bytes into so shrink")
				}
				input, runErr = readLimited(os.Stdin)
			}
			if errors.Is(runErr, compact.ErrTooLarge) {
				return cli.Fail(cli.ExitFail, "cave_input_too_large", "the cap is 64 MiB and the input is not truncated")
			}
			if runErr != nil && len(input) == 0 {
				return runErr
			}
			if int64(len(input)) > compact.MaxInput {
				return cli.Fail(cli.ExitFail, "cave_input_too_large", "the cap is 64 MiB and the input is not truncated")
			}
			out := compact.CompressAny(root, input, contentType)
			return printShrink(cmd, input, out, raw, runErr)
		},
	}
	cmd.Flags().StringVar(&contentType, "type", "", "Force a content type. Empty detects. toon, a11y, tool schema, and repetition stay explicit")
	cmd.Flags().BoolVar(&raw, "raw", false, "Print only the transformed bytes for a pipe")
	return cmd
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func cmdRetrieve() *cobra.Command {
	var raw bool
	cmd := &cobra.Command{
		Use:   "retrieve <handle>",
		Short: "Inspect the original; --raw prints its exact bytes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if raw && out().Flags.JSON {
				return cli.Usage("--raw and --json cannot be combined", "use --raw for bytes or --json for a structured result")
			}
			root := repoRoot()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			body, err := compact.Retrieve(root, args[0])
			if err != nil {
				if errors.Is(err, ccr.ErrNotFound) {
					return cli.Fail(cli.ExitNotFound, "recovery handle not found", "so shrink")
				}
				return err
			}
			if raw {
				_, err = cmd.OutOrStdout().Write(body)
				return err
			}
			result := map[string]any{"handle": args[0], "bytes": len(body), "content": string(body)}
			o := out()
			o.Next("so retrieve --raw " + args[0])
			return o.HumanOrJSON("retrieved", func() { fmt.Fprint(o.W, cli.Toon(result, o.Flags.Full)) }, result)
		},
	}
	cmd.Flags().BoolVar(&raw, "raw", false, "Print the exact original bytes for a pipe")
	return cmd
}

func printShrink(cmd *cobra.Command, input []byte, result compact.Outcome, raw bool, runErr error) error {
	if raw {
		if _, err := cmd.OutOrStdout().Write(result.Output); err != nil {
			return err
		}
		return runErr
	}
	data := map[string]any{
		"bytes_before": len(input),
		"bytes_after":  len(result.Output),
		"content":      string(result.Output),
	}
	if result.ContentType != "" {
		data["content_type"] = result.ContentType
	}
	o := out()
	if result.Handle != "" {
		data["recover"] = "so retrieve --raw " + result.Handle
		o.Next("so retrieve --raw " + result.Handle)
	}
	// A formatter clip can hide failures even when compression passed through.
	// Only the compressor may shorten this output, with a recovery handle.
	if err := o.HumanOrJSON("shrink", func() { fmt.Fprint(o.W, cli.Toon(data, true)) }, data); err != nil {
		return err
	}
	return runErr
}

func readLimited(r io.Reader) ([]byte, error) {
	buf, err := io.ReadAll(io.LimitReader(r, compact.MaxInput+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > compact.MaxInput {
		return nil, compact.ErrTooLarge
	}
	return buf, nil
}

func runCapture(args []string) ([]byte, error) {
	c := exec.Command(args[0], args[1:]...)
	c.Stderr = os.Stderr
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := c.Start(); err != nil {
		return nil, err
	}
	buf, readErr := readLimited(stdout)
	waitErr := c.Wait()
	if errors.Is(readErr, compact.ErrTooLarge) {
		return nil, compact.ErrTooLarge
	}
	if readErr != nil {
		return buf, readErr
	}
	return buf, waitErr
}
