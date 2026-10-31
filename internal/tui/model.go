package tui

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rokuroo171/adpm/internal/render"
)

type tickMsg time.Time
type chromeTickMsg time.Time

const chromeInterval = time.Second / 30

// model drives one ad playback.
type model struct {
	frames            []image.Image
	title             string
	total             time.Duration
	start             time.Time
	cols              int
	rows              int
	cellW             int
	cellH             int
	tier              render.Tier
	trueColor         bool
	mode              render.Mode
	frameIx           int
	popup             popupKind
	choice            int
	paused            time.Time
	message           string
	messageUntil      time.Time
	rewardLines       []string
	reportMessage     string
	rewardIndex       int
	result            Result
	typo              bool
	graphicValid      bool
	graphicFrame      int
	graphicTier       render.Tier
	graphicData       string
	rasterValid       bool
	rasterFrame       int
	rasterCols        int
	rasterRows        int
	rasterTier        render.Tier
	rasterTrueColor   bool
	rasterData        string
	popupBase         render.CellGrid
	popupBaseValid    bool
	popupGraphicClear bool
	playerBlockTop    int
	playerBlockHeight int
	frameTickPending  bool
	chromeTickPending bool
	reportText        string
	reportFocus       int
	reportSubmitted   bool
}

type popupKind uint8

const (
	popupNone popupKind = iota
	popupReward
	popupReport
)

const reportMaxLength = 300

const (
	reportFocusInput = iota
	reportFocusSubmit
	reportFocusCancel
)

func (m *model) Init() tea.Cmd {
	return m.ensurePlaybackTicks()
}

func (m *model) ensurePlaybackTicks() tea.Cmd {
	var cmds []tea.Cmd
	if !m.frameTickPending {
		m.frameTickPending = true
		cmds = append(cmds, tea.Tick(frameInterval, func(t time.Time) tea.Msg { return tickMsg(t) }))
	}
	if !m.chromeTickPending {
		m.chromeTickPending = true
		cmds = append(cmds, tea.Tick(chromeInterval, func(t time.Time) tea.Msg { return chromeTickMsg(t) }))
	}
	return tea.Batch(cmds...)
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
		m.frameTickPending = false
		if m.popup != popupNone {
			return m, nil
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
		if m.typo {
			ix %= len(m.frames)
		} else if ix >= len(m.frames) {
			ix = len(m.frames) - 1
		}
		m.frameIx = ix
		return m, m.ensurePlaybackTicks()

	case chromeTickMsg:
		m.chromeTickPending = false
		if m.popup != popupNone {
			return m, nil
		}
		return m, m.ensurePlaybackTicks()

	case tea.WindowSizeMsg:
		m.cols, m.rows = msg.Width, msg.Height
		m.resize()
		m.rasterValid = false
		m.popupBaseValid = false
		m.popupGraphicClear = false
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
			m.popupBaseValid = false
			m.popupGraphicClear = false
			m.choice = 0
			m.paused = time.Now()
			m.rewardIndex++
		case "r":
			m.popup = popupReport
			m.popupBaseValid = false
			m.popupGraphicClear = false
			m.paused = time.Now()
			m.reportText = ""
			m.reportFocus = reportFocusInput
			m.reportSubmitted = false
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
		case "esc":
			m.resumePopup()
			return m, m.ensurePlaybackTicks()
		case "ctrl+c":
			return m, nil
		case "left", "right", "tab":
			m.choice = 1 - m.choice
		case "enter":
			if m.choice == 0 {
				m.resumePopup()
				return m, m.ensurePlaybackTicks()
			} else {
				m.result.Cancelled = true
				return m, tea.Quit
			}
		}
		return m, nil
	}
	if m.reportSubmitted {
		if msg.String() == "enter" {
			m.start = time.Now()
			m.paused = time.Time{}
			m.frameIx = 0
			m.popup = popupNone
			m.popupBaseValid = false
			m.popupGraphicClear = false
			m.message = ""
			return m, m.ensurePlaybackTicks()
		}
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.resumePopup()
		return m, m.ensurePlaybackTicks()
	case "tab", "right":
		m.reportFocus = (m.reportFocus + 1) % 3
	case "shift+tab", "left":
		m.reportFocus = (m.reportFocus + 2) % 3
	case "enter":
		switch m.reportFocus {
		case reportFocusInput:
			m.reportFocus = reportFocusSubmit
		case reportFocusSubmit:
			if validReport(m.reportText) {
				m.reportSubmitted = true
			}
		case reportFocusCancel:
			m.resumePopup()
			return m, m.ensurePlaybackTicks()
		}
	case "backspace", "ctrl+h":
		if m.reportFocus == reportFocusInput {
			runes := []rune(m.reportText)
			if len(runes) > 0 {
				m.reportText = string(runes[:len(runes)-1])
			}
		}
	default:
		if m.reportFocus == reportFocusInput && (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) {
			add := string(msg.Runes)
			if msg.Type == tea.KeySpace {
				add = " "
			}
			if len([]rune(m.reportText))+utf8.RuneCountInString(add) <= reportMaxLength {
				m.reportText += add
			}
		}
	}
	return m, nil
}

