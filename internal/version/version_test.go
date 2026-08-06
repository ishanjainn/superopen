package version_test

import (
	"testing"

	"github.com/superopen/so/internal/version"
)

func TestDisplaySemver(t *testing.T) {
	prev := version.Version
	t.Cleanup(func() { version.Version = prev })

	version.Version = "0.1.0"
	if got := version.Display(); got != "0.1.0" {
		t.Fatalf("Display() = %q, want 0.1.0", got)
	}
	version.Version = "v1.2.3"
	if got := version.Display(); got != "1.2.3" {
		t.Fatalf("Display() = %q, want 1.2.3", got)
	}
}
