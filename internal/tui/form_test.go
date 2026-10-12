package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/device"
)

func appSessionList() []app.Session {
	return []app.Session{session("1"), session("2"), session("3")}
}

func displayWidth(s string) int { return ansi.StringWidth(s) }

// enterSettings puts a model on the settings screen with the form ready.
func enterSettings(t *testing.T, m *Model) {
	t.Helper()
	m.screen = screenList
	upd(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.screen != screenSettings {
		t.Fatalf("screen = %v, want settings", m.screen)
	}
}

func TestSettingsBackwardNavWraps(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	enterSettings(t, &m)
	if m.form.focus != 0 {
		t.Fatalf("initial focus = %d", m.form.focus)
	}
	upd(t, &m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.form.focus != len(settingsFields)-1 {
		t.Fatalf("shift+tab wrap focus = %d, want %d", m.form.focus, len(settingsFields)-1)
	}
}

func TestSettingsPresetAutocomplete(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	enterSettings(t, &m)
	for i := 0; i < fldDevicePreset; i++ {
		upd(t, &m, tea.KeyMsg{Type: tea.KeyTab})
	}
	if m.form.focus != fldDevicePreset {
		t.Fatalf("focus = %d, want preset", m.form.focus)
	}

	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("edge_5")})
	if len(m.form.sugg) == 0 {
		t.Fatal("expected preset suggestions")
	}
	for _, p := range m.form.sugg {
		if !strings.Contains(p.ID, "edge_5") {
			t.Fatalf("suggestion %q does not match input", p.ID)
		}
	}

	pick := m.form.sugg[0].ID
	upd(t, &m, tea.KeyMsg{Type: tea.KeyTab}) // tab accepts the highlighted option
	if got := m.form.inputs[fldDevicePreset].Value(); got != pick {
		t.Fatalf("accepted preset = %q, want %q", got, pick)
	}
	if _, err := device.Lookup(pick); err != nil {
		t.Fatalf("accepted an unknown preset: %v", err)
	}
}

func TestSettingsSerialRejectsNonDigits(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	enterSettings(t, &m)
	m.form.focusField(fldDeviceSerial)
	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("12ab34")})
	if got := m.form.inputs[fldDeviceSerial].Value(); got != "1234" {
		t.Fatalf("serial input = %q, want digits only", got)
	}
}

func TestSettingsTypingQDoesNotQuit(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	enterSettings(t, &m)
	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.screen != screenSettings {
		t.Fatalf("screen = %v; typing q must not quit", m.screen)
	}
	if !strings.Contains(fieldValue(m.settings, fldMywhooshEmail), "q") {
		t.Fatalf("q not inserted into email field: %q", fieldValue(m.settings, fldMywhooshEmail))
	}
}

func TestLogsToggleAndEntries(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	m.screen = screenList
	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !m.showLog {
		t.Fatal("l should open the log panel")
	}
	if m.logs.count() == 0 {
		t.Fatal("expected log entries from startup")
	}
	if !strings.Contains(m.View(), "Logs") {
		t.Fatalf("log panel not rendered:\n%s", m.View())
	}
	upd(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showLog {
		t.Fatal("esc should close the log panel")
	}
}

func TestHelpToggle(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	m.screen = screenList
	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.showHelp || !strings.Contains(m.View(), "Keyboard shortcuts") {
		t.Fatalf("help not shown:\n%s", m.View())
	}
	upd(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp {
		t.Fatal("esc should close help")
	}
}

func TestMouseClickSelectsThenPreviews(t *testing.T) {
	m := New(Options{Service: &fakeService{}, Settings: Settings{DevicePreset: "edge_540"}})
	m.screen = screenList
	m.sessions = appSessionList() // 3 sessions
	m.width, m.height = 80, 24

	// First ride row sits at inner row bodyTop; terminal Y is one lower.
	y := 1 + bodyTop + 1
	upd(t, &m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: y})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after click", m.cursor)
	}
	upd(t, &m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: y})
	if m.screen != screenPreview {
		t.Fatalf("screen = %v, want preview on second click", m.screen)
	}
}

func TestViewRespectsTerminalSize(t *testing.T) {
	m := New(Options{Service: &fakeService{}, Settings: Settings{DevicePreset: "edge_540"}})
	m.screen = screenList
	m.sessions = appSessionList()
	m.showHelp = false
	for _, size := range [][2]int{{40, 12}, {80, 24}, {120, 40}, {200, 60}} {
		m.width, m.height = size[0], size[1]
		lines := strings.Split(m.View(), "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), size[1])
		}
		for i, l := range lines {
			if w := displayWidth(l); w != size[0] {
				t.Errorf("%dx%d: line %d width %d, want %d: %q", size[0], size[1], i, w, size[0], l)
			}
		}
	}
}

func TestViewSmallHeightDoesNotPanic(t *testing.T) {
	m := New(Options{Service: &fakeService{}, Settings: Settings{DevicePreset: "edge_540"}})
	m.sessions = appSessionList()
	for _, h := range []int{1, 2, 6, 10} {
		m.width, m.height = 80, h
		_ = m.View()
	}
}
