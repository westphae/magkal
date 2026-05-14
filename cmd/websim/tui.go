package main

import (
	"fmt"
	"io"
	stdlog "log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/westphae/magkal/pkg/kalman"
)

// tui renders a stable, in-place status block plus a scrolling messages
// pane to stdout using bare ANSI escape sequences. The point is to keep
// the per-step EKF stream from scrolling useful state off the screen.
//
// When stdout is not a TTY (piped/redirected), the TUI is disabled and
// Logf falls through to the standard logger so log lines still reach the
// pipe.
//
// pkg/kalman has noisy debug log.Printf calls in its update path that
// would otherwise scribble over the fixed pane; when the TUI is enabled
// we route the global logger to io.Discard. cmd/websim's own status
// messages flow through tui.Logf to the messages pane instead.
type tui struct {
	mu          sync.Mutex
	listenAddr  string
	connections int
	status      statusSnapshot
	messages    []tuiMessage
	msgCap      int
	out         io.Writer
	enabled     bool
	quit        chan struct{}
}

// statusSnapshot is the data model for the fixed status block. Updated
// under tui.mu from connection goroutines and rendered on a ticker.
type statusSnapshot struct {
	have       bool
	source     string
	mode       string
	n          int
	n0         float64
	sigmaK0    float64
	sigmaK     float64
	sigmaM     float64
	k, l       []float64
	sigK, sigL []float64
	nis        float64
	nisValid   bool
	converged  bool
	lastM      []float64
	steps      int
}

type tuiMessage struct {
	t   time.Time
	msg string
}

const (
	ansiHideCursor      = "\x1b[?25l"
	ansiShowCursor      = "\x1b[?25h"
	ansiClearScreen     = "\x1b[2J"
	ansiHome            = "\x1b[H"
	ansiClearFromCursor = "\x1b[J"
)

var ui *tui

func initTUI(listenAddr string) {
	ui = &tui{
		listenAddr: listenAddr,
		out:        os.Stdout,
		msgCap:     12,
		quit:       make(chan struct{}),
	}
	fi, err := os.Stdout.Stat()
	if err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		ui.enabled = true
		// Mute pkg/kalman's per-step debug stream so it doesn't clobber
		// the fixed pane. cmd/websim's own status messages go through
		// tui.Logf instead of the stdlib logger.
		stdlog.SetOutput(io.Discard)
		fmt.Fprint(ui.out, ansiHideCursor+ansiClearScreen+ansiHome)
		go ui.renderLoop()
	}
}

func stopTUI() {
	if ui == nil || !ui.enabled {
		return
	}
	close(ui.quit)
	fmt.Fprint(ui.out, ansiShowCursor+"\n")
}

func (t *tui) renderLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.quit:
			return
		case <-ticker.C:
			t.render()
		}
	}
}

func (t *tui) render() {
	t.mu.Lock()
	defer t.mu.Unlock()

	var b strings.Builder
	b.WriteString(ansiHome)
	b.WriteString(ansiClearFromCursor)

	now := time.Now().Format("15:04:05")
	fmt.Fprintf(&b, "magkal websim                                  [connected: %d]\n", t.connections)
	fmt.Fprintf(&b, "listening %-30s    %s\n\n", t.listenAddr, now)

	s := t.status
	if !s.have {
		b.WriteString(" (no client activity yet)\n")
	} else {
		nisStr := "—"
		if s.nisValid {
			nisStr = fmt.Sprintf("%.2f", s.nis)
		}
		convStr := "no"
		if s.converged {
			convStr = "yes"
		}
		fmt.Fprintf(&b, " source : %-15s mode  : %s\n", s.source, s.mode)
		fmt.Fprintf(&b, " n / n0 : %d / %-7g       NIS   : %-5s  conv: %s\n", s.n, s.n0, nisStr, convStr)
		fmt.Fprintf(&b, " sigmas : K0=%g K=%g M=%g\n\n", s.sigmaK0, s.sigmaK, s.sigmaM)

		b.WriteString(" axis    k         l         sigma_k    sigma_l\n")
		for i := 0; i < s.n; i++ {
			fmt.Fprintf(&b, "   %d    %7.4f  %8.2f     %7.4f    %7.4f\n",
				i, getf(s.k, i), getf(s.l, i), getf(s.sigK, i), getf(s.sigL, i))
		}
		b.WriteString("\n m last :")
		if len(s.lastM) == 0 {
			b.WriteString("   —\n")
		} else {
			for i := 0; i < s.n; i++ {
				fmt.Fprintf(&b, "  %7.2f", getf(s.lastM, i))
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, " steps  : %d\n", s.steps)
	}

	b.WriteString("\n-- messages -------------------------------------------------\n")
	for _, m := range t.messages {
		fmt.Fprintf(&b, "%s %s\n", m.t.Format("15:04:05"), m.msg)
	}

	fmt.Fprint(t.out, b.String())
}

func getf(s []float64, i int) float64 {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// Logf appends a message to the scrolling pane. Safe to call from any
// goroutine; falls back to the standard logger when the TUI is disabled.
func (t *tui) Logf(format string, args ...any) {
	if t == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if !t.enabled {
		stdlog.Print(msg)
		return
	}
	t.mu.Lock()
	t.messages = append(t.messages, tuiMessage{t: time.Now(), msg: msg})
	if len(t.messages) > t.msgCap {
		t.messages = t.messages[len(t.messages)-t.msgCap:]
	}
	t.mu.Unlock()
}

func (t *tui) IncConnections(d int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.connections += d
	if t.connections < 0 {
		t.connections = 0
	}
	t.mu.Unlock()
}

// PushFilterState captures the current filter state into the status
// snapshot. lastM may be nil; steps is the count of EKF Z-updates this
// connection has driven so far.
func (t *tui) PushFilterState(src source, kf *kalman.Filter, modeOverride string, p params, lastM []float64, steps int) {
	if t == nil {
		return
	}
	pmat := kf.P()
	n := len(kf.K())
	sigK := make([]float64, n)
	sigL := make([]float64, n)
	for i := 0; i < n; i++ {
		sigK[i] = math.Sqrt(pmat[2*i][2*i])
		sigL[i] = math.Sqrt(pmat[2*i+1][2*i+1])
	}
	nis := kf.NIS()
	nisValid := !math.IsNaN(nis)
	mode := modeOverride
	if mode == "" {
		mode = kf.Mode().String()
	}

	t.mu.Lock()
	t.status.have = true
	t.status.source = sourceName(src)
	t.status.mode = mode
	t.status.n = p.N
	t.status.n0 = p.N0
	t.status.sigmaK0 = p.SigmaK0
	t.status.sigmaK = p.SigmaK
	t.status.sigmaM = p.SigmaM
	t.status.k = append(t.status.k[:0], kf.K()...)
	t.status.l = append(t.status.l[:0], kf.L()...)
	t.status.sigK = sigK
	t.status.sigL = sigL
	t.status.nis = nis
	t.status.nisValid = nisValid
	t.status.converged = kf.Converged()
	if lastM != nil {
		t.status.lastM = append(t.status.lastM[:0], lastM...)
	}
	t.status.steps = steps
	t.mu.Unlock()
}

func sourceName(s source) string {
	switch s {
	case manual:
		return "manual"
	case random:
		return "random"
	case file:
		return "file"
	case actual:
		return "actual"
	case scenarioSrc:
		return "scenario"
	}
	return "?"
}
