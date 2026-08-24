package harvest

import "strings"

const graphFirstLine = `Superopen: codebase questions → so graph query "<question>" first.`

// SessionStartLine is at most one harvest status line. Empty when nothing is pending.
func SessionStartLine(root string) string {
	store, err := OpenRoot(root)
	if err != nil {
		return ""
	}
	defer store.Close()
	if n := store.OpenCount(); n > 0 {
		return "HARVEST " + itoa(n) + " OPEN — so harvest review"
	}
	if pending := store.PendingSession(); pending != "" {
		return "HARVEST pending session " + pending + " — so harvest scan"
	}
	return ""
}

// AttachSessionStart appends the harvest line after a graph-first index.
// If the index is empty and harvest is pending, emit graph-first + harvest.
func AttachSessionStart(index, root string) string {
	extra := SessionStartLine(root)
	if extra == "" {
		return index
	}
	index = strings.TrimSpace(index)
	if index == "" {
		return graphFirstLine + "\n" + extra
	}
	combined := index + "\n" + extra
	if estTokens(combined) <= 350 {
		return combined
	}
	first, _, _ := strings.Cut(index, "\n")
	return first + "\n" + extra
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [16]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}

func estTokens(s string) int {
	return (len([]rune(s)) + 3) / 4
}
