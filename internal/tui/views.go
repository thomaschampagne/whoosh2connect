package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/version"
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

// winW returns the layout width, defaulting to 80 before the first resize.
func (m Model) winW() int {
	if m.width > 0 {
		return m.width
	}
	return 80
}

// contentW is the width available for content inside the outer frame.
func (m Model) contentW() int {
	return m.winW() - 4
}

func (m Model) renderDevice() string {
	name := m.settings.DevicePreset
	if m.settings.DeviceName != "" {
		name += " (" + m.settings.DeviceName + ")"
	}
	return m.th.paint("device: "+name, m.th.muted)
}

func (m Model) renderHeader() string {
	th := m.th
	left := th.bold("whoosh2connect", th.accent)
	if version.String() != "dev" && version.String() != "" {
		left += "  " + th.paint(version.String(), th.muted)
	} else {
		left += "  " + th.paint("dev", th.muted)
	}
	right := m.renderDevice()
	pad := m.contentW() - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 2 {
		pad = 2
	}
	rule := th.paint(strings.Repeat("─", m.contentW()), th.muted)
	return left + strings.Repeat(" ", pad) + right + "\n" + rule
}

type counts struct {
	synced  int
	pending int
	dupes   int
	total   int
}

func (m Model) stats() counts {
	var c counts
	c.total = len(m.sessions)
	for _, s := range m.sessions {
		if s.Uploaded {
			c.synced++
			if r, ok := m.results[s.ID]; ok && strings.Contains(r, "already on Garmin") {
				c.dupes++
			}
		} else {
			c.pending++
		}
	}
	return c
}

func (m Model) renderStats() string {
	th := m.th
	c := m.stats()
	var b strings.Builder
	b.WriteString(th.paint("✓", th.success))
	b.WriteString(th.paint(" "+itoa(c.synced)+" synced  ", th.text))
	b.WriteString(th.paint("○", th.secondary))
	b.WriteString(th.paint(" "+itoa(c.pending)+" pending  ", th.text))
	if c.dupes > 0 {
		b.WriteString(th.paint("●", th.muted))
		b.WriteString(th.paint(" "+itoa(c.dupes)+" on Garmin", th.text))
	}
	return b.String()
}

// pageSessions returns the slice of sessions shown on the current page.
func (m Model) pageSessions() []app.Session {
	start := m.pageStart()
	if start >= len(m.sessions) {
		return nil
	}
	end := start + m.pageSize()
	if end > len(m.sessions) {
		end = len(m.sessions)
	}
	return m.sessions[start:end]
}

func (m Model) renderList() string {
	th := m.th
	width := m.contentW()

	var b strings.Builder
	if m.screen == screenLoading {
		b.WriteString(th.paint(m.spinner.View(), th.primary))
		b.WriteString(th.paint(" signing in…", th.text))
		return b.String()
	}

	if len(m.sessions) == 0 {
		b.WriteString(th.paint("  no sessions yet — ride in MyWhoosh and hit r to refresh", th.muted))
		return b.String()
	}

	ps := m.pageSessions()
	titleW := width - 2 - 18 - 12
	maxTitle := 24
	for _, s := range ps {
		if n := lipgloss.Width(displayTitle(s)); n > maxTitle {
			maxTitle = n
		}
	}
	if maxTitle > titleW {
		maxTitle = titleW
	}

	for i, s := range ps {
		abs := m.pageStart() + i
		var marker string
		if abs == m.cursor {
			marker = th.bold("➤", th.accent)
		} else {
			marker = " "
		}
		var status string
		switch {
		case s.Uploaded:
			status = "✓"
		default:
			status = "○"
		}
		if r, ok := m.results[s.ID]; ok {
			status = iconForResult(r)
		}
		title := displayTitle(s)
		if lipgloss.Width(title) > maxTitle {
			title = truncate(title, maxTitle)
		}
		foot := fmt.Sprintf("%-2s %-18s %-*s %s", marker, s.ParsedStart().Format("02 Jan 15:04"), maxTitle, title, status)
		if abs == m.cursor && m.screen == screenList {
			foot = th.paint(foot, th.text)
		} else {
			foot = th.paint(foot, th.muted)
		}
		b.WriteString("  " + foot + "\n")
	}

	if m.pageCount() > 1 {
		fmt.Fprintf(&b, "  %s\n", th.dim(fmt.Sprintf("page %d/%d · %d ride(s)", m.page+1, m.pageCount(), len(m.sessions))))
	} else {
		c := m.stats()
		fmt.Fprintf(&b, "  %s\n", th.dim(fmt.Sprintf("%d ride(s) · %d synced · %d pending", c.total, c.synced, c.pending)))
	}
	return th.panel(width).Render(strings.TrimSuffix(b.String(), "\n"))
}

