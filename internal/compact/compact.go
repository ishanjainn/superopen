// Package compact shrinks noisy tool bytes and keeps the exact original in
// the repository recovery store. Token figures from this package are inferred
// local estimates. They are never a provider bill and never verified savings.
package compact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/third_party/caveman/engine"
	"github.com/ishanjainn/superopen/third_party/caveman/engine/ccr"
)

const (
	// MaxInput is the CLI cap. Above it the command fails and does not truncate.
	MaxInput = 64 << 20
	// minBytes skips tiny payloads. The transform would not pay for itself,
	// and the hook has a few seconds for the whole event.
	minBytes = 2048
	// Basis is the only label this package emits for a token count.
	Basis = engine.BasisInferred
)

// ErrTooLarge is the CLI failure for an input above MaxInput.
var ErrTooLarge = errors.New("cave_input_too_large")

// Outcome is one compress call. PassedThrough means Output is the input.
type Outcome struct {
	Output        []byte
	Handle        string
	ContentType   string
	TokensBefore  int
	TokensAfter   int
	Basis         string
	PassedThrough bool
}

// StorePath is the SQLite file under .so/db/. That directory is gitignored.
func StorePath(root string) string {
	return filepath.Join(paths.Resolve(root).DBDir, "ccr.db")
}

// ProtectCommand reports commands whose stdout must stay byte-for-byte.
// Graph, memory, and session listings are already compact.
func ProtectCommand(cmd string) bool {
	fields := strings.Fields(cmd)
	for i, f := range fields {
		if !paths.IsSoBinary(strings.Trim(f, `"'`)) {
			continue
		}
		if i+1 >= len(fields) {
			return false
		}
		switch strings.ToLower(strings.Trim(fields[i+1], `"'`)) {
		case "graph", "memory", "sessions":
			return true
		}
	}
	return false
}

// Compress transforms input when the result is smaller and the original is
// stored. Payloads under a few kilobytes pass through so a hook does not
// spend its budget on them. Any failure, including a full store, returns the
// original bytes.
func Compress(root string, input []byte, contentType string) Outcome {
	return compress(root, input, contentType, minBytes)
}

// CompressAny is Compress without the small-payload floor. Callers that
// already decided the bytes are worth transforming use it.
func CompressAny(root string, input []byte, contentType string) Outcome {
	return compress(root, input, contentType, 1)
}

func compress(root string, input []byte, contentType string, floor int) Outcome {
	out := Outcome{
		Output:        append([]byte(nil), input...),
		ContentType:   contentType,
		Basis:         Basis,
		PassedThrough: true,
	}
	if len(input) < floor || len(input) > MaxInput {
		return out
	}
	store, err := openStore(root)
	if err != nil || store == nil {
		return out
	}
	defer store.Close()
	eng := engine.New(store, nil)
	res, err := eng.Compress(input, engine.Options{Mode: engine.ModeCompress, Type: contentType})
	if err != nil || res.RecoveryHandle == "" || len(res.Output) >= len(input) {
		out.TokensBefore = res.TokensBefore
		out.TokensAfter = res.TokensBefore
		return out
	}
	out.Output = res.Output
	out.Handle = res.RecoveryHandle
	out.ContentType = res.ContentType
	out.TokensBefore = res.TokensBefore
	out.TokensAfter = res.TokensAfter
	out.Basis = Basis
	out.PassedThrough = false
	return out
}

// Retrieve returns the exact original bytes for a handle. A bad handle fails.
func Retrieve(root, handle string) ([]byte, error) {
	store, err := openStore(root)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	eng := engine.New(store, nil)
	return eng.Retrieve(strings.TrimSpace(handle))
}

func openStore(root string) (*ccr.Store, error) {
	path := StorePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return ccr.Open(path)
}
