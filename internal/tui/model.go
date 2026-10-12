// Package tui implements the Bubble Tea interface for whoosh2connect.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/mywhoosh"
)

// Service is the slice of app.Service the TUI needs.
type Service interface {
	LoginMyWhoosh(ctx context.Context, email, password string) error
	LoginGarmin(ctx context.Context, email, password string) error
	SubmitMFA(ctx context.Context, code string) error
	Sessions(ctx context.Context) ([]app.Session, error)
	Upload(ctx context.Context, act mywhoosh.Activity) app.UploadResult
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
}

// Model is the Bubble Tea state machine.
type Model struct {
	svc            Service
	settings       Settings
	saveFn         func(Settings) error
	th             theme
	screen         screen
	cursor         int
	settingsCursor int
	confirm        int
	sessions       []app.Session
	results        map[string]string
	status         string
	mfaInput       textinput.Model
	mfaMethod      string
	uploading      int
	uploadingName  string
	showHelp       bool
	width          int
	height         int
	spinner        spinner.Model
	page           int
	pageSizeVal    int
}

func (m Model) pageSize() int {
	if m.pageSizeVal > 0 {
		return m.pageSizeVal
	}
	return 10
}

func (m *Model) clampPage() {
	max := m.pageCount()
	if max <= 0 {
		m.page = 0
		return
	}
	if m.page >= max {
		m.page = max - 1
	}
	if m.page < 0 {
		m.page = 0
	}
}

func (m Model) pageCount() int {
	n := len(m.sessions)
	ps := m.pageSize()
	if n == 0 {
		return 1
	}
	p := n / ps
	if n%ps != 0 {
		p++
	}
	return p
}

func (m Model) pageStart() int {
	return m.page * m.pageSize()
}

type keymap struct {
	up        key.Binding
	down      key.Binding
	enter     key.Binding
	upload    key.Binding
	uploadAll key.Binding
	settings  key.Binding
	refresh   key.Binding
	help      key.Binding
	quit      key.Binding
}

var keys = keymap{
	up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "upload")),
	upload:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "upload")),
	uploadAll: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "upload all")),
	settings:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "settings")),
	refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// New creates a Model.
func New(opts Options) Model {
	th := darkTheme
	if opts.Dark {
		th = lightTheme
	}
	m := Model{
		svc:      opts.Service,
		settings: opts.Settings,
		saveFn:   func(Settings) error { return nil },
		th:       th,
		results:  make(map[string]string),
		screen:   screenLoading,
	}
	if opts.SaveSettings != nil {
		m.saveFn = opts.SaveSettings
	}
	m.spinner = spinner.New()
	m.spinner.Spinner = spinner.Dot
	m.mfaInput = textinput.New()
	m.mfaInput.Placeholder = "6-digit code"
	m.mfaInput.Prompt = " MFA code: "
	m.mfaInput.CharLimit = 12
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

type mfaMsg struct{ err error }

func (m Model) bootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
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
		sessions, err := m.svc.Sessions(context.Background())
		return sessionsMsg{sessions: sessions, err: err}
	}
}

func (m Model) uploadCmd(act mywhoosh.Activity) tea.Cmd {
	return func() tea.Msg {
		res := m.svc.Upload(context.Background(), act)
		return uploadMsg{id: act.ID, res: res}
	}
}

