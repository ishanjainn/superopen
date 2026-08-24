// Package cli implements shared CLI output helpers for Superopen.
// Principles: compact defaults, --json / --full escape hatches, definitive empty
// states, structured errors, next-step hints, content-first root command.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// Exit codes (stable for agents).
const (
	ExitOK                   = 0
	ExitFail                 = 1
	ExitUsage                = 2
	ExitNotFound             = 3
	ExitContinuationRequired = 4
)

// DefaultTruncate is the default rune budget for string fields unless --full.
const DefaultTruncate = 80

// Flags holds persistent CLI presentation options.
type Flags struct {
	JSON bool
	Full bool
	Root string
}

// Out is the presentation context for one command invocation.
type Out struct {
	Flags Flags
	W     io.Writer
	ErrW  io.Writer
	hints []string
}

// Bind attaches --json / --full / --root to the root command (persistent).
func Bind(root *cobra.Command, f *Flags) {
	root.PersistentFlags().BoolVar(&f.JSON, "json", false, "Machine-readable JSON (AXI)")
	root.PersistentFlags().BoolVar(&f.Full, "full", false, "Do not truncate fields (AXI)")
	root.PersistentFlags().StringVar(&f.Root, "root", "", "Repository root (overrides SUPEROPEN_ROOT and walk-up)")
	// Env override for agent shells
	root.PersistentPreRunE = chainPreRun(root.PersistentPreRunE, func(cmd *cobra.Command, args []string) error {
		if envTruthy(os.Getenv("SO_JSON")) || envTruthy(os.Getenv("SUPEROPEN_JSON")) {
			f.JSON = true
		}
		if envTruthy(os.Getenv("SO_FULL")) || envTruthy(os.Getenv("SUPEROPEN_FULL")) {
			f.Full = true
		}
		return nil
	})
}

func chainPreRun(prev, next func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(cmd, args); err != nil {
				return err
			}
		}
		if next != nil {
			return next(cmd, args)
		}
		return nil
	}
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "json":
		return true
	default:
		return false
	}
}

// New builds an Out from flags (stdout/stderr).
func New(f Flags) *Out {
	return &Out{Flags: f, W: os.Stdout, ErrW: os.Stderr}
}

// FromCmd reads persistent flags from the command tree.
func FromCmd(cmd *cobra.Command, f *Flags) *Out {
	return New(*f)
}

// FlagsFromCmd reads persistent --json / --full / --root from a cobra command.
func FlagsFromCmd(cmd *cobra.Command) Flags {
	return Flags{
		JSON: flagBool(cmd, "json"),
		Full: flagBool(cmd, "full"),
		Root: flagString(cmd, "root"),
	}
}

func flagBool(cmd *cobra.Command, name string) bool {
	raw := flagValue(cmd, name)
	if raw == "" {
		return false
	}
	v, _ := strconv.ParseBool(raw)
	return v
}

func flagString(cmd *cobra.Command, name string) string {
	return flagValue(cmd, name)
}

func flagValue(cmd *cobra.Command, name string) string {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.Flags().Lookup(name); f != nil {
			return f.Value.String()
		}
		if f := c.PersistentFlags().Lookup(name); f != nil {
			return f.Value.String()
		}
	}
	return ""
}

// NewFromCmd builds an Out bound to the command's stdout/stderr.
func NewFromCmd(cmd *cobra.Command) *Out {
	o := New(FlagsFromCmd(cmd))
	if cmd != nil {
		o.W = cmd.OutOrStdout()
		o.ErrW = cmd.ErrOrStderr()
	}
	return o
}

// BinPath is the current executable with $HOME rendered as ~.
func BinPath() string {
	exe, err := os.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return "so"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(exe, home) {
		return "~" + strings.TrimPrefix(exe, home)
	}
	return exe
}

// UnknownCommand is an AXI usage error for a bad group subcommand.
func UnknownCommand(group, name string) error {
	name = strings.TrimSpace(name)
	hint := "so " + strings.TrimSpace(group) + " --help"
	if name == "" {
		return Usage("command required", hint)
	}
	return Usage(fmt.Sprintf("unknown command %q", name), hint)
}

// Truncate shortens s unless --full.
func (o *Out) Truncate(s string, limit int) string {
	if o.Flags.Full {
		return s
	}
	if limit <= 0 {
		limit = DefaultTruncate
	}
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit]) + "…"
}

// TruncateHint truncates and appends an AXI size hint when clipped.
func (o *Out) TruncateHint(s string, limit int) string {
	if o.Flags.Full {
		return s
	}
	if limit <= 0 {
		limit = DefaultTruncate
	}
	n := utf8.RuneCountInString(s)
	if n <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit]) + fmt.Sprintf("… (truncated, %d chars — use --full)", n)
}

// Next queues a next-step hint (printed after primary output in text mode;
// included as "next" in JSON envelopes).
func (o *Out) Next(hints ...string) {
	for _, h := range hints {
		h = strings.TrimSpace(h)
		if h != "" {
			o.hints = append(o.hints, h)
		}
	}
}

func (o *Out) flushHints() {
	if o.Flags.JSON || len(o.hints) == 0 {
		return
	}
	fmt.Fprintf(o.W, "help[%d]:\n", len(o.hints))
	for _, h := range o.hints {
		fmt.Fprintf(o.W, "  %s\n", h)
	}
}

// Empty prints a definitive empty state.
func (o *Out) Empty(kind string) {
	if o.Flags.JSON {
		_ = json.NewEncoder(o.W).Encode(map[string]any{
			"ok":    true,
			"kind":  kind,
			"count": 0,
			"items": []any{},
			"next":  o.hints,
		})
		return
	}
	fmt.Fprintf(o.W, "0 %s\n", kind)
	o.flushHints()
}

