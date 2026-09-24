package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/vendors"
)

func installPromptInput() (io.Reader, func()) {
	if writerIsTerminal(os.Stdin) {
		return os.Stdin, func() {}
	}
	name := "/dev/tty"
	if runtime.GOOS == "windows" {
		name = "CONIN$"
	}
	tty, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, func() {}
	}
	return tty, func() { _ = tty.Close() }
}

func printInstallBanner(w io.Writer) {
	if os.Getenv("SUPEROPEN_INSTALLER") != "" {
		return
	}
	bold, reset := "", ""
	if writerIsTerminal(w) {
		bold, reset = "\033[1m", "\033[0m"
	}
	fmt.Fprintf(w, "\n%s", bold)
	fmt.Fprintln(w, "  ____  _   _ ____  _____ ____   ___  ____  _____ _   _ ")
	fmt.Fprintln(w, " / ___|| | | |  _ \\| ____|  _ \\ / _ \\|  _ \\| ____| \\ | |")
	fmt.Fprintln(w, " \\___ \\| | | | |_) |  _| | |_) | | | | |_) |  _| |  \\| |")
	fmt.Fprintln(w, "  ___) | |_| |  __/| |___|  _ <| |_| |  __/| |___| |\\  |")
	fmt.Fprintln(w, " |____/ \\___/|_|   |_____|_| \\_\\\\___/|_|   |_____|_| \\_|")
	fmt.Fprintf(w, "%s\n", reset)
}

func writerIsTerminal(w io.Writer) bool {
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

// promptInstallVendors asks which harnesses to wire. Empty input and "all"
// install every harness in vendors.All. Later agents show up here automatically.
func promptInstallVendors(in io.Reader, out io.Writer) ([]string, error) {
	all := vendors.All()
	fmt.Fprintln(out, "  Which agents should Superopen install?")
	fmt.Fprintln(out, "  Add another later with: so install --vendor <id>")
	fmt.Fprintln(out)
	for i, spec := range all {
		fmt.Fprintf(out, "  %2d  %s\n", i+1, spec.Label)
	}
	fmt.Fprintln(out)
	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "  Press enter to install all, or numbers separated by spaces: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "all") {
			return vendorIDs(all), nil
		}
		ids, bad := parseVendorChoice(line, all)
		if bad != "" {
			fmt.Fprintf(out, "  Unknown choice %q. Use numbers from the list, or all.\n", bad)
			continue
		}
		return ids, nil
	}
}

func vendorIDs(all []vendors.Spec) []string {
	ids := make([]string, len(all))
	for i, spec := range all {
		ids[i] = spec.ID
	}
	return ids
}

func parseVendorChoice(line string, all []vendors.Spec) ([]string, string) {
	fields := strings.Fields(strings.ReplaceAll(line, ",", " "))
	seen := map[string]bool{}
	var ids []string
	for _, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 1 || n > len(all) {
			return nil, field
		}
		id := all[n-1].ID
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, ""
}
