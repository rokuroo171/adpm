// Package tui is the ad player: a fullscreen alt-screen Bubble Tea program
// that plays baked frames with the half-block renderer.
package tui

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/rokuroo171/adpm/internal/render"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// frameInterval gives the spec's ~10 fps playback.
const frameInterval = 100 * time.Millisecond

// Options describe the ad to play.
type Options struct {
	Frames        []image.Image
	Title         string
	RewardLines   []string
	ReportMessage string
}

// Result reports how the ad ended.
type Result struct {
	Duration  time.Duration
	Finished  bool // watched to the end
	Cancelled bool // user exited early (popup flow lands in Milestone 3)
	Suspended bool // Ctrl+Z: terminal restored, process re-stopped
}

// Play runs the ad player and always leaves the terminal usable, including on
// panic. It returns only after cleanup has run.
func Play(opts Options) (Result, error) {
	res, err := withCleanup(func() (Result, error) { return play(opts) })
	if err != nil {
		return res, err
	}
	if res.Finished {
		fmt.Fprintf(os.Stdout, "[adpm] ad watched (%ds). Thank you for your support.\n",
			int(res.Duration.Seconds()))
	}
	if res.Suspended {
		// Terminal is already restored; hand the stop back to the shell.
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGTSTP)
	}
	return res, nil
}

// withCleanup saves the terminal state first and guarantees it is put back
// before returning, converting a panic into an error so the install proceeds.
func withCleanup(fn func() (Result, error)) (res Result, err error) {
	fd := int(os.Stdin.Fd())
	state, _ := term.GetState(fd) // nil when stdin is not a terminal
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		// Leave the alt screen and show the cursor no matter what happened.
		fmt.Fprint(os.Stdout, "\x1b[?1049l\x1b[?25h")
		if state != nil {
			_ = term.Restore(fd, state)
		}
	}
	defer func() {
		restore()
		if r := recover(); r != nil {
			err = fmt.Errorf("adpm: player panic: %v", r)
		}
	}()

	res, err = fn()
	restore() // terminal back before any scrollback output
	return res, err
}

func play(opts Options) (Result, error) {
	if len(opts.Frames) == 0 {
		return Result{}, errors.New("adpm: no frames to play")
	}
	total := time.Duration(len(opts.Frames)) * frameInterval
	cols, rows, cellW, cellH := winSize()

	if render.ModeFor(cols, rows) == render.ModeOneLine {
		return playOneLine(opts, cols, total)
	}
	return playTUI(opts, total, cols, rows, cellW, cellH)
}

func playTUI(opts Options, total time.Duration, cols, rows, cellW, cellH int) (Result, error) {
	m := &model{
		frames:        opts.Frames,
		title:         opts.Title,
		rewardLines:   opts.RewardLines,
		reportMessage: opts.ReportMessage,
		total:         total,
		start:         time.Now(),
		cols:          cols,
		rows:          rows,
		cellW:         cellW,
		cellH:         cellH,
	}
	m.resize()

	p := tea.NewProgram(m, tea.WithAltScreen())

	// External signals (kill, kill -INT) must reach the cleanup path too;
	// Ctrl+C arrives as a key because the terminal runs in raw mode.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		if _, ok := <-sigCh; ok {
			p.Quit()
		}
	}()

	if _, err := p.Run(); err != nil {
		return m.result, fmt.Errorf("adpm: player: %w", err)
	}
	return m.result, nil
}

// playOneLine handles terminals under ~20x6: one plain line plus countdown,
// no alt screen and no Bubble Tea.
func playOneLine(opts Options, cols int, total time.Duration) (Result, error) {
	m := &model{
		title: opts.Title, total: total, start: time.Now(),
		rewardLines: opts.RewardLines, reportMessage: opts.ReportMessage,
	}
	keys, restoreInput := lineInput()
	defer restoreInput()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		now := time.Now()
		if m.popup == popupNone && now.Sub(m.start) >= total {
			fmt.Fprintln(os.Stdout)
			return Result{Finished: true, Duration: total}, nil
		}
		if m.popup == popupNone && m.message != "" && now.After(m.messageUntil) {
			m.message = ""
		}
		line := oneLineView(m, cols, now)
		fmt.Fprintf(os.Stdout, "\r\x1b[2K%s", line)
		select {
		case sig := <-sigCh:
			if sig == syscall.SIGINT {
				m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			} else {
				fmt.Fprintln(os.Stdout)
				return Result{}, nil
			}
		case key, ok := <-keys:
			if !ok {
				keys = nil
				continue
			}
			m.Update(key)
			if m.result.Cancelled || m.result.Finished || m.result.Suspended {
				fmt.Fprintln(os.Stdout)
				return m.result, nil
			}
		case <-ticker.C:
		}
	}
}

