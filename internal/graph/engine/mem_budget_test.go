package engine

import "testing"

func TestResolveMemoryBudgetFractions(t *testing.T) {
	got := resolveMemoryBudget(16*ramBytesPerGB, "")
	if got != 4*ramBytesPerGB {
		t.Fatalf("16GiB budget=%d want 4GiB", got)
	}
	var total32 uint64 = 32 * ramBytesPerGB
	got = resolveMemoryBudget(total32, "")
	want32 := uint64(float64(total32) * 0.35)
	if got != want32 {
		t.Fatalf("32GiB budget=%d want %d", got, want32)
	}
	got = resolveMemoryBudget(64*ramBytesPerGB, "")
	if got != 32*ramBytesPerGB {
		t.Fatalf("64GiB budget=%d want 32GiB", got)
	}
	got = resolveMemoryBudget(16*ramBytesPerGB, "2048")
	if got != 2048*1024*1024 {
		t.Fatalf("override=%d", got)
	}
}