func (m *model) resumePopup() {
	m.start = m.start.Add(time.Since(m.paused))
	m.paused = time.Time{}
	m.popup = popupNone
	m.popupBaseValid = false
	m.popupGraphicClear = false
	m.message = ""
}

func validReport(text string) bool {
	count := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count >= 10
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
	if m.popup != popupNone && m.popupBaseValid {
		return m.popupViewOnGrid()
	}
	elapsed := m.elapsedAt(time.Now())
	remaining := m.total - elapsed
	if remaining < 0 {
		remaining = 0
	}
	var content string
	var graphicPayload, graphicPlaceholder string
	displayFrame := m.frames[m.frameIx]
	if m.mode == render.ModeCompact {
		width := max(min(m.cols-2, 100), 1)
		footer := footerLine(m, elapsed, width)
		if m.typo {
			footer = fmt.Sprintf("AD · %s · %ds · lockout", m.title, int(math.Ceil(remaining.Seconds())))
		}
		if m.message != "" {
			footer = render.Truncate(m.message, width)
		}
		content = strings.Join([]string{
			headerLine(width, remaining, m.popup != popupNone),
			progressBar(elapsed, m.total, width, m.tier == render.TierASCII),
			"",
			footer,
		}, "\n")
	} else {
		b := displayFrame.Bounds()
		availableCols := max(min(m.cols-2, 100), 1)
		boxCols, boxRows := render.Fit(b.Dx(), b.Dy(), availableCols,
			max(m.rows-4, 1), m.cellW, m.cellH)
		var img string
		switch m.tier {
		case render.TierKitty, render.TierSixel, render.TierITerm2:
			graphicPlaceholder = cellPlaceholder(boxCols, boxRows)
			if m.popup != popupNone {
				img = graphicPlaceholder
			} else if payload, err := m.encodeGraphicFrame(displayFrame); err == nil {
				graphicPayload = payload
				img = graphicPlaceholder
			} else {
				img = render.HalfBlock(displayFrame, boxCols, boxRows)
			}
		case render.TierASCII:
			img = m.renderRaster(displayFrame, boxCols, boxRows)
		case render.TierHalfBlock:
			img = m.renderRaster(displayFrame, boxCols, boxRows)
		default:
			img = m.renderRaster(displayFrame, boxCols, boxRows)
		}
		footer := footerLine(m, elapsed, boxCols)
		if m.typo {
			footer = "Unskippable ad"
		}
		if m.message != "" {
			footer = render.Truncate(m.message, boxCols)
		}
		content = strings.Join([]string{
			headerLine(boxCols, remaining, m.popup != popupNone), img,
			progressBar(elapsed, m.total, boxCols, m.tier == render.TierASCII), "",
			footer,
		}, "\n")
	}
	contentHeight := lipgloss.Height(content)
	top := max(m.rows/3-contentHeight/2, 0)
	m.playerBlockTop = top
	m.playerBlockHeight = contentHeight
	view := lipgloss.Place(m.cols, m.rows, lipgloss.Center, lipgloss.Top,
		strings.Repeat("\n", top)+content)
	if graphicPayload != "" {
		placeholderRow := strings.SplitN(graphicPlaceholder, "\n", 2)[0]
		view = strings.Replace(view, placeholderRow,
			m.clearKitty()+graphicPayload+placeholderRow, 1)
	} else if m.popup == popupNone && m.tier == render.TierKitty && m.mode == render.ModeCompact {
		view = m.clearKitty() + view
	}
	if m.popup != popupNone {
		base := render.ParseANSIGrid(view, m.cols, m.rows, func(r rune) int { return lipgloss.Width(string(r)) })
		m.popupBase = render.DimGrid(base, 0.4, m.tier == render.TierASCII)
		m.popupBaseValid = true
		m.popupGraphicClear = false
		return m.popupViewOnGrid()
	}
	m.popupBaseValid = false
	return view
}

