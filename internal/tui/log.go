package tui

import (
	"fmt"
	"time"
)

// logLevel classifies a log line for colouring and iconography.
type logLevel int

const (
	logInfo logLevel = iota
	logSuccess
	logWarn
	logError
)

// logEntry is a single timestamped event shown in the log panel.
type logEntry struct {
	at    time.Time
	level logLevel
	msg   string
}

// logLimit is the maximum number of retained entries.
const logLimit = 500

// logBuffer is a bounded, scrollable ring of log entries.
type logBuffer struct {
	entries []logEntry
	scroll  int // offset from the bottom; 0 follows the tail
}

// add appends an entry, trimming the oldest beyond logLimit.
func (l *logBuffer) add(level logLevel, msg string) {
	l.entries = append(l.entries, logEntry{at: time.Now(), level: level, msg: msg})
	if len(l.entries) > logLimit {
		l.entries = l.entries[len(l.entries)-logLimit:]
	}
	l.scroll = 0
}

// lines renders up to n entries, newest last, honouring the scroll offset.
// It returns the visible window and the clamped scroll value.
func (l logBuffer) lines(n int) []logEntry {
	if n < 1 || len(l.entries) == 0 {
		return nil
	}
	end := len(l.entries) - l.scroll
	if end < 0 {
		end = 0
	}
	if end > len(l.entries) {
		end = len(l.entries)
	}
	start := end - n
	if start < 0 {
		start = 0
	}
	return l.entries[start:end]
}

// maxScroll is the largest valid scroll offset for a window of n rows.
func (l logBuffer) maxScroll(n int) int {
	if n < 1 {
		return 0
	}
	if extra := len(l.entries) - n; extra > 0 {
		return extra
	}
	return 0
}

// clamp keeps the scroll offset within range for a window of n rows.
func (l *logBuffer) clamp(n int) {
	if l.scroll < 0 {
		l.scroll = 0
	}
	if max := l.maxScroll(n); l.scroll > max {
		l.scroll = max
	}
}

func (l logBuffer) render(th theme, width, height int) string {
	if height < 1 {
		return ""
	}
	if len(l.entries) == 0 {
		return th.dim("no log entries yet")
	}
	entries := l.lines(height)
	var out []string
	for _, e := range entries {
		stamp := th.dim(e.at.Format("15:04:05"))
		out = append(out, clip("  "+stamp+" "+iconForLevel(th, e.level)+" "+e.msg, width))
	}
	return joinLines(out)
}

func iconForLevel(th theme, l logLevel) string {
	switch l {
	case logSuccess:
		return th.paint("✓", th.success)
	case logWarn:
		return th.paint("!", th.warn)
	case logError:
		return th.paint("✗", th.danger)
	default:
		return th.paint("·", th.muted)
	}
}

func (l logBuffer) count() int { return len(l.entries) }

func (l logBuffer) String() string {
	return fmt.Sprintf("%d entries", len(l.entries))
}
