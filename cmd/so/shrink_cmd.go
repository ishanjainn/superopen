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
	cmd := &cobra.Command{
		Use:   "shrink [-- command]",
		Short: "Shrink noisy bytes and keep the original locally",
		Long: `Read stdin, or the stdout of the command after --, and print a smaller form
when one exists. The exact original is stored under .so/db/. Token counts are
inferred local estimates, not a provider bill.

Input above 64 MiB fails with cave_input_too_large and is not truncated.
so graph, so memory, and so sessions output is printed unchanged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
					if _, err := os.Stdout.Write(input); err != nil {
						return err
					}
					return runErr
				}
				input, runErr = runCapture(args)
			} else {
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
			if _, err := os.Stdout.Write(out.Output); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "inferred tokens_before=%d tokens_after=%d basis=%s\n", out.TokensBefore, out.TokensAfter, compact.Basis)
			if out.Handle != "" {
				fmt.Fprintf(os.Stderr, "recover so retrieve %s\n", out.Handle)
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&contentType, "type", "", "Force a content type. Empty detects. toon, a11y, tool schema, and repetition stay explicit")
	return cmd
}

func cmdRetrieve() *cobra.Command {
	return &cobra.Command{
		Use:   "retrieve <handle>",
		Short: "Print the exact original bytes for a recovery handle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			_, err = os.Stdout.Write(body)
			return err
		},
	}
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