func oneLineView(m *model, cols int, now time.Time) string {
	if m.popup == popupReward {
		line := "You won't get your reward. Enter: Cancel, Tab: OK"
		if len(m.rewardLines) > 0 {
			line = m.rewardLines[(m.rewardIndex-1)%len(m.rewardLines)] + " Enter: Cancel, Tab: OK"
		}
		if m.choice == 1 {
			line = strings.Replace(line, "Enter: Cancel, Tab: OK", "Tab: Cancel, Enter: OK", 1)
		}
		return render.Truncate(line, cols)
	}
	if m.popup == popupReport {
		message := m.reportMessage
		if message == "" {
			message = "Thank you. Your report will be reviewed in 3-5 business years."
		}
		return render.Truncate(message+" Enter: restart", cols)
	}
	remaining := m.total - now.Sub(m.start)
	secs := int(math.Ceil(remaining.Seconds()))
	line := fmt.Sprintf("AD · %s · %ds · Skip: Enter", m.title, secs)
	if now.Sub(m.start) < skipAfter(m.total) {
		wait := int(math.Ceil((skipAfter(m.total) - now.Sub(m.start)).Seconds()))
		line = fmt.Sprintf("AD · %s · %ds · Skip in %d", m.title, secs, wait)
	}
	if m.message != "" {
		line = m.message
	}
	return render.Truncate(line, cols)
}

func lineInput() (<-chan tea.KeyMsg, func()) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, func() {}
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, func() {}
	}
	keys := make(chan tea.KeyMsg, 8)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			b, err := reader.ReadByte()
			if err != nil {
				close(keys)
				return
			}
			var key tea.KeyMsg
			switch b {
			case 3:
				key = tea.KeyMsg{Type: tea.KeyCtrlC}
			case 26:
				key = tea.KeyMsg{Type: tea.KeyCtrlZ}
			case '\r', '\n':
				key = tea.KeyMsg{Type: tea.KeyEnter}
			case '\t':
				key = tea.KeyMsg{Type: tea.KeyTab}
			case 'r':
				key = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
			case 27:
				sequence := make([]byte, 2)
				if _, err := io.ReadFull(reader, sequence); err != nil || sequence[0] != '[' {
					continue
				}
				if sequence[1] == 'D' {
					key = tea.KeyMsg{Type: tea.KeyLeft}
				} else if sequence[1] == 'C' {
					key = tea.KeyMsg{Type: tea.KeyRight}
				}
			default:
				continue
			}
			keys <- key
		}
	}()
	return keys, func() { _ = term.Restore(fd, state) }
}

// winSize reads the terminal geometry. Cell pixel size falls back to 1:2
// (handled by render.Fit) when the terminal reports none.
func winSize() (cols, rows, cellW, cellH int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws == nil || ws.Col == 0 || ws.Row == 0 {
		return 80, 24, 0, 0
	}
	cols, rows = int(ws.Col), int(ws.Row)
	if ws.Xpixel > 0 {
		cellW = int(ws.Xpixel) / cols
	}
	if ws.Ypixel > 0 {
		cellH = int(ws.Ypixel) / rows
	}
	return cols, rows, cellW, cellH
}

// countdownBar renders the remaining time as a bar plus seconds.
func countdownBar(remaining, total time.Duration, cols int) string {
	width := min(20, max(cols-10, 4))
	filled := width
	if total > 0 {
		filled = int(float64(remaining)/float64(total)*float64(width) + 0.5)
	}
	filled = max(min(filled, width), 0)
	secs := int(math.Ceil(remaining.Seconds()))
	return fmt.Sprintf("[%s%s] %ds",
		strings.Repeat("#", filled), strings.Repeat("-", width-filled), secs)
}
