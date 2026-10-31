package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/version"
)

// chromeH is the number of non-body rows: header (2), stats (1), gap (1), gap
// (1), status (1), hint (1).
const chromeH = 7

// --- layout math -------------------------------------------------------------

// winW is the terminal width, defaulting to 80 before the first resize.
func (m Model) winW() int {
	if m.width > 0 {
		return m.width
	}
	return 80
}

// winH is the terminal height, defaulting to 24 before the first resize.
func (m Model) winH() int {
	if m.height > 0 {
		return m.height
	}
	return 24
}

// innerW is the width inside the outer frame (border + 1-col padding each side).
func (m Model) innerW() int {
	return max(10, m.winW()-4)
}

// innerH is the height inside the outer frame (border only, no vertical pad).
func (m Model) innerH() int {
	return max(6, m.winH()-2)
}

// bodyH is the number of rows available to the current screen's body.
func (m Model) bodyH() int {
	return max(1, m.innerH()-chromeH)
}

// logPaneHeight is the height of the bottom log pane when it is open.
func (m Model) logPaneHeight() int {
	if !m.showLog {
		return 0
	}
	h := m.bodyH() / 3
	h = min(max(h, 3), 10)
	h = min(h, m.bodyH()-2)
	return max(h, 0)
}

// listHeight is the height left for the ride list (log pane subtracted).
func (m Model) listHeight() int {
	h := m.bodyH()
	if m.showLog {
		h -= m.logPaneHeight() + 1
	}
	return max(1, h)
}

// listRows is how many ride rows fit on a page (one row is reserved for the
// pager/legend line when there are rides).
func (m Model) listRows() int {
	h := m.listHeight()
	if len(m.sessions) > 0 {
		h--
	}
	return max(1, h)
}

// --- top-level render --------------------------------------------------------

// View renders the current screen, clamped to the terminal.
func (m Model) View() string {
	inner := fitLines(joinLines(m.buildInner()), m.innerW(), m.innerH())
	// Box the content at exactly innerW×innerH (no wrap/padding here), then
	// add the border + padding on top so the finished frame is winW×winH.
	content := lipgloss.NewStyle().Width(m.innerW()).Height(m.innerH()).Render(inner)
	return m.th.panel().Render(content)
}

func (m Model) buildInner() []string {
	out := make([]string, 0, m.innerH())
	out = append(out, m.renderHeader()...)
	out = append(out, m.renderStats())
	out = append(out, "")

	body := m.renderBody()
	if len(body) < m.bodyH() {
		for len(body) < m.bodyH() {
			body = append(body, "")
		}
	}
	if len(body) > m.bodyH() {
		body = body[:m.bodyH()]
	}
	out = append(out, body...)

	out = append(out, "", m.renderStatus(), m.renderHint())
	return out
}

func (m Model) renderHeader() []string {
	th := m.th
	left := th.bold("whoosh2connect", th.accent)
	ver := shortVersion()
	left += "  " + th.paint(ver, th.muted)
	right := m.renderDevice()

	gap := m.innerW() - ansi.StringWidth(left) - ansi.StringWidth(right)
	var line string
	if gap >= 2 {
		line = left + strings.Repeat(" ", gap) + right
	} else {
		line = clip(left+"  "+right, m.innerW())
	}
	return []string{line, rule(th, m.innerW())}
}

// shortVersion renders "dev" or "X.Y.Z (commit sha)" without repeating the app
// name that version.String() already carries.
func shortVersion() string {
	if version.Version == "dev" && version.Commit == "none" {
		return "dev"
	}
	return fmt.Sprintf("%s (commit %s)", version.Version, version.Commit)
}

func (m Model) renderDevice() string {
	name := m.settings.DevicePreset
	if name == "" {
		name = "unset"
	}
	if m.settings.DeviceName != "" {
		name += " (" + m.settings.DeviceName + ")"
	}
	return m.th.paint("device: "+name, m.th.muted)
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
	parts := []string{
		th.paint("✓", th.success) + th.paint(" "+itoa(c.synced)+" synced", th.text),
		th.paint("○", th.secondary) + th.paint(" "+itoa(c.pending)+" pending", th.text),
	}
	if c.dupes > 0 {
		parts = append(parts, th.paint("●", th.muted)+th.paint(" "+itoa(c.dupes)+" on Garmin", th.text))
	}
	return clip("  "+strings.Join(parts, "   "), m.innerW())
}

