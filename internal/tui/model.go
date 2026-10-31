// Package tui implements the Bubble Tea interface for whoosh2connect.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/mywhoosh"
	"whoosh2connect/internal/notify"
)

// Service is the slice of app.Service the TUI needs.
type Service interface {
	LoginMyWhoosh(ctx context.Context, email, password string) error
	LoginGarmin(ctx context.Context, email, password string) error
	SubmitMFA(ctx context.Context, code string) error
	Sessions(ctx context.Context) ([]app.Session, error)
	Upload(ctx context.Context, act mywhoosh.Activity) app.UploadResult
	UploadPending(ctx context.Context, sessions []app.Session, report func(mywhoosh.Activity, app.UploadResult))
	SaveSpoofed(ctx context.Context, act mywhoosh.Activity, dir string) (string, error)
	Watch(ctx context.Context, sc app.RideScanner, interval time.Duration, n notify.Notifier, out chan<- app.WatchEvent)
}

type screen int

const (
	screenLoading screen = iota
	screenList
	screenPreview
	screenSettings
	screenMFA
	screenConfirm
)

// Options configures a Model.
type Options struct {
	Service      Service
	Settings     Settings
	SaveSettings func(Settings) error
	Dark         bool
	Context      context.Context
	// SpoofDir is where spoofed FIT files are saved on request.
	SpoofDir string
	// OpenDir opens a folder in the file manager.
	OpenDir func(path string) error
	// Watch configures auto-watch of the ride folder.
	Watch WatchOptions
}

// Model is the Bubble Tea state machine.
type Model struct {
	svc       Service
	ctx       context.Context
	settings  Settings
	saveFn    func(Settings) error
	th        theme
	watchOpts WatchOptions
	watch     *watchRun

	screen  screen
	cursor  int
	confirm int
	form    settingsForm

	sessions      []app.Session
	results       map[string]string
	status        string
	mfaInput      textinput.Model
	uploading     int
	uploadingName string
	batch         *batchRun

	showHelp bool
	showLog  bool
	logs     logBuffer
	mouseOn  bool
	spoofDir string
	openDir  func(string) error

	width  int
	height int

	spinner     spinner.Model
	page        int
	pageSizeVal int
}

// New creates a Model.
func New(opts Options) Model {
	th := darkTheme
	if opts.Dark {
		th = lightTheme
	}
	m := Model{
		svc:       opts.Service,
		ctx:       opts.Context,
		settings:  opts.Settings,
		saveFn:    func(Settings) error { return nil },
		th:        th,
		results:   make(map[string]string),
		screen:    screenLoading,
		mouseOn:   true,
		spoofDir:  opts.SpoofDir,
		openDir:   opts.OpenDir,
		watchOpts: opts.Watch,
	}
	if opts.Context == nil {
		m.ctx = context.Background()
	}
	if opts.SaveSettings != nil {
		m.saveFn = opts.SaveSettings
	}
	m.spinner = spinner.New()
	m.spinner.Spinner = spinner.Dot
	m.mfaInput = textinput.New()
	m.mfaInput.Placeholder = "6-digit code"
	m.mfaInput.Prompt = "  "
	m.mfaInput.CharLimit = 12
	m.form = newSettingsForm(th, opts.Settings)
	m.form.setWidth(m.innerW())
	m.logf(logInfo, "whoosh2connect %s starting", shortVersion())
	return m
}

// Init starts the bootstrap (login + session fetch).
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.bootstrapCmd())
}

type bootstrapMsg struct {
	sessions []app.Session
	err      error
	mfa      bool
}

type sessionsMsg struct {
	sessions []app.Session
	err      error
}

type uploadMsg struct {
	id  string
	res app.UploadResult
}

type spoofedMsg struct {
	id   string
	path string
	err  error
}

type mfaMsg struct{ err error }

// logf appends an entry to the in-model log panel.
func (m *Model) logf(level logLevel, format string, a ...interface{}) {
	m.logs.add(level, fmt.Sprintf(format, a...))
}

