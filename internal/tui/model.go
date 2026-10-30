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
type chromeTickMsg time.Time

const chromeInterval = time.Second / 30

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
	tier          render.Tier
	trueColor     bool
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
	graphicValid  bool
	graphicFrame  int
	graphicTier   render.Tier
	graphicData   string
}

type popupKind uint8

const (
	popupNone popupKind = iota
	popupReward
	popupReport
)

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(frameInterval, func(t time.Time) tea.Msg { return tickMsg(t) }),
		tea.Tick(chromeInterval, func(t time.Time) tea.Msg { return chromeTickMsg(t) }),
	)
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
		elapsed := m.elapsedAt(time.Now())
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

	case chromeTickMsg:
		return m, tea.Tick(chromeInterval, func(t time.Time) tea.Msg { return chromeTickMsg(t) })

	case tea.WindowSizeMsg:
		m.cols, m.rows = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tea.KeyMsg:
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
			if m.elapsedAt(time.Now()) >= skipAfter(m.total) {
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
			m.paused = time.Time{}
			m.popup = popupNone
			m.message = ""
		}
		return m, nil
	}
	if msg.String() == "enter" {
		m.start = time.Now()
		m.paused = time.Time{}
		m.frameIx = 0
		m.popup = popupNone
		m.message = ""
	}
	return m, nil
}

func (m *model) elapsedAt(now time.Time) time.Duration {
	if m.popup != popupNone && !m.paused.IsZero() {
		now = m.paused
	}
	elapsed := now.Sub(m.start)
	if elapsed < 0 {
		return 0
	}
	return elapsed
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
	elapsed := m.elapsedAt(time.Now())
	remaining := m.total - elapsed
	if remaining < 0 {
		remaining = 0
	}
	var content string
	var graphicPayload, graphicPlaceholder string
	if m.mode == render.ModeCompact {
		width := max(min(m.cols-2, 100), 1)
		footer := footerLine(m, elapsed, width)
		if m.message != "" {
			footer = render.Truncate(m.message, width)
		}
		content = strings.Join([]string{
			headerLine(width, remaining),
			progressBar(elapsed, m.total, width, m.tier == render.TierASCII),
			footer,
		}, "\n")
	} else {
		b := m.frames[m.frameIx].Bounds()
		availableCols := max(min(m.cols-2, 100), 1)
		boxCols, boxRows := render.Fit(b.Dx(), b.Dy(), availableCols,
			max(m.rows-3, 1), m.cellW, m.cellH)
		var img string
		switch m.tier {
		case render.TierKitty, render.TierSixel, render.TierITerm2:
			graphicPlaceholder = cellPlaceholder(boxCols, boxRows)
			if payload, err := m.encodeGraphicFrame(); err == nil {
				graphicPayload = payload
				img = graphicPlaceholder
			} else {
				img = render.HalfBlock(m.frames[m.frameIx], boxCols, boxRows)
			}
		case render.TierASCII:
			img = render.ASCII(m.frames[m.frameIx], boxCols, boxRows)
		case render.TierHalfBlock:
			if m.trueColor {
				img = render.HalfBlock(m.frames[m.frameIx], boxCols, boxRows)
			} else {
				img = render.HalfBlock256(m.frames[m.frameIx], boxCols, boxRows)
			}
		default:
			img = render.HalfBlock(m.frames[m.frameIx], boxCols, boxRows)
		}
		if m.message != "" {
			content = strings.Join([]string{
				headerLine(boxCols, remaining), img,
				progressBar(elapsed, m.total, boxCols, m.tier == render.TierASCII),
				render.Truncate(m.message, boxCols),
			}, "\n")
		} else {
			content = strings.Join([]string{
				headerLine(boxCols, remaining), img,
				progressBar(elapsed, m.total, boxCols, m.tier == render.TierASCII),
				footerLine(m, elapsed, boxCols),
			}, "\n")
		}
	}
	if m.popup != popupNone {
		return m.clearGraphic() + m.popupView()
	}
	contentHeight := lipgloss.Height(content)
	top := max(m.rows/3-contentHeight/2, 0)
	view := lipgloss.Place(m.cols, m.rows, lipgloss.Center, lipgloss.Top,
		strings.Repeat("\n", top)+content)
	if graphicPayload != "" {
		placeholderRow := strings.SplitN(graphicPlaceholder, "\n", 2)[0]
		view = strings.Replace(view, placeholderRow,
			m.clearKitty()+graphicPayload+placeholderRow, 1)
	} else if m.tier == render.TierKitty && m.mode == render.ModeCompact {
		view = m.clearKitty() + view
	}
	return view
}