// --- body dispatch -----------------------------------------------------------

func (m Model) renderBody() []string {
	bodyH := m.bodyH()
	switch {
	case m.showHelp:
		return m.helpLines()
	case m.showLog && m.screen != screenList:
		return m.logLines(bodyH)
	}
	switch m.screen {
	case screenLoading, screenList:
		if m.showLog {
			return m.listWithLogs()
		}
		return m.listLines(m.listHeight())
	case screenPreview:
		return m.previewLines()
	case screenSettings:
		return m.form.lines(m.th, m.settings, m.innerW(), bodyH)
	case screenMFA:
		return m.mfaLines()
	case screenConfirm:
		return m.confirmLines()
	}
	return nil
}

// pageSessions returns the slice of sessions shown on the current page.
func (m Model) pageSessions() []app.Session {
	start := m.pageStart()
	if start >= len(m.sessions) {
		return nil
	}
	end := min(start+m.pageSize(), len(m.sessions))
	return m.sessions[start:end]
}

func (m Model) listLines(height int) []string {
	th := m.th
	if m.screen == screenLoading {
		return []string{clip("  "+th.paint(m.spinner.View(), th.primary)+th.paint(" signing in…", th.text), m.innerW())}
	}
	if len(m.sessions) == 0 {
		return []string{clip("  "+th.paint("no sessions yet — ride in MyWhoosh, then press r to refresh", th.muted), m.innerW())}
	}

	ps := m.pageSessions()
	rows := min(len(ps), m.listRows())
	out := make([]string, 0, rows+1)
	for i := 0; i < rows; i++ {
		abs := m.pageStart() + i
		out = append(out, m.listRow(ps[i], abs == m.cursor, abs == m.cursor && m.screen == screenList))
	}
	out = append(out, clip("  "+th.dim(m.pagerText()), m.innerW()))
	return out
}

func (m Model) listRow(s app.Session, selected, active bool) string {
	th := m.th
	marker := " "
	if selected {
		marker = th.bold("➤", th.accent)
	}
	status, statusColor := "○", th.secondary
	if s.Uploaded {
		status, statusColor = "✓", th.success
	}
	if r, ok := m.results[s.ID]; ok {
		status, statusColor = iconForResult(r), resultColorFromKey(th, r)
	}

	date := s.ParsedStart().Format("02 Jan 15:04")
	titleW := max(6, m.innerW()-20)
	title := clip(displayTitle(s), titleW)
	titleColor, dateColor := th.muted, th.muted
	if active {
		titleColor, dateColor = th.text, th.text
	}
	pad := strings.Repeat(" ", max(0, titleW-ansi.StringWidth(title)))
	line := "  " + marker + " " + th.paint(date, dateColor) + " " + th.paint(title, titleColor) + pad + " " + th.paint(status, statusColor)
	return clip(line, m.innerW())
}

func (m Model) pagerText() string {
	if m.pageCount() > 1 {
		return fmt.Sprintf("page %d/%d · %d ride(s)", m.page+1, m.pageCount(), len(m.sessions))
	}
	c := m.stats()
	return fmt.Sprintf("%d ride(s) · %d synced · %d pending", c.total, c.synced, c.pending)
}

func (m Model) listWithLogs() []string {
	lh, listH := m.logPaneHeight(), m.listHeight()
	out := m.listLines(listH)
	for len(out) < listH {
		out = append(out, "")
	}
	out = append(out, rule(m.th, m.innerW()))
	out = append(out, m.logLines(lh)...)
	return out
}

func (m Model) logLines(height int) []string {
	if height < 1 {
		return nil
	}
	th := m.th
	title := th.bold("Logs", th.accent) + th.dim("  (l close · ↑/↓ scroll)")
	out := []string{clip("  "+title, m.innerW())}
	if body := m.logs.render(th, m.innerW(), height-1); body != "" {
		out = append(out, strings.Split(body, "\n")...)
	}
	return out
}