func (m Model) bootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		if err := m.svc.LoginMyWhoosh(ctx, m.settings.MywhooshEmail, m.settings.MywhooshPassword); err != nil {
			return bootstrapMsg{err: fmt.Errorf("mywhoosh login: %w", err)}
		}
		if err := m.svc.LoginGarmin(ctx, m.settings.GarminEmail, m.settings.GarminPassword); err != nil {
			if errors.Is(err, garmin.ErrMFARequired) {
				return bootstrapMsg{mfa: true}
			}
			return bootstrapMsg{err: fmt.Errorf("garmin login: %w", err)}
		}
		sessions, err := m.svc.Sessions(ctx)
		return bootstrapMsg{sessions: sessions, err: err}
	}
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.svc.Sessions(m.ctx)
		return sessionsMsg{sessions: sessions, err: err}
	}
}

func (m Model) uploadCmd(act mywhoosh.Activity) tea.Cmd {
	return func() tea.Msg {
		res := m.svc.Upload(m.ctx, act)
		return uploadMsg{id: act.ID, res: res}
	}
}

func (m Model) saveSpoofedCmd(act mywhoosh.Activity) tea.Cmd {
	return func() tea.Msg {
		path, err := m.svc.SaveSpoofed(m.ctx, act, m.spoofDir)
		return spoofedMsg{id: act.ID, path: path, err: err}
	}
}

func (m Model) mfaCmd(code string) tea.Cmd {
	return func() tea.Msg {
		return mfaMsg{err: m.svc.SubmitMFA(m.ctx, code)}
	}
}

// openSpoofDir opens the spoof folder in the file manager, creating it first.
func (m *Model) openSpoofDir() {
	if m.openDir == nil || m.spoofDir == "" {
		m.logf(logError, "no spoof folder configured")
		return
	}
	if err := os.MkdirAll(m.spoofDir, 0o700); err != nil {
		m.logf(logError, "create spoof folder: %v", err)
		return
	}
	if err := m.openDir(m.spoofDir); err != nil {
		m.logf(logError, "open folder: %v", err)
	}
}

// Update handles messages and input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.form.setWidth(m.innerW())
		m.clampCursor()
		m.clampPage()
		m.ensureCursorVisible()
		m.logs.clamp(max(0, m.logViewHeight()))

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case bootstrapMsg:
		if msg.err != nil {
			m.status = m.th.paint(msg.err.Error(), m.th.danger)
			m.logf(logError, "%s", msg.err.Error())
			m.screen = screenSettings
			m.form = newSettingsForm(m.th, m.settings)
			m.form.setWidth(m.innerW())
			target := 0
			if i := m.settings.errorField(); i >= 0 {
				target = i
			}
			cmds = append(cmds, m.form.focusField(target))
			return m, tea.Batch(cmds...)
		}
		if msg.mfa {
			m.logf(logInfo, "garmin requires a verification code")
			m.screen = screenMFA
			m.status = ""
			m.mfaInput.SetValue("")
			cmds = append(cmds, m.mfaInput.Focus())
			return m, tea.Batch(cmds...)
		}
		m.sessions = msg.sessions
		m.screen = screenList
		m.page = 0
		m.logf(logSuccess, "loaded %d session(s)", len(msg.sessions))
		if m.watchOpts.Enabled && m.watch == nil {
			cmds = append(cmds, m.startWatchCmd())
		}
		m.clampCursor()
		m.clampPage()

	case sessionsMsg:
		m.screen = screenList
		m.page = 0
		if msg.err != nil {
			m.status = m.th.paint(msg.err.Error(), m.th.danger)
			m.logf(logError, "refresh failed: %s", msg.err.Error())
			return m, nil
		}
		m.sessions = msg.sessions
		m.logf(logSuccess, "refreshed: %d session(s)", len(msg.sessions))
		m.clampCursor()
		m.clampPage()

	case spoofedMsg:
		if msg.err != nil {
			m.status = m.th.paint("save failed: "+msg.err.Error(), m.th.danger)
			m.logf(logError, "save spoofed %s: %v", msg.id, msg.err)
		} else {
			m.status = m.th.paint("saved "+msg.path, m.th.success)
			m.logf(logInfo, "saved spoofed %s to %s", msg.id, msg.path)
		}

	case watchStartedMsg:
		cmds = append(cmds, m.startedWatch(msg.run))

	case watchEventMsg:
		cmds = append(cmds, m.applyWatchEvent(msg.run, msg.ev))

	case watchStoppedMsg:
		m.stoppedWatch(msg.run)

	case watchFailedMsg:
		m.failedWatch(msg.err)

	case batchStepMsg:
		b := m.batch
		if b == nil {
			break
		}
		b.done++
		switch msg.res.Status {
		case app.StatusUploaded, app.StatusDuplicate:
			b.uploaded++
		case app.StatusSkipped:
			b.skipped++
		default:
			b.failed++
		}
		text := resLabel(msg.res)
		m.results[msg.act.ID] = text
		m.logf(resultLevel(msg.res), "upload %s: %s", msg.act.DisplayName(), text)
		if len(b.queue) > 0 {
			cmds = append(cmds, m.nextInBatch())
		} else {
			m.finishBatch()
		}

	case uploadMsg:
		if m.uploading > 0 {
			m.uploading--
		}
		text := resLabel(msg.res)
		if msg.id != "" {
			m.results[msg.id] = text
		}
		m.status = m.th.paint(msgPrefix(msg.id)+text, resultColor(m.th, msg.res))
		m.logf(resultLevel(msg.res), "upload %s%s", msgPrefix(msg.id), text)

	case mfaMsg:
		if msg.err != nil {
			m.status = m.th.paint("MFA failed: "+msg.err.Error(), m.th.danger)
			m.logf(logError, "MFA failed: %s", msg.err.Error())
			m.mfaInput.SetValue("")
			cmds = append(cmds, m.mfaInput.Focus())
			return m, tea.Batch(cmds...)
		}
		m.logf(logSuccess, "MFA accepted")
		m.status = ""
		return m, m.refreshCmd()

	case tea.KeyMsg:
		return m.handleKey(msg, cmds)
	}

	return m, tea.Batch(cmds...)
}

