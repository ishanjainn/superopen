package buildpool

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTryAcquireCapsAtTwo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, ".config"))
	t.Setenv(slotEnv, "2")

	first, err := TryAcquire("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := TryAcquire("/repo/b")
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	if _, err := TryAcquire("/repo/c"); !errors.Is(err, ErrFull) {
		t.Fatalf("expected ErrFull, got %v", err)
	}
	if !Full() {
		t.Fatal("expected pool full")
	}
}

func TestDeadPIDReclaim(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, ".config"))
	t.Setenv(slotEnv, "1")

	unlock, err := TryAcquire("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := slotDir()
	if err != nil {
		t.Fatal(err)
	}
	writeMeta(filepath.Join(dir, "0.lock"), 1, "/repo/gone")
	unlock()

	next, err := TryAcquire("/repo/b")
	if err != nil {
		t.Fatal(err)
	}
	next()
}

func TestDetachSkipWhenFullIsFast(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, ".config"))
	t.Setenv(slotEnv, "2")

	a, _ := TryAcquire("/a")
	defer a()
	b, _ := TryAcquire("/b")
	defer b()

	started := time.Now()
	if !Full() {
		t.Fatal("expected full")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("Full() blocked for %s", time.Since(started))
	}
}

func TestConcurrentMaxTwoRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, ".config"))
	t.Setenv(slotEnv, "2")

	var mu sync.Mutex
	running := 0
	max := 0
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := TryAcquire("/r")
			if err != nil {
				return
			}
			defer unlock()
			mu.Lock()
			running++
			if running > max {
				max = running
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if max > 2 {
		t.Fatalf("max concurrent = %d", max)
	}
}
