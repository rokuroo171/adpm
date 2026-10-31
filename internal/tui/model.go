package tui

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rokuroo171/adpm/internal/render"
)

type tickMsg time.Time

// model drives one ad playback.
type model struct {
	frames        []image.Image
	title         string
	total         time.Duration
	start         time.Time
	cols          int
	rows          int
	cellW         int
	cellH         int
	mode          render.Mode
	frameIx       int
	popup         popupKind
	choice        int
	paused        time.Time
	message       string
	messageUntil  time.Time
	rewardLines   []string
	reportMessage string
	rewardIndex   int
	result        Result
	typo          bool
}

type popupKind uint8

const (
	popupNone popupKind = iota
	popupReward
	popupReport
)

func (m *model) Init() tea.Cmd {
	return tea.Tick(frameInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// resize recomputes the layout tier for the current terminal size. Once the
// TUI is up there is no way back to the no-TUI one-line tier.
func (m *model) resize() {
	m.mode = render.ModeFor(m.cols, m.rows)
	if m.mode == render.ModeOneLine {
		m.mode = render.ModeCompact
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.popup != popupNone {
			return m, tea.Tick(frameInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
		}
		if m.message != "" && time.Now().After(m.messageUntil) {
			m.message = ""
		}
		elapsed := time.Since(m.start)
		if elapsed >= m.total {
			m.result.Finished = true
			m.result.Duration = m.total
			return m, tea.Quit
		}
		ix := int(elapsed / frameInterval)
		if m.typo {
			ix %= len(m.frames)
		} else if ix >= len(m.frames) {
			ix = len(m.frames) - 1
		}
		m.frameIx = ix
		return m, tea.Tick(frameInterval, func(t time.Time) tea.Msg { return tickMsg(t) })

	case tea.WindowSizeMsg:
		m.cols, m.rows = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tea.KeyMsg:
		if m.typo {
			if msg.String() == "ctrl+c" {
				m.message = "Nice try."
				m.messageUntil = time.Now().Add(2 * time.Second)
			}
			return m, nil
		}
		if m.popup != popupNone {
			return m.updatePopup(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			m.popup = popupReward
			m.choice = 0
			m.paused = time.Now()
			m.rewardIndex++
		case "r":
			m.popup = popupReport
			m.paused = time.Now()
		case "ctrl+z":
			m.result.Suspended = true
			return m, tea.Quit
		case "enter":
			if time.Since(m.start) >= skipAfter(m.total) {
				m.result.Finished = true
				m.result.Duration = time.Since(m.start)
				return m, tea.Quit
			}
			m.start = time.Now()
			m.frameIx = 0
			m.message = "Skipping detected. Ad restarted."
			m.messageUntil = m.start.Add(2 * time.Second)
		}
	}
	return m, nil
}

func (m *model) updatePopup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.popup == popupReward {
		switch msg.String() {
		case "ctrl+c":
			return m, nil
		case "left", "right", "tab":
			m.choice = 1 - m.choice
		case "enter":
			if m.choice == 0 {
				m.result.Cancelled = true
				return m, tea.Quit
			}
			m.start = m.start.Add(time.Since(m.paused))
			m.popup = popupNone
			m.message = ""
		}
		return m, nil
	}
	if msg.String() == "enter" {
		m.start = time.Now()
		m.frameIx = 0
		m.popup = popupNone
		m.message = ""
	}
	return m, nil
}

func skipAfter(total time.Duration) time.Duration {
	delay := total * 4 / 10
	if delay < 2*time.Second {
		return 2 * time.Second
	}
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

func (m *model) View() string {
	if m.cols < 1 || m.rows < 1 {
		return ""
	}
	remaining := m.total - time.Since(m.start)
	if remaining < 0 {
		remaining = 0
	}
	secs := int(math.Ceil(remaining.Seconds()))
	bar := render.Truncate(countdownBar(remaining, m.total, m.cols), m.cols)

	var content string
	if m.mode == render.ModeCompact {
		line := fmt.Sprintf("AD · %s · %ds · Skip: Enter", m.title, secs)
		if m.typo {
			line = fmt.Sprintf("AD · %s · %ds · lockout", m.title, secs)
		} else if time.Since(m.start) < skipAfter(m.total) {
			line = fmt.Sprintf("AD · %s · %ds · Skip in %d", m.title, secs,
				int(math.Ceil((skipAfter(m.total) - time.Since(m.start)).Seconds())))
		}
		content = render.Truncate(line, m.cols) + "\n" + bar
	} else {
		b := m.frames[m.frameIx].Bounds()
		boxCols, boxRows := render.Fit(b.Dx(), b.Dy(),
			m.cols, max(m.rows-3, 1), m.cellW, m.cellH)
		img := render.HalfBlock(m.frames[m.frameIx], boxCols, boxRows)
		controlsText := "[Enter] skip · [Ctrl+C] reward · [r] report"
		if m.typo {
			controlsText = "Unskippable ad"
		} else if time.Since(m.start) < skipAfter(m.total) {
			wait := int(math.Ceil((skipAfter(m.total) - time.Since(m.start)).Seconds()))
			controlsText = fmt.Sprintf("[Enter] Skip in %ds · [Ctrl+C] reward · [r] report", wait)
		}
		if m.message != "" {
			controlsText = m.message
		}
		controls := render.Truncate(controlsText, m.cols)
		content = img + "\n" + bar + "\n" + controls
	}
	if m.message != "" && m.mode == render.ModeCompact {
		content += "\n" + render.Truncate(m.message, m.cols)
	}
	if m.popup != popupNone {
		return m.popupView()
	}
	return lipgloss.Place(m.cols, m.rows, lipgloss.Center, lipgloss.Center, content)
}

func (m *model) popupView() string {
	var text string
	if m.popup == popupReward {
		line := "You won't get your reward if you skip this ad."
		if len(m.rewardLines) > 0 {
			line = m.rewardLines[(m.rewardIndex-1)%len(m.rewardLines)]
		}
		cancel, ok := "Cancel", "OK"
		if m.choice == 0 {
			cancel = "[Cancel]"
		} else {
			ok = "[OK]"
		}
		text = line + "\n\n" + cancel + "    " + ok
	} else {
		text = m.reportMessage
		if text == "" {
			text = "Thank you. Your report will be reviewed in 3-5 business years."
		}
		text += "\n\nPress Enter to restart the ad."
	}
	if m.cols < 24 || m.rows < 10 {
		if m.popup == popupReward {
			choice := "Enter: Cancel"
			if m.choice == 1 {
				choice = "Enter: OK"
			}
			text = strings.SplitN(text, "\n\n", 2)[0] + "\n" + choice
		} else {
			text = strings.SplitN(text, "\n\n", 2)[0]
		}
		return lipgloss.Place(m.cols, m.rows, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Width(m.cols).Render(text))
	}
	width := min(54, max(min(60, m.cols)-6, 1))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width).Render(text)
	return lipgloss.Place(m.cols, m.rows, lipgloss.Center, lipgloss.Center, box)
}
