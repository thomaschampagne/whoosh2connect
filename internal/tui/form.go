package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"whoosh2connect/internal/device"
)

// maxSuggestions caps how many device presets the autocomplete shows at once.
const maxSuggestions = 6

// bodyTop is the inner row at which every screen's body starts (after the
// header, stats line, and a blank).
const bodyTop = 4

// settingsForm is the interactive settings editor: one text input per field,
// plus an autocomplete dropdown for the device preset.
type settingsForm struct {
	inputs  []textinput.Model
	focus   int
	sugg    []device.Preset
	suggIdx int
}

// newSettingsForm builds the form from existing settings and focuses field one.
func newSettingsForm(th theme, s Settings) settingsForm {
	f := settingsForm{inputs: make([]textinput.Model, len(settingsFields))}
	for i, spec := range settingsFields {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = spec.placeholder
		ti.CharLimit = spec.maxLen
		ti.EchoCharacter = '•'
		ti.TextStyle = lipgloss.NewStyle().Foreground(th.text)
		ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(th.muted)
		if spec.kind == fieldSecret {
			ti.EchoMode = textinput.EchoPassword
		}
		ti.SetValue(fieldValue(s, i))
		f.inputs[i] = ti
	}
	f.focusField(0)
	return f
}

// apply copies the form values back into a Settings value.
func (f settingsForm) apply(s *Settings) {
	for i := range f.inputs {
		s.setField(i, f.inputs[i].Value())
	}
}

// setWidth sizes the visible portion of every input to the given inner width.
func (f *settingsForm) setWidth(w int) {
	iw := w - formLabelWidth() - 6
	if iw < 12 {
		iw = 12
	}
	for i := range f.inputs {
		f.inputs[i].Width = iw
	}
}

// focusField focuses field i (wrapping) and returns the cursor blink command.
func (f *settingsForm) focusField(i int) tea.Cmd {
	n := len(f.inputs)
	i = ((i % n) + n) % n
	f.focus = i
	var cmd tea.Cmd
	for j := range f.inputs {
		if j == i {
			cmd = f.inputs[j].Focus()
		} else {
			f.inputs[j].Blur()
		}
	}
	f.refreshSuggestions()
	return cmd
}

// update routes a message to the focused input, handling form-level keys first.
func (f *settingsForm) update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "tab":
			if f.acceptSuggestion() {
				return nil
			}
			return f.focusField(f.focus + 1)
		case "shift+tab":
			return f.focusField(f.focus - 1)
		case "enter":
			if f.acceptSuggestion() {
				return nil
			}
			return f.focusField(f.focus + 1)
		case "up":
			if f.moveSuggestion(-1) {
				return nil
			}
			return f.focusField(f.focus - 1)
		case "down":
			if f.moveSuggestion(1) {
				return nil
			}
			return f.focusField(f.focus + 1)
		}
		if f.focus == fldDeviceSerial && km.Type == tea.KeyRunes {
			digits := make([]rune, 0, len(km.Runes))
			for _, r := range km.Runes {
				if r >= '0' && r <= '9' {
					digits = append(digits, r)
				}
			}
			if len(digits) == 0 {
				return nil
			}
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: digits}
		}
	}
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	f.refreshSuggestions()
	return cmd
}

// refreshSuggestions recomputes the preset dropdown for the current input.
func (f *settingsForm) refreshSuggestions() {
	f.sugg = nil
	f.suggIdx = 0
	if f.focus != fldDevicePreset {
		return
	}
	val := strings.ToLower(strings.TrimSpace(f.inputs[fldDevicePreset].Value()))
	for _, p := range device.Presets() {
		if val == "" {
			f.sugg = append(f.sugg, p)
		} else if p.ID == val {
			return
		} else if strings.Contains(strings.ToLower(p.ID), val) {
			f.sugg = append(f.sugg, p)
		}
		if len(f.sugg) >= maxSuggestions {
			return
		}
	}
}

// acceptSuggestion fills the preset field with the highlighted suggestion.
func (f *settingsForm) acceptSuggestion() bool {
	if f.focus != fldDevicePreset || len(f.sugg) == 0 {
		return false
	}
	idx := f.suggIdx
	if idx < 0 || idx >= len(f.sugg) {
		idx = 0
	}
	f.inputs[fldDevicePreset].SetValue(f.sugg[idx].ID)
	f.inputs[fldDevicePreset].CursorEnd()
	f.refreshSuggestions()
	return true
}

// moveSuggestion moves the dropdown highlight when it is open.
func (f *settingsForm) moveSuggestion(delta int) bool {
	if f.focus != fldDevicePreset || len(f.sugg) == 0 {
		return false
	}
	f.suggIdx = (f.suggIdx + delta + len(f.sugg)) % len(f.sugg)
	return true
}

// lines renders the form at the given inner width.
func (f settingsForm) lines(th theme, s Settings, width, height int) []string {
	labelW := formLabelWidth()
	var out []string
	for i, spec := range settingsFields {
		active := i == f.focus
		lead := "  "
		label := th.paint(padLabel(spec.label, labelW), th.text)
		if active {
			lead = th.bold("▸", th.accent) + " "
			label = th.bold(padLabel(spec.label, labelW), th.accent)
		}
		line := lead + label + "  " + f.inputs[i].View()
		showingSugg := active && i == fldDevicePreset && len(f.sugg) > 0
		if e := fieldError(s, i); e != "" && !showingSugg {
			line += "  " + th.paint("← "+e, th.danger)
		}
		out = append(out, clip(line, width))
		if showingSugg {
			out = append(out, f.suggestionLines(th, width)...)
		}
	}
	out = append(out, "")
	out = append(out, clip(th.dim("  enter/tab next · shift+tab/↑ back · ctrl+s save · esc cancel"), width))
	if height > 0 && len(out) > height {
		out = out[:height]
	}
	return out
}

func (f settingsForm) suggestionLines(th theme, width int) []string {
	out := make([]string, 0, len(f.sugg))
	for j, p := range f.sugg {
		if j == f.suggIdx {
			out = append(out, clip("    "+th.paint("›", th.accent)+" "+th.paint(p.ID, th.accent)+"  "+th.dim(p.DefaultName), width))
		} else {
			out = append(out, clip("      "+th.paint(p.ID, th.text)+"  "+th.dim(p.DefaultName), width))
		}
	}
	return out
}

// fieldRow returns the inner row (0-based) where field i is drawn.
func (f settingsForm) fieldRow(i int) int {
	row := bodyTop + i
	if i > fldDevicePreset && f.focus == fldDevicePreset {
		row += len(f.sugg)
	}
	return row
}

// suggestionAt maps an inner row to a dropdown index, if that row is a
// suggestion.
func (f settingsForm) suggestionAt(row int) (int, bool) {
	if f.focus != fldDevicePreset || len(f.sugg) == 0 {
		return 0, false
	}
	start := bodyTop + fldDevicePreset + 1
	if row < start || row >= start+len(f.sugg) {
		return 0, false
	}
	return row - start, true
}

func formLabelWidth() int {
	w := 0
	for _, f := range settingsFields {
		if len(f.label) > w {
			w = len(f.label)
		}
	}
	return w
}

func padLabel(s string, w int) string {
	return fmt.Sprintf("%-*s", w, s)
}