func (m Model) mfaCmd(code string) tea.Cmd {
	return func() tea.Msg {
		return mfaMsg{err: m.svc.SubmitMFA(context.Background(), code)}
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
		m.width = msg.Width
		m.height = msg.Height

	case bootstrapMsg:
		if msg.err != nil {
			m.status = m.th.paint(msg.err.Error(), m.th.danger)
			m.screen = screenSettings
			return m, nil
		}
		if msg.mfa {
			m.screen = screenMFA
			cmds = append(cmds, m.mfaInput.Focus())
			return m, tea.Batch(cmds...)
		}
		m.sessions = msg.sessions
		m.screen = screenList
		m.page = 0
		m.clampCursor()
		m.clampPage()

	case sessionsMsg:
		m.screen = screenList
		m.page = 0
		if msg.err != nil {
			m.status = m.th.paint(msg.err.Error(), m.th.danger)
			return m, nil
		}
		m.sessions = msg.sessions
		m.clampCursor()
		m.clampPage()
		m.clampPage()

	case uploadMsg:
		if m.uploading > 0 {
			m.uploading--
		}
		text := resLabel(msg.res)
		if msg.id != "" {
			m.results[msg.id] = text
		}
		m.status = m.th.paint(msgPrefix(msg.id)+text, resultColor(m.th, msg.res))

	case mfaMsg:
		if msg.err != nil {
			m.status = m.th.paint("MFA failed: "+msg.err.Error(), m.th.danger)
			cmds = append(cmds, m.mfaInput.Focus())
			return m, tea.Batch(cmds...)
		}
		m.status = ""
		return m, m.refreshCmd()

	case tea.KeyMsg:
		return m.handleKey(msg, cmds)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.sessions) {
		m.cursor = len(m.sessions) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) handleKey(msg tea.KeyMsg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMFA:
		switch {
		case msg.String() == "enter":
			code := strings.TrimSpace(m.mfaInput.Value())
			if code == "" {
				return m, nil
			}
			m.uploading = 1
			m.uploadingName = "Garmin login"
			m.uploadingName = "Garmin login"
			return m, m.mfaCmd(code)
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			m.screen = screenList
			return m, nil
		default:
			var cmd tea.Cmd
			m.mfaInput, cmd = m.mfaInput.Update(msg)
			return m, cmd
		}

	case screenConfirm:
		switch {
		case msg.String() == "y" || msg.String() == "enter":
			m.screen = screenList
			return m, m.uploadAllCmd()
		case msg.String() == "n" || msg.String() == "esc" || msg.String() == "q":
			m.screen = screenList
			return m, nil
		default:
			return m, nil
		}

	case screenSettings:
		switch {
		case msg.String() == "ctrl+s":
			return m.saveSettings()
		case msg.String() == "tab" || msg.String() == "down" || msg.String() == "enter":
			m.settingsCursor = (m.settingsCursor + 1) % len(settingsFields)
			return m, nil
		case msg.String() == "up":
			m.settingsCursor = (m.settingsCursor - 1 + len(settingsFields)) % len(settingsFields)
			return m, nil
		case msg.String() == "esc" || msg.String() == "q":
			m.screen = screenList
			return m, nil
		case msg.String() == "backspace":
			v := fieldValue(m.settings, m.settingsCursor)
			if len(v) > 0 {
				m.settings.setField(m.settingsCursor, v[:len(v)-1])
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.settings.setField(m.settingsCursor, fieldValue(m.settings, m.settingsCursor)+string(msg.Runes))
			}
			return m, nil
		}

	case screenPreview:
		switch {
		case msg.String() == "esc" || msg.String() == "q" || key.Matches(msg, keys.enter):
			m.screen = screenList
			return m, nil
		case key.Matches(msg, keys.upload):
			s := m.selected()
			if s != nil {
				m.uploading++
				m.uploadingName = s.DisplayName()
				if m.uploadingName == "" {
					m.uploadingName = s.ID
				}
				m.screen = screenList
				cmds = append(cmds, m.uploadCmd(s.Activity))
			}
		}
		return m, tea.Batch(cmds...)

	case screenList:
		switch {
		case msg.String() == "esc":
			m.screen = screenList
			return m, nil
		case key.Matches(msg, keys.quit):
			return m, tea.Quit
		case key.Matches(msg, keys.up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, keys.down):
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		case key.Matches(msg, keys.enter):
			s := m.selected()
			if s != nil {
				m.screen = screenPreview
				return m, nil
			}
		case key.Matches(msg, keys.upload):
			if s := m.selected(); s != nil {
				m.uploading++
				m.uploadingName = s.DisplayName()
				if m.uploadingName == "" {
					m.uploadingName = s.ID
				}
				cmds = append(cmds, m.uploadCmd(s.Activity))
			}
		case key.Matches(msg, keys.uploadAll):
			if m.pendingCount() > 0 {
				m.confirm = m.pendingCount()
				m.screen = screenConfirm
			}
		case key.Matches(msg, keys.settings):
			m.screen = screenSettings
		case key.Matches(msg, keys.refresh):
			cmds = append(cmds, m.refreshCmd())
		case key.Matches(msg, keys.help):
			m.showHelp = !m.showHelp
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) saveSettings() (tea.Model, tea.Cmd) {
	if err := m.settings.Validate(); err != nil {
		m.status = m.th.paint(err.Error(), m.th.danger)
		return m, nil
	}
	if err := m.saveFn(m.settings); err != nil {
		m.status = m.th.paint(err.Error(), m.th.danger)
		return m, nil
	}
	m.screen = screenLoading
	m.status = ""
	return m, m.bootstrapCmd()
}

func (m Model) uploadAllCmd() tea.Cmd {
	acts := make([]mywhoosh.Activity, 0)
	for _, s := range m.sessions {
		if !s.Uploaded {
			acts = append(acts, s.Activity)
		}
	}
	n := len(acts)
	return func() tea.Msg {
		var last app.UploadResult
		for i := 0; i < n; i++ {
			last = m.svc.Upload(context.Background(), acts[i])
		}
		return uploadMsg{id: "batch", res: last}
	}
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

// View renders the current screen.
func (m Model) View() string {
	var body string
	switch m.screen {
	case screenLoading, screenList:
		body = m.renderList()
	case screenPreview:
		body = m.renderPreview()
	case screenSettings:
		body = renderSettings(m.th, m.settings, m.settingsCursor, m.contentW())
	case screenMFA:
		body = m.renderMFA()
	case screenConfirm:
		body = m.renderConfirm()
	}

	inner := strings.Join([]string{m.renderHeader(), m.renderStats(), body, m.renderFooter()}, "\n")
	frame := m.th.panel(m.winW() - 2)
	return frame.Render(inner)
}
