package compact

import (
	"bytes"
	"strings"
	"testing"
)

func TestProtectCommand(t *testing.T) {
	if !ProtectCommand(`/usr/local/bin/so graph query "x"`) {
		t.Fatal("graph stdout is protected")
	}
	if !ProtectCommand(`so memory recall "x"`) {
		t.Fatal("memory stdout is protected")
	}
	if !ProtectCommand(`so.exe sessions list`) {
		t.Fatal("sessions stdout is protected")
	}
	if ProtectCommand(`so shrink -- pnpm test`) {
		t.Fatal("shrink is not a protected listing")
	}
	if ProtectCommand(`go test ./...`) {
		t.Fatal("go test is not protected")
	}
}

func TestCompressStoresAndRetrieves(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	b.WriteString("[\n")
	for i := 0; i < 400; i++ {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString(`{"id":`)
		b.WriteString(strings.Repeat("1", 3))
		b.WriteString(`,"name":"row","noise":"`)
		b.WriteString(strings.Repeat("x", 40))
		b.WriteString(`"}`)
	}
	b.WriteString("\n]\n")
	input := []byte(b.String())
	if len(input) < minBytes {
		t.Fatalf("fixture too small: %d", len(input))
	}
	out := Compress(root, input, "")
	if out.PassedThrough || out.Handle == "" {
		t.Fatalf("expected a stored transform, got passthrough %v handle %q before %d after %d", out.PassedThrough, out.Handle, out.TokensBefore, out.TokensAfter)
	}
	if out.Basis != Basis {
		t.Fatalf("basis %q", out.Basis)
	}
	if len(out.Output) >= len(input) {
		t.Fatalf("output not smaller: %d >= %d", len(out.Output), len(input))
	}
	got, err := Retrieve(root, out.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatal("retrieve did not return the exact original")
	}
	if _, err := Retrieve(root, "ccr_deadbeef"); err == nil {
		t.Fatal("bad handle must fail")
	}
}

func TestSmallInputPassesThrough(t *testing.T) {
	out := Compress(t.TempDir(), []byte(`{"a":1}`), "json")
	if !out.PassedThrough || out.Handle != "" {
		t.Fatalf("small input must pass through: %+v", out)
	}
}
