package tui

import (
	"fmt"
	"image"
	"image/color"
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
	graphicValid      bool
	graphicFrame      int
	graphicTier       render.Tier
	graphicData       string
	graphicDim        bool
	rasterValid       bool
	rasterFrame       int
	rasterCols        int
	rasterRows        int
	rasterTier        render.Tier
	rasterTrueColor   bool
	rasterData        string
	dimFrameIndex     int
	dimFrameImage     image.Image
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
		if ix >= len(m.frames) {
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
	elapsed := m.elapsedAt(time.Now())
	remaining := m.total - elapsed
	if remaining < 0 {
		remaining = 0
	}
	var content string
	var graphicPayload, graphicPlaceholder string
	displayFrame := m.frames[m.frameIx]
	if m.popup != popupNone && isGraphicTier(m.tier) {
		displayFrame = m.dimmedFrame()
	}
	if m.mode == render.ModeCompact {
		width := max(min(m.cols-2, 100), 1)
		footer := footerLine(m, elapsed, width)
		if m.message != "" {
			footer = render.Truncate(m.message, width)
		}
		content = strings.Join([]string{
			headerLine(width, remaining),
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
			if payload, err := m.encodeGraphicFrame(displayFrame); err == nil {
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
		if m.message != "" {
			content = strings.Join([]string{
				headerLine(boxCols, remaining), img,
				progressBar(elapsed, m.total, boxCols, m.tier == render.TierASCII), "",
				render.Truncate(m.message, boxCols),
			}, "\n")
		} else {
			content = strings.Join([]string{
				headerLine(boxCols, remaining), img,
				progressBar(elapsed, m.total, boxCols, m.tier == render.TierASCII), "",
				footerLine(m, elapsed, boxCols),
			}, "\n")
		}
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
	if m.popup != popupNone {
		return overlayPopup(m.dimView(view), m.popupView(), m.cols, m.rows)
	}
	return view
}

func (m *model) dimView(view string) string {
	return lipgloss.NewStyle().Faint(true).Render(view)
}

func overlayPopup(base, popup string, cols, rows int) string {
	placed := lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, popup)
	baseLines := strings.Split(base, "\n")
	popupLines := strings.Split(placed, "\n")
	for y := range baseLines {
		if y >= len(popupLines) || strings.TrimSpace(stripANSI(popupLines[y])) == "" {
			continue
		}
		baseLines[y] = overlayLine(baseLines[y], popupLines[y], cols)
	}
	return strings.Join(baseLines, "\n")
}

func overlayLine(base, overlay string, width int) string {
	overlayWidth := lipgloss.Width(overlay)
	left := max((width-overlayWidth)/2, 0)
	return cutCells(base, 0, left) + overlay + cutCells(base, left+overlayWidth, width)
}

func cutCells(s string, start, end int) string {
	var out, priorSGR strings.Builder
	cells := 0
	wroteCell := false
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := escapeEnd(s, i)
			sequence := s[i:j]
			if strings.HasPrefix(sequence, "\x1b[") && strings.HasSuffix(sequence, "m") {
				if cells < start {
					priorSGR.WriteString(sequence)
				} else if cells < end {
					if !wroteCell && start > 0 {
						out.WriteString(priorSGR.String())
						wroteCell = true
					}
					out.WriteString(sequence)
				} else if sequence == "\x1b[0m" {
					out.WriteString(sequence)
					break
				}
			} else if cells >= start && cells < end {
				out.WriteString(sequence)
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := lipgloss.Width(string(r))
		if cells >= start && cells+w <= end {
			if !wroteCell && start > 0 {
				out.WriteString(priorSGR.String())
			}
			out.WriteRune(r)
			wroteCell = true
		}
		cells += w
		i += size
	}
	return out.String()
}

func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			i = escapeEnd(s, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

func escapeEnd(s string, start int) int {
	if start+1 >= len(s) {
		return len(s)
	}
	switch s[start+1] {
	case '[':
		for i := start + 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
	case ']', 'P', '_', '^':
		for i := start + 2; i < len(s); i++ {
			if s[i] == '\a' {
				return i + 1
			}
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
	}
	return min(start+2, len(s))
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
	dim := m.popup != popupNone
	if m.graphicValid && m.graphicFrame == m.frameIx && m.graphicTier == m.tier && m.graphicDim == dim {
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
	m.graphicDim = dim
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

func dimFrame(frame image.Image) image.Image {
	if uniform, ok := frame.(*image.Uniform); ok {
		r, g, b, a := uniform.C.RGBA()
		return image.NewUniform(color.RGBA{R: uint8((r >> 8) / 2), G: uint8((g >> 8) / 2), B: uint8((b >> 8) / 2), A: uint8(a >> 8)})
	}
	b := frame.Bounds()
	dim := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := frame.At(x, y).RGBA()
			dim.SetRGBA(x, y, color.RGBA{
				R: uint8((r >> 8) / 2), G: uint8((g >> 8) / 2),
				B: uint8((bl >> 8) / 2), A: uint8(a >> 8),
			})
		}
	}
	return dim
}

func (m *model) dimmedFrame() image.Image {
	if m.dimFrameImage == nil || m.dimFrameIndex != m.frameIx {
		m.dimFrameImage = dimFrame(m.frames[m.frameIx])
		m.dimFrameIndex = m.frameIx
	}
	return m.dimFrameImage
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
	small := m.cols < 24 || m.rows < 10
	if m.popup == popupReport {
		if m.reportSubmitted {
			text = "Thank you. Your report will be reviewed in 3-5 business years.\n\nPress Enter to restart the ad."
		} else if small {
			input := m.reportText
			if m.reportFocus == reportFocusInput {
				input = lipgloss.NewStyle().Reverse(true).Render(input + " ")
			}
			submit, cancel := "Submit", "Cancel"
			if m.reportFocus == reportFocusSubmit {
				submit = focusedButton(submit)
			}
			if m.reportFocus == reportFocusCancel {
				cancel = focusedButton(cancel)
			}
			text = fmt.Sprintf("Report this ad\n%s\n%d/%d\n%s %s", input, len([]rune(m.reportText)), reportMaxLength, submit, cancel)
		} else {
			text = m.reportFormView()
		}
		if small {
			return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).
				Width(max(m.cols-2, 1)).Background(lipgloss.Color("#181818")).
				BorderForeground(lipgloss.Color("#777777")).Render(text)
		}
		return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1, 2).
			Width(min(54, max(m.cols-10, 1))).Background(lipgloss.Color("#181818")).
			BorderForeground(lipgloss.Color("#777777")).Render(text)
	}
	if m.popup == popupReward {
		line := "Skipping loses your reward and cancels the pending install."
		continueLabel, skipLabel := "Continue", "Skip"
		if m.choice == 0 {
			continueLabel = focusedButton(continueLabel)
		} else {
			skipLabel = focusedButton(skipLabel)
		}
		text = line + "\n\n" + continueLabel + "    " + skipLabel
		if small {
			text = "Skipping loses reward and cancels install.\n" + continueLabel + " " + skipLabel
			return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).
				Width(max(m.cols-2, 1)).Background(lipgloss.Color("#181818")).
				BorderForeground(lipgloss.Color("#777777")).Render(text)
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1, 2).
		Width(min(54, max(m.cols-10, 1))).Background(lipgloss.Color("#181818")).
		BorderForeground(lipgloss.Color("#777777")).Render(text)
}

func (m *model) reportFormView() string {
	input := m.reportText
	if m.reportFocus == reportFocusInput {
		input = lipgloss.NewStyle().Reverse(true).Render(input + " ")
	}
	count := len([]rune(m.reportText))
	validity := validReport(m.reportText)
	submit := "Submit report"
	if m.reportFocus == reportFocusSubmit {
		submit = focusedButton(submit)
	}
	if !validity {
		submit = lipgloss.NewStyle().Faint(true).Render(submit)
	}
	cancel := "Cancel"
	if m.reportFocus == reportFocusCancel {
		cancel = focusedButton(cancel)
	}
	return "Report this ad\n\nWhy are you reporting this ad?\n" + input + "\n" +
		fmt.Sprintf("%d/%d", count, reportMaxLength) + "\n\n" + submit + "    " + cancel
}

func focusedButton(label string) string {
	return lipgloss.NewStyle().Reverse(true).Bold(true).Render("[ " + label + " ]")
}
