//go:build cgo

package cpreproc

import (
	"strings"
	"testing"
)

func TestWithMapExpandsObjectMacro(t *testing.T) {
	src := []byte("#define LOG(msg) printk(msg)\nvoid f(void) { LOG(\"x\"); }\n")
	got := WithMap(src, "log.c", false)
	if got == nil {
		t.Fatal("expected expansion")
	}
	if !strings.Contains(got.Source, "printk") {
		t.Fatalf("expanded source missing printk: %q", got.Source)
	}
	if strings.Contains(got.Source, "LOG(") {
		t.Fatalf("macro call survived expansion: %q", got.Source)
	}
}

func TestWithMapSkipsFilesWithoutDirectives(t *testing.T) {
	if got := WithMap([]byte("void f(void) { printk(\"x\"); }\n"), "log.c", false); got != nil {
		t.Fatalf("unexpected expansion: %#v", got)
	}
}
