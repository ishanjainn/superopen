package guards

import (
	"time"
)

// TimestampFormat is the canonical layout for a event timestamp: RFC3339 in UTC
// with fixed-width nanoseconds.
//
// It is deliberately not time.RFC3339Nano. That layout trims trailing zeros from the
// fractional second, so the same clock renders at three different widths depending on the
// value it happens to land on ("...:05Z", "...:05.5Z", "...:05.000000001Z"). Fixed width
// keeps every event the same shape, which is what makes a byte-wise comparison of two
// UTC event timestamps agree with their chronological order -- something SIEM queries,
// `sort` over a JSONL tail, and human eyes on a log all end up relying on.
const TimestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

// FormatTimestamp renders t as a canonical event timestamp. Every writer of the
// endpoint event stream goes through here, so the two capture paths -- the hook adapter
// and the collector exporter -- stamp the same instant the same way.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(TimestampFormat)
}

// ParseTimestamp parses a event timestamp. It accepts the canonical nanosecond
// form and the second-resolution RFC3339 that agents wrote before nanosecond stamping,
// so a log spanning an agent upgrade still reads back end to end.
func ParseTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