// Rows prints a compact table (text) or JSON list.
// Each row is a map; cols defines column order and keys.
func (o *Out) Rows(kind string, cols []string, rows []map[string]any) {
	if o.Flags.JSON {
		_ = json.NewEncoder(o.W).Encode(map[string]any{
			"ok":    true,
			"kind":  kind,
			"count": len(rows),
			"items": rows,
			"next":  o.hints,
		})
		return
	}
	if len(rows) == 0 {
		o.Empty(kind)
		return
	}
	fmt.Fprintf(o.W, "%s[%d]{%s}:\n", kind, len(rows), strings.Join(cols, ","))
	for _, row := range rows {
		parts := make([]string, 0, len(cols))
		for _, c := range cols {
			s := fmt.Sprint(row[c])
			if c == "title" || c == "detail" || c == "preview" || c == "snippet" || c == "text" {
				orig := utf8.RuneCountInString(s)
				s = o.Truncate(s, DefaultTruncate)
				if !o.Flags.Full && orig > DefaultTruncate {
					s = fmt.Sprintf("%s (truncated, %d chars — use --full)", s, orig)
				}
			}
			parts = append(parts, toonField(s))
		}
		fmt.Fprintf(o.W, "  %s\n", strings.Join(parts, ","))
	}
	fmt.Fprintf(o.W, "count: %d of %d\n", len(rows), len(rows))
	o.flushHints()
}

func toonField(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if s == "" || strings.ContainsAny(s, ",\"") {
		s = strings.ReplaceAll(s, `"`, `""`)
		return `"` + s + `"`
	}
	return s
}

// Home prints an AXI content-first dashboard (bin, description, body, help[]).
func (o *Out) Home(description string, lines []string) {
	if o.Flags.JSON {
		payload := map[string]any{
			"ok":          true,
			"kind":        "home",
			"bin":         BinPath(),
			"description": description,
			"lines":       lines,
			"next":        o.hints,
		}
		_ = json.NewEncoder(o.W).Encode(payload)
		return
	}
	fmt.Fprintf(o.W, "bin: %s\n", BinPath())
	fmt.Fprintf(o.W, "description: %s\n", description)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			fmt.Fprintln(o.W, line)
		}
	}
	o.flushHints()
}

// Object prints one JSON object or key=value lines.
func (o *Out) Object(kind string, v any) {
	if o.Flags.JSON {
		_ = json.NewEncoder(o.W).Encode(map[string]any{
			"ok":   true,
			"kind": kind,
			"data": v,
			"next": o.hints,
		})
		return
	}
	switch m := v.(type) {
	case map[string]any:
		for k, val := range m {
			fmt.Fprintf(o.W, "%s: %v\n", k, val)
		}
	default:
		enc := json.NewEncoder(o.W)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
	}
	o.flushHints()
}

// OK prints a small success payload.
func (o *Out) OK(kind string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	if o.Flags.JSON {
		payload := map[string]any{"ok": true, "kind": kind, "next": o.hints}
		for k, v := range fields {
			payload[k] = v
		}
		_ = json.NewEncoder(o.W).Encode(payload)
		return
	}
	parts := []string{kind}
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	fmt.Fprintln(o.W, strings.Join(parts, "  "))
	o.flushHints()
}

// Error is a structured CLI error with an exit code.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"error"`
	Hint    string `json:"hint,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func Fail(code int, msg string, hint string) error {
	return &Error{Code: code, Message: msg, Hint: hint}
}

// WriteError emits a structured AXI error on stdout (stderr is debug only).
func (o *Out) WriteError(err error) {
	if err == nil {
		return
	}
	var ae *Error
	if !errors.As(err, &ae) {
		ae = &Error{Code: ExitFail, Message: err.Error()}
	}
	if o.Flags.JSON {
		_ = json.NewEncoder(o.W).Encode(map[string]any{
			"ok":    false,
			"code":  ae.Code,
			"error": ae.Message,
			"hint":  ae.Hint,
		})
		return
	}
	fmt.Fprintf(o.W, "error: %s\n", ae.Message)
	if ae.Hint != "" {
		fmt.Fprintf(o.W, "hint: %s\n", ae.Hint)
	}
}

// ExitCode extracts an AXI exit code from err.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ExitFail
}

// HumanOrJSON runs human() for text mode, or emits a JSON envelope with data.
func (o *Out) HumanOrJSON(kind string, human func(), data any) error {
	if o.Flags.JSON {
		_ = json.NewEncoder(o.W).Encode(map[string]any{
			"ok":   true,
			"kind": kind,
			"data": data,
			"next": o.hints,
		})
		return nil
	}
	if human != nil {
		human()
	}
	o.flushHints()
	return nil
}

// Err wraps a plain error as an AXI Error (ExitFail).
func Err(err error) error {
	if err == nil {
		return nil
	}
	if ae, ok := err.(*Error); ok {
		return ae
	}
	return &Error{Code: ExitFail, Message: err.Error()}
}

// Usage returns an ExitUsage error.
func Usage(msg, hint string) error { return Fail(ExitUsage, msg, hint) }

// NotFound returns an ExitNotFound error.
func NotFound(msg, hint string) error { return Fail(ExitNotFound, msg, hint) }

// Continuation tells a coding-agent caller that the command made durable
// progress and must be resumed through the machine-readable protocol.
func Continuation(msg, hint string) error { return Fail(ExitContinuationRequired, msg, hint) }
