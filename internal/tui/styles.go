package tui

import "github.com/charmbracelet/lipgloss"

// theme holds the color set for a light or dark rendering.
type theme struct {
	primary   lipgloss.Color
	secondary lipgloss.Color
	success   lipgloss.Color
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
		danger:    "#f38ba8",
		muted:     "#7f849c",
		accent:    "#cba6f7",
		text:      "#cdd6f4",
	}
	lightTheme = theme{
		primary:   "#1e66f5",
		secondary: "#40a02b",
		success:   "#40a02b",
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
