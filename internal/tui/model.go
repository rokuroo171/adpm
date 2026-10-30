package tui

import (
	"image"
	"math"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rokuroo171/adpm/internal/render"
)

type tickMsg time.Time

// model drives one ad playback.
type model struct {
	frames  []image.Image
	title   string
	total   time.Duration
	start   time.Time
	cols    int
	rows    int
	cellW   int
	cellH   int
	mode    render.Mode
	frameIx int
	result  Result
}

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
		elapsed := time.Since(m.start)
		if elapsed >= m.total {
			m.result.Finished = true
			m.result.Duration = m.total
			return m, tea.Quit
		}
		ix := int(elapsed / frameInterval)
		if ix >= len(m.frames) {
			ix = len(m.frames) - 1
		}
		m.frameIx = ix
		return m, tea.Tick(frameInterval, func(t time.Time) tea.Msg { return tickMsg(t) })

	case tea.WindowSizeMsg:
		m.cols, m.rows = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// Milestone 3 replaces this with the reward popup.
			m.result.Cancelled = true
			return m, tea.Quit
		case "ctrl+z":
			m.result.Suspended = true
			return m, tea.Quit
		}
	}
	return m, nil
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
	bar := countdownBar(remaining, m.total, m.cols)

	var content string
	if m.mode == render.ModeCompact {
		line := render.Truncate(render.CompactLine(m.title, secs), m.cols)
		content = line + "\n" + bar
	} else {
		b := m.frames[m.frameIx].Bounds()
		boxCols, boxRows := render.Fit(b.Dx(), b.Dy(),
			m.cols, max(m.rows-2, 1), m.cellW, m.cellH)
		img := render.HalfBlock(m.frames[m.frameIx], boxCols, boxRows)
		content = img + "\n" + bar
	}
	return lipgloss.Place(m.cols, m.rows, lipgloss.Center, lipgloss.Center, content)
}