func resultLevel(res app.UploadResult) logLevel {
	switch res.Status {
	case app.StatusUploaded:
		return logSuccess
	case app.StatusDuplicate, app.StatusSkipped:
		return logInfo
	default:
		return logError
	}
}

func (m Model) handleKey(msg tea.KeyMsg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.screen {
	case screenMFA:
		switch msg.String() {
		case "esc":
			m.screen = screenList
			return m, nil
		case "enter":
			code := strings.TrimSpace(m.mfaInput.Value())
			if code == "" {
				return m, nil
			}
			m.uploading = 1
			m.uploadingName = "Garmin login"
			m.logf(logInfo, "submitting verification code")
			return m, m.mfaCmd(code)
		default:
			var cmd tea.Cmd
			m.mfaInput, cmd = m.mfaInput.Update(msg)
			return m, cmd
		}

	case screenConfirm:
		switch msg.String() {
		case "y", "enter":
			m.screen = screenList
			return m, m.startBatch()
		case "n", "esc":
			m.screen = screenList
		}
		return m, nil

	case screenSettings:
		switch msg.String() {
		case "esc":
			if len(m.form.sugg) > 0 {
				m.form.sugg = nil
				return m, nil
			}
			m.screen = screenList
			m.status = ""
			return m, nil
		case "ctrl+s":
			return m.saveSettings()
		default:
			cmd := m.form.update(msg)
			m.form.apply(&m.settings)
			return m, cmd
		}

	case screenPreview:
		switch msg.String() {
		case "esc":
			m.screen = screenList
		case "u":
			m.screen = screenList
			cmds = append(cmds, m.startUpload(m.selected()))
		}
		return m, tea.Batch(cmds...)
	}

	// screenLoading, screenList
	switch msg.String() {
	case "esc":
		if m.showHelp {
			m.showHelp = false
		} else if m.showLog {
			m.showLog = false
		}
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
	case "l":
		m.toggleLogs()
	case "m":
		m.mouseOn = !m.mouseOn
		if m.mouseOn {
			m.logf(logInfo, "mouse on: scroll and click enabled")
			cmds = append(cmds, tea.EnableMouseCellMotion)
		} else {
			m.logf(logInfo, "mouse off: select and copy text with the terminal")
			cmds = append(cmds, tea.DisableMouse)
		}
	case "up", "k":
		if m.movingLogs() {
			m.scrollLogs(1)
		} else {
			m.moveCursor(-1)
		}
	case "down", "j":
		if m.movingLogs() {
			m.scrollLogs(-1)
		} else {
			m.moveCursor(1)
		}
	case "left", "pgup":
		if !m.showHelp && !m.showLog {
			m.pageBy(-1)
		}
	case "right", "pgdown":
		if !m.showHelp && !m.showLog {
			m.pageBy(1)
		}
	case "enter":
		if !m.showHelp && !m.showLog {
			if s := m.selected(); s != nil {
				m.screen = screenPreview
			}
		}
	case "u":
		cmds = append(cmds, m.startUpload(m.selected()))
	case "a":
		if n := m.pendingCount(); n > 0 {
			m.confirm = n
			m.screen = screenConfirm
		}
	case "s":
		m.form = newSettingsForm(m.th, m.settings)
		m.form.setWidth(m.innerW())
		m.screen = screenSettings
		cmds = append(cmds, m.form.focusField(0))
	case "t":
		cmds = append(cmds, m.toggleWatch())
	case "w":
		if s := m.selected(); s != nil {
			m.logf(logInfo, "saving spoofed FIT for %s", s.ID)
			cmds = append(cmds, m.saveSpoofedCmd(s.Activity))
		}
	case "o":
		m.openSpoofDir()
	case "r":
		m.logf(logInfo, "refreshing sessions…")
		cmds = append(cmds, m.refreshCmd())
	}
	return m, tea.Batch(cmds...)
}