func headerLine(width int, remaining time.Duration) string {
	label, timer := "Ad", formatTime(remaining)
	spaces := max(width-lipgloss.Width(label)-lipgloss.Width(timer), 1)
	return render.Truncate(label+strings.Repeat(" ", spaces)+timer, width)
}

func formatTime(remaining time.Duration) string {
	seconds := int(math.Ceil(max(remaining, 0).Seconds()))
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func progressBar(elapsed, total time.Duration, width int, ascii bool) string {
	if width < 1 {
		return ""
	}
	progress := 0.0
	if total > 0 {
		progress = min(max(float64(elapsed)/float64(total), 0), 1)
	}
	if ascii {
		filled := int(math.Floor(progress*float64(width) + 0.5))
		return strings.Repeat("=", filled) + strings.Repeat("-", width-filled)
	}
	halfCells := int(math.Floor(progress * float64(width*2)))
	full := halfCells / 2
	half := halfCells%2 == 1
	if half && full == width {
		half = false
	}
	empty := width - full
	if half {
		empty--
	}
	filledText := strings.Repeat("━", full)
	if half {
		filledText += "╸"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB000")).Render(filledText) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Faint(true).Render(strings.Repeat("─", empty))
}

func footerLine(m *model, elapsed time.Duration, width int) string {
	var skip string
	if elapsed < skipAfter(m.total) {
		wait := int(math.Ceil((skipAfter(m.total) - elapsed).Seconds()))
		skip = fmt.Sprintf("Skip in %ds", wait)
	} else {
		skip = "[ Skip ▶ ]"
	}
	if width < 30 {
		if elapsed < skipAfter(m.total) {
			skip = lipgloss.NewStyle().Faint(true).Render(skip)
		} else {
			skip = lipgloss.NewStyle().Reverse(true).Bold(true).Render(skip)
		}
		return skip + "\n" + render.Truncate("^C ?   r report", width)
	}
	separator, controls := " · ", "[Ctrl+C] ??? · [r] report"
	if m.cols < 50 {
		separator, controls = "   ", "^C ?   r report"
	}
	skipWidth := max(width-lipgloss.Width(separator)-lipgloss.Width(controls), 0)
	skip = render.Truncate(skip, skipWidth)
	if elapsed < skipAfter(m.total) {
		skip = lipgloss.NewStyle().Faint(true).Render(skip)
	} else {
		skip = lipgloss.NewStyle().Reverse(true).Bold(true).Render(skip)
	}
	return skip + separator + controls
}

func (m *model) encodeGraphicFrame() (string, error) {
	if m.graphicValid && m.graphicFrame == m.frameIx && m.graphicTier == m.tier {
		return m.graphicData, nil
	}
	var encoded strings.Builder
	if err := render.EncodeImage(&encoded, m.tier, m.frames[m.frameIx]); err != nil {
		return "", err
	}
	m.graphicValid = true
	m.graphicFrame = m.frameIx
	m.graphicTier = m.tier
	m.graphicData = encoded.String()
	return m.graphicData, nil
}

func cellPlaceholder(cols, rows int) string {
	line := strings.Repeat("\u2800", cols)
	lines := make([]string, rows)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func (m *model) clearKitty() string {
	if m.tier == render.TierKitty {
		return "\x1b_Ga=d,d=A\x1b\\"
	}
	return ""
}

func (m *model) clearGraphic() string {
	if m.tier == render.TierKitty {
		return m.clearKitty()
	}
	if m.tier == render.TierSixel || m.tier == render.TierITerm2 {
		return "\x1b[2J\x1b[H"
	}
	return ""
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
