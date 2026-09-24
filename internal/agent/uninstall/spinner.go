package uninstall

import (
	"fmt"
	"io"
	"os"
	"time"
)

// spinWork shows a spinner while work runs. tick prints the finished line.
// Not a terminal: no animation, same tick.
func spinWork(w io.Writer, label string, work func()) {
	s := startSpin(w, label)
	work()
	s.stop()
}

func tick(w io.Writer, label, detail string) {
	green, reset := "", ""
	if isTerm(w) {
		green, reset = "\033[32m", "\033[0m"
	}
	if detail == "" {
		fmt.Fprintf(w, "  %s✓%s  %s\n", green, reset, label)
		return
	}
	fmt.Fprintf(w, "  %s✓%s  %-14s %s\n", green, reset, label, detail)
}

type spin struct {
	w    io.Writer
	quit chan struct{}
	done chan struct{}
}

func startSpin(w io.Writer, label string) *spin {
	s := &spin{w: w}
	if !isTerm(w) {
		return s
	}
	s.quit = make(chan struct{})
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		i := 0
		pulse := time.NewTicker(80 * time.Millisecond)
		defer pulse.Stop()
		for {
			select {
			case <-s.quit:
				fmt.Fprint(w, "\r\033[2K")
				return
			case <-pulse.C:
				fmt.Fprintf(w, "\r  %c  %s", frames[i%len(frames)], label)
				i++
			}
		}
	}()
	return s
}

func (s *spin) stop() {
	if s == nil || s.quit == nil {
		return
	}
	close(s.quit)
	<-s.done
	s.quit = nil
}

func isTerm(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