// handleMouse maps clicks and wheel events onto the current layout.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	inner := msg.Y - 1 // top border only; frame has no vertical padding

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.mouseInLog(msg) {
			m.scrollLogs(1)
		} else if m.screen == screenList || m.screen == screenLoading {
			m.moveCursor(-1)
		}
		return m, nil
	case tea.MouseButtonWheelDown:
		if m.mouseInLog(msg) {
			m.scrollLogs(-1)
		} else if m.screen == screenList || m.screen == screenLoading {
			m.moveCursor(1)
		}
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	var cmds []tea.Cmd
	switch m.screen {
	case screenLoading, screenList:
		if m.showLog && inner >= bodyTop+m.listHeight() {
			return m, nil
		}
		idx := inner - bodyTop
		page := m.pageSessions()
		if idx < 0 || idx >= len(page) {
			return m, nil
		}
		cursor := m.pageStart() + idx
		if cursor == m.cursor {
			if s := m.selected(); s != nil {
				m.screen = screenPreview
			}
		} else {
			m.cursor = cursor
		}
	case screenSettings:
		if i, ok := m.form.suggestionAt(inner); ok {
			m.form.suggIdx = i
			m.form.acceptSuggestion()
			m.form.apply(&m.settings)
			return m, nil
		}
		for i := range settingsFields {
			if m.form.fieldRow(i) == inner {
				cmds = append(cmds, m.form.focusField(i))
				return m, tea.Batch(cmds...)
			}
		}
	case screenConfirm:
		m.screen = screenList
		cmds = append(cmds, m.startBatch())
	}
	return m, tea.Batch(cmds...)
}

// mouseInLog reports whether the event falls on the log pane.
func (m Model) mouseInLog(msg tea.MouseMsg) bool {
	if !m.showLog || m.showHelp {
		return false
	}
	inner := msg.Y - 1
	if m.screen == screenLoading || m.screen == screenList {
		return inner >= bodyTop+m.listHeight()
	}
	return inner >= bodyTop
}

// logViewHeight is the number of scrollable rows in the log pane.
func (m Model) logViewHeight() int {
	if m.screen == screenLoading || m.screen == screenList {
		return max(1, m.logPaneHeight()-1)
	}
	return max(1, m.bodyH()-1)
}

func (m *Model) scrollLogs(delta int) {
	m.logs.scroll += delta
	m.logs.clamp(m.logViewHeight())
}

func (m Model) movingLogs() bool {
	return m.showLog && !m.showHelp
}

func (m *Model) toggleLogs() {
	m.showLog = !m.showLog
	if m.showLog {
		m.logs.scroll = 0
		m.logs.clamp(m.logViewHeight())
	}
	m.clampCursor()
	m.clampPage()
	m.ensureCursorVisible()
}

func (m *Model) moveCursor(delta int) {
	if len(m.sessions) == 0 {
		return
	}
	m.cursor += delta
	m.clampCursor()
	m.ensureCursorVisible()
}

func (m *Model) pageBy(delta int) {
	if m.pageCount() <= 1 {
		return
	}
	m.page += delta
	m.clampPage()
	m.cursor = m.pageStart()
	m.clampCursor()
}

func (m *Model) ensureCursorVisible() {
	if len(m.sessions) == 0 {
		return
	}
	ps := m.pageSize()
	if m.cursor < m.pageStart() || m.cursor >= m.pageStart()+ps {
		m.page = m.cursor / ps
	}
	m.clampPage()
}