func (m *model) popupOrigin(width, height int) (int, int) {
	centerY := m.rows / 2
	if m.playerBlockHeight > 0 {
		centerY = m.playerBlockTop + m.playerBlockHeight/2
	}
	return max((m.cols-width)/2, 0), centerY - height/2
}

func (m *model) popupViewOnGrid() string {
	panel := m.popupView()
	panelWidth, panelHeight := lipgloss.Width(panel), lipgloss.Height(panel)
	panelGrid := render.ParseANSIGrid(panel, panelWidth, panelHeight,
		func(r rune) int { return lipgloss.Width(string(r)) })
	if m.popup == popupReport && !m.reportSubmitted && m.reportFocus == reportFocusInput {
		underlineInputCursor(&panelGrid)
	}
	x, y := m.popupOrigin(panelWidth, panelHeight)
	grid := render.OverlayGrid(m.popupBase, panelGrid, x, y)
	if isGraphicTier(m.tier) {
		clear := ""
		if !m.popupGraphicClear {
			clear = m.clearGraphic()
			m.popupGraphicClear = true
		}
		return clear + render.RenderGrid(grid)
	}
	return render.RenderGrid(grid)
}

func underlineInputCursor(grid *render.CellGrid) {
	for i := range grid.Cells {
		if grid.Cells[i].Rune != '▏' {
			continue
		}
		y := i / grid.Width
		for x := i % grid.Width; x >= 0; x-- {
			cell := &grid.Cells[y*grid.Width+x]
			if cell.Rune == '│' || cell.Rune == '╭' || cell.Rune == '╰' {
				break
			}
			cell.Underline = true
		}
	}
}

func headerLine(width int, remaining time.Duration, paused bool) string {
	label, timer := "Ad", formatTime(remaining)
	if paused {
		label = "Ad · paused"
	}
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
	if elapsed < skipAfter(m.total) {
		skip = lipgloss.NewStyle().Faint(true).Render(skip)
	} else {
		skip = lipgloss.NewStyle().Reverse(true).Bold(true).Render(skip)
	}
	center, right := "[Ctrl+C] ???", "[r] report"
	if width < lipgloss.Width(skip)+lipgloss.Width(center)+lipgloss.Width(right)+2 {
		center, right = "^C ?", "r report"
	}
	leftWidth, centerWidth, rightWidth := lipgloss.Width(skip), lipgloss.Width(center), lipgloss.Width(right)
	leftGap := max(width/2-centerWidth/2-leftWidth, 1)
	rightGap := max(width-leftWidth-centerWidth-rightWidth-leftGap, 1)
	return skip + strings.Repeat(" ", leftGap) + center + strings.Repeat(" ", rightGap) + right
}

func (m *model) encodeGraphicFrame(frame image.Image) (string, error) {
	if m.graphicValid && m.graphicFrame == m.frameIx && m.graphicTier == m.tier {
		return m.graphicData, nil
	}
	var encoded strings.Builder
	if err := render.EncodeImage(&encoded, m.tier, frame); err != nil {
		return "", err
	}
	m.graphicValid = true
	m.graphicFrame = m.frameIx
	m.graphicTier = m.tier
	m.graphicData = encoded.String()
	return m.graphicData, nil
}

