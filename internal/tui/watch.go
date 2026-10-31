package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/notify"
)

// WatchOptions controls auto-watch of the local MyWhoosh ride folder.
type WatchOptions struct {
	// Enabled is the saved state; the watch starts with the TUI when true.
	Enabled bool
	// Dir is the folder to watch. Empty means NewScanner picks the default.
	Dir      string
	Interval time.Duration
	Notifier notify.Notifier
	// NewScanner builds the folder scanner. It runs off the Update loop.
	NewScanner func(dir string) (app.RideScanner, error)
	// Save persists the toggle state.
	Save func(enabled bool) error
}

// watchRun is one running watch loop. Messages carry it so that events from a
// stopped loop are ignored.
type watchRun struct {
	cancel context.CancelFunc
	ch     <-chan app.WatchEvent
}

type watchStartedMsg struct{ run *watchRun }

type watchEventMsg struct {
	run *watchRun
	ev  app.WatchEvent
}

type watchStoppedMsg struct{ run *watchRun }

type watchFailedMsg struct{ err error }

func (m Model) startWatchCmd() tea.Cmd {
	ctx := m.ctx
	svc := m.svc
	opts := m.watchOpts
	return func() tea.Msg {
		sc, err := opts.NewScanner(opts.Dir)
		if err != nil {
			return watchFailedMsg{err: err}
		}
		runCtx, cancel := context.WithCancel(ctx)
		ch := make(chan app.WatchEvent)
		go svc.Watch(runCtx, sc, opts.Interval, opts.Notifier, ch)
		return watchStartedMsg{run: &watchRun{cancel: cancel, ch: ch}}
	}
}

// nextWatchEvent waits for one event. Update re-arms it after each event.
func nextWatchEvent(run *watchRun) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-run.ch
		if !ok {
			return watchStoppedMsg{run: run}
		}
		return watchEventMsg{run: run, ev: ev}
	}
}

func (m *Model) toggleWatch() tea.Cmd {
	if m.watch != nil {
		m.stopWatch()
		m.saveWatch(false)
		m.logf(logInfo, "auto-watch off")
		return nil
	}
	m.saveWatch(true)
	m.logf(logInfo, "auto-watch on")
	return m.startWatchCmd()
}

// stopWatch cancels the running loop, if any.
func (m *Model) stopWatch() {
	if m.watch == nil {
		return
	}
	if m.watch.cancel != nil {
		m.watch.cancel()
	}
	m.watch = nil
}

func (m *Model) saveWatch(on bool) {
	if m.watchOpts.Save == nil {
		return
	}
	if err := m.watchOpts.Save(on); err != nil {
		m.logf(logError, "save auto-watch: %v", err)
	}
}

func (m *Model) startedWatch(run *watchRun) tea.Cmd {
	if m.watch != nil {
		if run.cancel != nil {
			run.cancel()
		}
		return nil
	}
	m.watch = run
	return nextWatchEvent(run)
}

func (m *Model) applyWatchEvent(run *watchRun, ev app.WatchEvent) tea.Cmd {
	if run != m.watch {
		return nil
	}
	if ev.Err != nil {
		m.logf(logError, "auto-watch stopped: %v", ev.Err)
		m.status = m.th.paint("auto-watch stopped: "+ev.Err.Error(), m.th.danger)
		m.stopWatch()
		return nil
	}
	text := ev.File + ": " + ev.Result.DescribeUpload()
	m.logf(resultLevel(ev.Result), "auto %s", text)
	m.status = m.th.paint(text, m.th.text)
	return nextWatchEvent(run)
}

func (m *Model) stoppedWatch(run *watchRun) {
	if run == m.watch {
		m.watch = nil
		m.logf(logInfo, "auto-watch stopped")
	}
}

func (m *Model) failedWatch(err error) {
	m.logf(logError, "auto-watch: %v", err)
	m.status = m.th.paint(fmt.Sprintf("auto-watch: %v", err), m.th.danger)
}
