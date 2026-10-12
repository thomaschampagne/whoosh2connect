package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// theme holds the color set for a light or dark rendering.
type theme struct {
	primary   lipgloss.Color
	secondary lipgloss.Color
	success   lipgloss.Color
	warn      lipgloss.Color
	danger    lipgloss.Color
	muted     lipgloss.Color
	accent    lipgloss.Color
	text      lipgloss.Color
}

var (
	darkTheme = theme{
		primary:   "#89b4fa",
		secondary: "#a6e3a1",
		success:   "#a6e3a1",
		warn:      "#f9e2af",
		danger:    "#f38ba8",
		muted:     "#7f849c",
		accent:    "#cba6f7",
		text:      "#cdd6f4",
	}
	lightTheme = theme{
		primary:   "#1e66f5",
		secondary: "#40a02b",
		success:   "#40a02b",
		warn:      "#df8e1d",
		danger:    "#d20f39",
		muted:     "#9ca0b0",
		accent:    "#8839ef",
		text:      "#4c4f69",
	}
)

// paint renders s in color c.
func (t theme) paint(s string, c lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render(s)
}

// bold renders s bold in color c.
func (t theme) bold(s string, c lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(s)
}

// panel returns a rounded, bordered box styled in the theme. The caller must
// pre-render content to the exact inner width/height: the panel adds a 1-cell
// border and 1-cell horizontal padding, so the final box is width+4 by
// height+2 cells.
func (t theme) panel() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.primary).
		Padding(0, 1)
}

// dim renders s with reduced emphasis.
func (t theme) dim(s string) string {
	return lipgloss.NewStyle().Faint(true).Foreground(t.muted).Render(s)
}

// clip truncates s to w display columns, appending an ellipsis when cut. It is
// ANSI-aware, so styled strings survive truncation.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return ansi.Truncate(s, w, "…")
}

// padLine right-pads s with spaces to exactly w display columns (truncating if
// longer). Used to guarantee every rendered line matches the frame width.
func padLine(s string, w int) string {
	s = clip(s, w)
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// fitLines forces s into exactly h lines of w columns, clipping both axes. It
// is the last line of defence against terminal overflow on resize.
func fitLines(s string, w, h int) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	raw := strings.Split(s, "\n")
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		if i < len(raw) {
			out = append(out, padLine(raw[i], w))
		} else {
			out = append(out, strings.Repeat(" ", w))
		}
	}
	return strings.Join(out, "\n")
}

// rule returns a horizontal divider of exactly w display columns.
func rule(t theme, w int) string {
	if w < 1 {
		w = 1
	}
	return t.paint(strings.Repeat("─", w), t.muted)
}
