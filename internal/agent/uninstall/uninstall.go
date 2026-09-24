// Package uninstall implements per-vendor hook teardown for `so uninstall --vendor=...`.
//
// Inverse of `so install`: removes the per-vendor host plugin manifests
// that install writes, optionally deregisters the plugin from the vendor's
// own CLI (Claude Code's `claude plugin uninstall`, Codex's `codex plugin
// remove`), and with `--purge` also drops shared config and session-state
// cache.
//
// We separate `--vendor` cleanup from `--purge` so the common
// "I want to stop Cursor from being tracked but keep my Claude Code
// telemetry" case is a single command and doesn't blow away shared
// local config under ~/.config/superopen.
package uninstall

import (
	"fmt"
	"io"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/skills"
	"github.com/ishanjainn/superopen/internal/agent/steer"
	"github.com/ishanjainn/superopen/internal/agent/vendors"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/spf13/cobra"
)

// RemoveAll uninstalls every coding-agent vendor hook and optionally purges shared state.
// Used by `so uninstall` so hook teardown stays in one place. keepData skips
// deleting per-repo .so directories (project index and other shared state still go).
func RemoveAll(purge, keepData, dryRun bool, stdout, stderr io.Writer) (removed []string, errs []string) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	var hooked []string
	spinWork(stdout, "Removing hooks", func() {
		targets, _ := vendorsFromArg("all")
		for _, v := range targets {
			r, e := uninstallVendor(v, dryRun)
			removed = append(removed, r...)
			errs = append(errs, e...)
			if len(r) > 0 {
				hooked = append(hooked, vendors.Label(v))
			}
			for _, msg := range e {
				fmt.Fprintf(stderr, "uninstall %s: %s\n", v, msg)
			}
		}
	})
	tick(stdout, "Hooks", strings.Join(hooked, ", "))
	if !dryRun {
		spinWork(stdout, "Removing the skill", func() {
			for _, path := range skills.RemoveAll() {
				removed = append(removed, path)
			}
			for _, path := range steer.RemoveAll() {
				removed = append(removed, path)
			}
		})
		tick(stdout, "Skill", "")
	}
	if purge {
		spinWork(stdout, "Removing local data", func() {
			r, e := purgeShared(dryRun, keepData)
			removed = append(removed, r...)
			errs = append(errs, e...)
			for _, msg := range e {
				fmt.Fprintf(stderr, "uninstall --purge: %s\n", msg)
			}
		})
		tick(stdout, "Data", "")
	}
	return removed, errs
}

// Run removes hooks for --vendor. Used by `so uninstall --vendor=…`.
func Run(cmd *cobra.Command, vendor string, purge, dryRun bool) error {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	if vendor == "" {
		return cli.Usage("--vendor is required", "so uninstall --vendor=all")
	}

	targets, err := vendorsFromArg(vendor)
	if err != nil {
		return cli.Usage(err.Error(), "so uninstall --vendor=all")
	}

	out := cli.NewFromCmd(cmd)
	rows := make([]map[string]any, 0, len(targets))
	pathRows := make([]map[string]any, 0)
	for _, v := range targets {
		removed, vendErrs := uninstallVendor(v, dryRun)
		rows = append(rows, map[string]any{
			"vendor":  v,
			"files":   len(removed),
			"dry_run": dryRun,
		})
		for _, p := range removed {
			pathRows = append(pathRows, map[string]any{"vendor": v, "path": p})
		}
		for _, e := range vendErrs {
			fmt.Fprintf(cmd.ErrOrStderr(), "so uninstall %s: %s\n", v, e)
		}
	}

	if purge {
		removed, purgeErrs := purgeShared(dryRun, false)
		rows = append(rows, map[string]any{
			"vendor":  "purge",
			"files":   len(removed),
			"dry_run": dryRun,
		})
		for _, p := range removed {
			pathRows = append(pathRows, map[string]any{"vendor": "purge", "path": p})
		}
		for _, e := range purgeErrs {
			fmt.Fprintf(cmd.ErrOrStderr(), "so uninstall --purge: %s\n", e)
		}
	}

	out.Next("so uninstall --vendor=cursor", "so install")
	if out.Flags.Full {
		out.Rows("files", []string{"vendor", "path"}, pathRows)
		return nil
	}
	out.Rows("uninstall", []string{"vendor", "files", "dry_run"}, rows)
	return nil
}

// vendorsFromArg mirrors the install package's accepted vendor IDs so
// the two commands stay in lock-step. We intentionally duplicate the
// switch rather than import install/ to keep uninstall buildable on
// its own (and the surface is small).
func vendorsFromArg(arg string) ([]string, error) {
	switch arg {
	case "all":
		return append([]string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"}, vendors.IDs()...), nil
	case "claude-code", "cc":
		return []string{"claude-code"}, nil
	case "cursor":
		return []string{"cursor"}, nil
	case "codex":
		return []string{"codex"}, nil
	case "gemini", "gemini-cli":
		return []string{"gemini"}, nil
	case "opencode", "open-code":
		return []string{"opencode"}, nil
	case "copilot", "copilot-cli":
		return []string{"copilot-cli"}, nil
	case "pi":
		return []string{"pi"}, nil
	default:
		if spec, ok := vendors.ByID(arg); ok {
			return []string{spec.ID}, nil
		}
		return nil, fmt.Errorf("unknown --vendor %q", arg)
	}
}