func (m *model) renderRaster(frame image.Image, cols, rows int) string {
	if m.rasterValid && m.rasterFrame == m.frameIx && m.rasterCols == cols &&
		m.rasterRows == rows && m.rasterTier == m.tier &&
		m.rasterTrueColor == m.trueColor {
		return m.rasterData
	}
	switch m.tier {
	case render.TierASCII:
		m.rasterData = render.ASCII(frame, cols, rows)
	case render.TierHalfBlock:
		if m.trueColor {
			m.rasterData = render.HalfBlock(frame, cols, rows)
		} else {
			m.rasterData = render.HalfBlock256(frame, cols, rows)
		}
	default:
		m.rasterData = render.HalfBlock(frame, cols, rows)
	}
	m.rasterValid = true
	m.rasterFrame = m.frameIx
	m.rasterCols = cols
	m.rasterRows = rows
	m.rasterTier = m.tier
	m.rasterTrueColor = m.trueColor
	return m.rasterData
}

func isGraphicTier(tier render.Tier) bool {
	return tier == render.TierKitty || tier == render.TierSixel || tier == render.TierITerm2
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
	width := popupWidth(m.cols)
	inner := max(width-2, 1)
	narrow := width < 44
	lines := []string{lipgloss.NewStyle().Faint(true).Render("Ad paused"), ""}
	switch {
	case m.popup == popupReward:
		lines = append(lines, wrapPopupText("Skipping loses your reward and cancels the pending install.", inner), "")
		continueButton := dialogButton("Continue", m.choice == 0, false)
		skipButton := dialogButton("Skip", m.choice == 1, false)
		if narrow {
			lines = append(lines, continueButton, skipButton)
		} else {
			lines = append(lines, continueButton+"    "+skipButton)
		}
	case m.popup == popupReport && m.reportSubmitted:
		lines[0] = lipgloss.NewStyle().Faint(true).Render("Report this ad")
		lines = append(lines,
			wrapPopupText("Thank you. Your report will be reviewed in 3-5 business years.", inner),
			"",
			wrapPopupText("Press Enter to restart the ad.", inner))
	case m.popup == popupReport:
		lines[0] = lipgloss.NewStyle().Faint(true).Render("Report this ad")
		lines = append(lines,
			wrapPopupText("Why are you reporting this ad?", inner),
			reportInputLine(m.reportText, inner, m.reportFocus == reportFocusInput),
			fmt.Sprintf("%d/%d", len([]rune(m.reportText)), reportMaxLength),
			"")
		submit := dialogButton("Submit report", m.reportFocus == reportFocusSubmit, !validReport(m.reportText))
		cancel := dialogButton("Cancel", m.reportFocus == reportFocusCancel, false)
		if narrow {
			lines = append(lines, submit, cancel)
		} else {
			lines = append(lines, submit+"    "+cancel)
		}
	}
	if m.popup != popupReport || !m.reportSubmitted {
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render("←/→ choose · Enter confirm"))
	}
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(inner).Padding(1, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#626262")).
		Foreground(lipgloss.Color("#D0D0D0")).
		Background(lipgloss.Color("#171717")).Render(content)
}

func popupWidth(cols int) int {
	if cols < 44 {
		return max(cols-2, 1)
	}
	return min(60, max(cols-4, 1))
}

func wrapPopupText(text string, width int) string {
	return lipgloss.NewStyle().Width(width).Render(text)
}

func reportInputLine(text string, width int, focused bool) string {
	cursor := ""
	if focused {
		cursor = "▏"
		width = max(width-lipgloss.Width(cursor), 0)
	}
	runes := []rune(text)
	start, used := len(runes), 0
	for start > 0 {
		runeWidth := lipgloss.Width(string(runes[start-1]))
		if used+runeWidth > width {
			break
		}
		used += runeWidth
		start--
	}
	return lipgloss.NewStyle().Underline(true).Render(string(runes[start:]) + cursor)
}

func dialogButton(label string, focused, disabled bool) string {
	style := lipgloss.NewStyle()
	if focused {
		style = style.Reverse(true)
	}
	if !focused || disabled {
		style = style.Faint(true)
	}
	return style.Render("[ " + label + " ]")
}