func (m Model) previewLines() []string {
	th := m.th
	s := m.selected()
	if s == nil {
		return []string{th.dim("  nothing selected")}
	}
	out := []string{
		th.bold("  "+displayTitle(*s), th.text),
		th.paint("  "+s.ParsedStart().Format("Mon 2 Jan 2006 · 15:04"), th.muted),
		"",
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
	for _, r := range rows {
		if r[1] == "" || r[1] == "0" || r[1] == "0.0" {
			continue
		}
		out = append(out, clip("  "+th.paint(padLabel(r[0], 9), th.muted)+th.paint(r[1], th.text), m.innerW()))
	}
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
	out = append(out, "", "  "+th.paint("status: ", th.muted)+sync)
	return out
}

func (m Model) mfaLines() []string {
	th := m.th
	out := []string{
		th.bold("  Two-factor authentication", th.accent),
		th.paint("  Enter the code Garmin sent you.", th.muted),
		"",
		"  " + m.mfaInput.View(),
		"",
		th.paint("  enter submit · esc cancel", th.muted),
	}
	return out
}

func (m Model) confirmLines() []string {
	th := m.th
	return []string{
		th.bold(fmt.Sprintf("  Upload %d pending ride(s) to Garmin?", m.confirm), th.text),
		"",
		th.paint("  y / enter   upload", th.text),
		th.paint("  n / esc     cancel", th.muted),
	}
}

func (m Model) helpLines() []string {
	th := m.th
	rows := []string{
		th.bold("Keyboard shortcuts", th.accent),
		"",
		"  move            ↑ / ↓  or  k / j",
		"  preview ride    enter",
		"  upload ride     u",
		"  upload all      a",
		"  refresh         r",
		"  settings        s",
		"  logs            l",
		"  mouse on/off    m  (off: select and copy text)",
		"  help            ?",
		"  quit            q  or  ctrl+c",
		"",
		th.dim("  press ? or esc to close"),
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r == "" {
			out = append(out, "")
			continue
		}
		out = append(out, clip("  "+r, m.innerW()))
	}
	return out
}

func (m Model) renderStatus() string {
	th := m.th
	switch {
	case m.uploading > 0:
		s := th.paint(m.spinner.View(), th.primary) + th.paint(" syncing", th.text)
		if m.uploadingName != "" {
			s += th.paint(" "+m.uploadingName, th.muted)
		}
		return clip("  "+s+"…", m.innerW())
	case m.status != "":
		return clip("  "+m.status, m.innerW())
	case m.logs.count() > 0:
		last := m.logs.entries[len(m.logs.entries)-1]
		return clip("  "+th.dim("last: "+last.msg), m.innerW())
	}
	return ""
}

func (m Model) renderHint() string {
	th := m.th
	var hint string
	switch {
	case m.showHelp:
		hint = "?/esc close help"
	case m.showLog:
		hint = "l/esc close logs · ↑/↓ scroll logs"
	case m.screen == screenSettings:
		hint = "ctrl+s save · esc cancel"
	case m.screen == screenMFA:
		hint = "enter submit · esc cancel"
	case m.screen == screenConfirm:
		hint = "y upload · n cancel · q quit"
	case m.screen == screenPreview:
		hint = "u upload · esc back · q quit"
	default:
		hint = "↑/↓ move · enter preview · u upload · a upload-all · s settings · l logs · r refresh · ? help · q quit"
	}
	return clip("  "+th.dim(hint), m.innerW())
}

// --- shared helpers ----------------------------------------------------------

func joinLines(lines []string) string { return strings.Join(lines, "\n") }

func displayTitle(s app.Session) string {
	if s.Name != "" {
		return s.Name
	}
	if s.RouteName != "" {
		return s.RouteName
	}
	return fmt.Sprintf("Ride · %s", s.ParsedStart().Format("02 Jan 15:04"))
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func iconForResult(label string) string {
	switch {
	case strings.Contains(label, "uploaded"), strings.Contains(label, "already on Garmin"):
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