func (m Model) saveSettings() (tea.Model, tea.Cmd) {
	m.form.apply(&m.settings)
	if err := m.settings.Validate(); err != nil {
		m.status = m.th.paint(err.Error(), m.th.danger)
		m.logf(logError, "settings: %s", err.Error())
		if i := m.settings.errorField(); i >= 0 {
			return m, m.form.focusField(i)
		}
		return m, nil
	}
	if err := m.saveFn(m.settings); err != nil {
		m.status = m.th.paint(err.Error(), m.th.danger)
		m.logf(logError, "could not save settings: %s", err.Error())
		return m, nil
	}
	m.logf(logSuccess, "settings saved")
	m.screen = screenLoading
	m.status = ""
	return m, m.bootstrapCmd()
}

func (m *Model) startUpload(s *app.Session) tea.Cmd {
	if s == nil {
		return nil
	}
	name := s.DisplayName()
	if name == "" {
		name = s.ID
	}
	if s.Uploaded {
		m.status = m.th.paint("already synced", m.th.secondary)
		m.logf(logInfo, "%s already synced", name)
		return nil
	}
	m.uploading++
	m.uploadingName = name
	m.logf(logInfo, "uploading %s", name)
	return m.uploadCmd(s.Activity)
}

// batchRun is an "upload all" in progress. Rides go one at a time, so the
// UI can show progress and the list updates as each ride finishes.
type batchRun struct {
	queue                     []app.Session
	total                     int
	done                      int
	uploaded, skipped, failed int
}

// batchStepMsg reports the outcome of one ride in a batch.
type batchStepMsg struct {
	act mywhoosh.Activity
	res app.UploadResult
}

// startBatch queues every ride not yet synced and starts the first one.
func (m *Model) startBatch() tea.Cmd {
	var queue []app.Session
	for _, s := range m.sessions {
		if !s.Uploaded {
			queue = append(queue, s)
		}
	}
	if len(queue) == 0 {
		m.status = m.th.paint("nothing to upload, all rides synced", m.th.secondary)
		return nil
	}
	m.batch = &batchRun{queue: queue, total: len(queue)}
	m.uploading++
	m.logf(logInfo, "uploading %d pending ride(s)", len(queue))
	return m.nextInBatch()
}

// nextInBatch uploads the head of the queue. The ride is removed from the
// queue here so a second batch step cannot pick it up twice.
func (m *Model) nextInBatch() tea.Cmd {
	b := m.batch
	act := b.queue[0].Activity
	b.queue = b.queue[1:]
	name := act.DisplayName()
	m.uploadingName = fmt.Sprintf("%d/%d %s", b.done+1, b.total, name)
	ctx := m.ctx
	svc := m.svc
	return func() tea.Msg {
		return batchStepMsg{act: act, res: svc.Upload(ctx, act)}
	}
}

// finishBatch clears batch state and reports the totals in the status line.
func (m *Model) finishBatch() {
	b := m.batch
	m.batch = nil
	m.uploading--
	m.uploadingName = ""
	summary := fmt.Sprintf("upload all done: %d uploaded, %d skipped, %d failed", b.uploaded, b.skipped, b.failed)
	m.status = m.th.paint(summary, m.th.secondary)
	m.logf(logInfo, "%s", summary)
}

func (m Model) selected() *app.Session {
	if len(m.sessions) == 0 || m.cursor < 0 || m.cursor >= len(m.sessions) {
		return nil
	}
	return &m.sessions[m.cursor]
}

func (m Model) pendingCount() int {
	n := 0
	for _, s := range m.sessions {
		if !s.Uploaded {
			n++
		}
	}
	return n
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.sessions) {
		m.cursor = len(m.sessions) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) clampPage() {
	pages := m.pageCount()
	switch {
	case pages <= 1:
		m.page = 0
	case m.page >= pages:
		m.page = pages - 1
	case m.page < 0:
		m.page = 0
	}
}

func (m Model) pageCount() int {
	n := len(m.sessions)
	ps := m.pageSize()
	if n == 0 || ps <= 0 {
		return 1
	}
	p := n / ps
	if n%ps != 0 {
		p++
	}
	return max(1, p)
}

func (m Model) pageStart() int { return m.page * m.pageSize() }

func (m Model) pageSize() int {
	if m.pageSizeVal > 0 {
		return m.pageSizeVal
	}
	return m.listRows()
}