func (m Model) renderPreview() string {
	th := m.th
	var b strings.Builder
	if s := m.selected(); s != nil {
		b.WriteString(th.bold("  "+displayTitle(*s), th.text) + "\n")
		b.WriteString(th.paint("  "+s.ParsedStart().Format("Mon 2 Jan 2006 · 15:04"), th.muted) + "\n")
		sync := th.paint("pending", th.secondary)
		if s.Uploaded {
			sync = th.paint("synced", th.success)
			if at := s.UploadedAt; !at.IsZero() {
				sync = th.paint("synced "+at.Format("02 Jan 15:04"), th.success)
			}
		}
		if r, ok := m.results[s.ID]; ok {
			sync = th.paint(r, resultColorFromKey(th, r))
		}

		rows := [][2]string{
			{"route", s.RouteName},
			{"distance", fmt.Sprintf("%.1f km", s.Distance)},
			{"duration", s.RideDuration},
			{"avg power", fmt.Sprintf("%.0f W", s.AvgPower)},
			{"avg HR", fmt.Sprintf("%.0f bpm", s.AvgHR)},
			{"elevation", fmt.Sprintf("%.0f m", s.Elevation)},
			{"sport", s.SportType},
		}
		labelW := 9
		for _, r := range rows {
			if r[1] == "" || r[1] == "0" || r[1] == "0.0" {
				continue
			}
			b.WriteString("  " + th.paint(fmt.Sprintf("%-*s", labelW, r[0]), th.muted) + th.paint(r[1], th.text) + "\n")
		}
		b.WriteString("\n  " + th.paint("status: ", th.muted) + sync + "\n")
	}
	return th.panel(m.contentW()).Render(strings.TrimSuffix(b.String(), "\n"))
}

func (m Model) renderMFA() string {
	th := m.th
	var b strings.Builder
	b.WriteString(th.bold("  Two-factor authentication", th.accent) + "\n")
	if m.mfaMethod != "" {
		b.WriteString(th.paint("  code sent to "+m.mfaMethod, th.muted) + "\n")
	}
	b.WriteString("\n  " + m.mfaInput.View() + "\n")
	b.WriteString(th.paint("\n  enter submit · esc cancel", th.muted))
	return th.panel(m.contentW()).Render(strings.TrimSuffix(b.String(), "\n"))
}

func (m Model) renderConfirm() string {
	th := m.th
	var b strings.Builder
	b.WriteString(th.bold(fmt.Sprintf("  Upload %d pending ride(s) to Garmin?", m.confirm), th.text) + "\n")
	b.WriteString(th.paint("  y/enter upload · n/esc cancel", th.muted))
	return th.panel(m.contentW()).Render(b.String())
}

func (m Model) renderHelp() string {
	th := m.th
	lines := []string{
		"Navigate:  ↑/j move   enter preview   esc back",
		"Actions:   u upload   a upload all   r refresh",
		"Screens:   s settings   ? help   q quit",
	}
	for i := range lines {
		lines[i] = "  " + th.paint(lines[i], th.text)
	}
	return th.panel(m.contentW()).Render(strings.Join(lines, "\n"))
}

func (m Model) renderStatus() string {
	th := m.th
	var b strings.Builder
	if m.uploading > 0 {
		b.WriteString(th.paint(m.spinner.View(), th.primary))
		b.WriteString(th.paint(" syncing", th.text))
		if m.uploadingName != "" {
			b.WriteString(th.paint(" "+m.uploadingName, th.muted))
		}
		b.WriteString(th.paint("…", th.text))
	} else if m.status != "" {
		b.WriteString(th.paint(m.status, th.text))
	} else {
		b.WriteString(th.dim("↑/↓ move · enter preview · u upload · a upload all · s settings · r refresh · ? help · q quit"))
	}
	return b.String()
}

func (m Model) renderFooter() string {
	if m.showHelp {
		return m.renderHelp()
	}
	return m.renderStatus()
}

func displayTitle(s app.Session) string {
	if s.Name != "" {
		return s.Name
	}
	if s.RouteName != "" {
		return s.RouteName
	}
	return fmt.Sprintf("Ride · %s", s.ParsedStart().Format("02 Jan 15:04"))
}

func truncate(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func iconForResult(label string) string {
	switch {
	case strings.Contains(label, "uploaded"):
		return "●"
	case strings.Contains(label, "already on Garmin"):
		return "●"
	case strings.Contains(label, "skipped"):
		return "✓"
	default:
		return "!"
	}
}

func resultColorFromKey(th theme, label string) lipgloss.Color {
	switch {
	case strings.Contains(label, "uploaded"):
		return th.success
	case strings.Contains(label, "already on Garmin"):
		return th.secondary
	case strings.Contains(label, "skipped"):
		return th.success
	default:
		return th.danger
	}
}
