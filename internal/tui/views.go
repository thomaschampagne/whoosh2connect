package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"whoosh2connect/internal/app"
)

func resLabel(res app.UploadResult) string {
	switch res.Status {
	case app.StatusUploaded:
		return "uploaded"
	case app.StatusDuplicate:
		return "already on Garmin"
	case app.StatusSkipped:
		return "skipped"
	default:
		if res.Err != nil {
			return res.Err.Error()
		}
		return "failed"
	}
}

func msgPrefix(id string) string {
	if id == "" || id == "batch" {
		return ""
	}
	return id + ": "
}

func resultColor(th theme, res app.UploadResult) lipgloss.Color {
	switch res.Status {
	case app.StatusUploaded:
		return th.success
	case app.StatusDuplicate, app.StatusSkipped:
		return th.secondary
	default:
		return th.danger
	}
}

func (m Model) renderDevice() string {
	name := m.settings.DevicePreset
	if m.settings.DeviceName != "" {
		name += " (" + m.settings.DeviceName + ")"
	}
	return m.th.paint("device: "+name, m.th.muted)
}

func (m Model) renderList() string {
	th := m.th
	var b strings.Builder
	if m.uploading > 0 {
		b.WriteString(th.paint(m.spinner.View(), th.primary))
		b.WriteString(" working…\n\n")
	}
	if len(m.sessions) == 0 {
		if m.screen == screenLoading {
			b.WriteString(th.paint(m.spinner.View(), th.primary))
			b.WriteString(" logging in…")
			return b.String()
		}
		b.WriteString(th.paint("  no sessions yet", th.muted))
		return b.String()
	}
	for i, s := range m.sessions {
		var marker string
		if s.Uploaded {
			marker = th.paint("✓", th.success)
		} else {
			marker = th.paint("•", th.muted)
		}
		if i == m.cursor && m.screen == screenList {
			marker = th.bold("➤", th.accent)
		}
		name := s.Name
		if name == "" {
			name = s.ID
		}
		starts := s.ParsedStart().Format("2006-01-02 15:04")
		res := th.paint("pending", th.muted)
		if r, ok := m.results[s.ID]; ok {
			res = th.paint(r, th.secondary)
		}
		fmt.Fprintf(&b, "  %s  %-18s  %s  %s\n", marker, starts, name, res)
	}
	fmt.Fprintf(&b, "\n  %d pending of %d total\n", m.pendingCount(), len(m.sessions))
	return b.String()
}

func (m Model) renderMFA() string {
	th := m.th
	var b strings.Builder
	b.WriteString(th.bold("  Two-factor authentication", th.accent))
	b.WriteString("\n")
	if m.mfaMethod != "" {
		b.WriteString(th.paint("  code sent to "+m.mfaMethod, th.muted))
		b.WriteString("\n")
	}
	b.WriteString("\n  " + m.mfaInput.View() + "\n")
	b.WriteString("\n" + th.paint("  enter: submit  esc: cancel", th.muted))
	return b.String()
}

func (m Model) renderConfirm() string {
	th := m.th
	return th.paint(fmt.Sprintf("  Upload %d pending session(s)?  (y/n)", m.confirm), th.primary)
}

func (m Model) renderHelp() string {
	th := m.th
	items := []string{"↑/↓ move", "enter upload", "a upload all", "s settings", "r refresh", "? help", "q quit"}
	return th.paint("  "+strings.Join(items, " │ "), th.muted)
}

func (m Model) renderStatus() string {
	th := m.th
	left := m.status
	if left == "" {
		left = th.paint("  press ? for help", th.muted)
	}
	return lipgloss.JoinHorizontal(lipgloss.Left,
		th.paint(" "+left+" ", th.text),
	)
}
