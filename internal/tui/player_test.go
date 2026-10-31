package tui

import (
	"bufio"
	"image"
	"image/color"
	"image/draw"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rokuroo171/adpm/internal/render"
)

func testFrames(n int) []image.Image {
	frames := make([]image.Image, n)
	for i := range frames {
		img := image.NewRGBA(image.Rect(0, 0, 320, 180))
		draw.Draw(img, img.Bounds(),
			&image.Uniform{C: color.RGBA{180, 90, 40, 255}}, image.Point{}, draw.Src)
		frames[i] = img
	}
	return frames
}

func newTestModel(t *testing.T, cols, rows int) *model {
	t.Helper()
	m := &model{
		frames:    testFrames(10),
		title:     "Synergy™",
		total:     10 * frameInterval,
		start:     time.Now(),
		tier:      render.TierHalfBlock,
		trueColor: true,
	}
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: cols, Height: rows}); cmd != nil {
		t.Fatalf("WindowSizeMsg should not return a command")
	}
	return m
}

var sgrRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func strippedLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = sgrRe.ReplaceAllString(l, "")
	}
	return out
}

// These terminal sizes must render without garbling:
// nothing wider than the terminal, no replacement runes, and the right tier.
func TestViewAtDoneWhenSizes(t *testing.T) {
	tests := []struct {
		cols, rows int
		wantTier   render.Mode
		wantMarker string
	}{
		{80, 24, render.ModeFull, "▀"},
		{40, 20, render.ModeFull, "▀"},
		{20, 6, render.ModeCompact, "Ad"},
	}
	for _, tt := range tests {
		t.Run("80x24x"[0:0]+itoa(tt.cols)+"x"+itoa(tt.rows), func(t *testing.T) {
			m := newTestModel(t, tt.cols, tt.rows)
			if m.mode != tt.wantTier {
				t.Errorf("mode = %v, want %v", m.mode, tt.wantTier)
			}
			view := m.View()
			if strings.Contains(view, "�") {
				t.Errorf("view contains replacement runes")
			}
			lines := strippedLines(view)
			if len(lines) > tt.rows+1 {
				t.Errorf("view has %d lines, terminal has %d rows", len(lines), tt.rows)
			}
			for i, l := range lines {
				if n := len([]rune(l)); n > tt.cols {
					t.Errorf("line %d is %d cells wide, terminal has %d cols: %q", i, n, tt.cols, l)
				}
			}
			if !strings.Contains(view, tt.wantMarker) {
				t.Errorf("view missing %q for tier %v", tt.wantMarker, tt.wantTier)
			}
		})
	}
}

func TestViewUsesASCIIAndIndexedHalfBlockTiers(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.tier = render.TierASCII
	view := m.View()
	if strings.Contains(view, "▀") || sgrRe.MatchString(view) {
		t.Errorf("ASCII tier contains image glyphs or color escapes")
	}

	m.tier = render.TierHalfBlock
	m.trueColor = false
	view = m.View()
	if !strings.Contains(view, ";5;") {
		t.Errorf("limited-color half-block view lacks indexed colors")
	}
}

func TestCountdownBarUsesHalfCellProgress(t *testing.T) {
	got := sgrRe.ReplaceAllString(progressBar(375*time.Millisecond, time.Second, 4, false), "")
	if got != "━╸──" {
		t.Fatalf("countdownBar = %q, want %q", got, "━╸──")
	}
}

func TestViewUsesClockHeaderAndShortFooter(t *testing.T) {
	m := newTestModel(t, 40, 20)
	m.total = 10 * time.Second
	view := m.View()
	if !strings.Contains(view, "Ad") || !strings.Contains(view, "0:10") {
		t.Fatalf("view missing label or remaining time: %q", view)
	}
	if !strings.Contains(view, "[Ctrl+C] ???") || !strings.Contains(view, "[r] report") {
		t.Fatalf("narrow footer missing short controls: %q", view)
	}
	if !strings.Contains(view, "[Ctrl+C] ???") || !strings.Contains(view, "[r] report") {
		t.Fatalf("footer missing aligned control labels: %q", view)
	}
}

func TestFooterSpreadsControlsAcrossVideoWidth(t *testing.T) {
	m := newTestModel(t, 90, 70)
	m.total = 10 * time.Second
	line := sgrRe.ReplaceAllString(footerLine(m, 0, 88), "")
	if !strings.HasPrefix(line, "Skip in 4s") {
		t.Fatalf("footer left label = %q", line)
	}
	if strings.Index(line, "[Ctrl+C] ???") < 25 || strings.Index(line, "[r] report") < 70 {
		t.Fatalf("footer controls are not edge-aligned: %q", line)
	}
}

func TestPlayerHasBlankRowBetweenBarAndFooter(t *testing.T) {
	m := newTestModel(t, 80, 24)
	lines := strippedLines(m.View())
	bar := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && strings.Trim(trimmed, "━╸─") == "" {
			bar = i
			break
		}
	}
	if bar < 0 || bar+2 >= len(lines) || strings.TrimSpace(lines[bar+1]) != "" {
		t.Fatalf("expected one blank row under progress bar: %q", lines)
	}
	if !strings.Contains(lines[bar+2], "Skip in") {
		t.Fatalf("footer did not follow the blank row: %q", lines)
	}
}

func TestCompactFooterKeepsSkipAndControlsVisible(t *testing.T) {
	m := newTestModel(t, 20, 6)
	m.total = 10 * time.Second
	view := sgrRe.ReplaceAllString(m.View(), "")
	if !strings.Contains(view, "Skip in") || !strings.Contains(view, "^C ?") || !strings.Contains(view, "r report") {
		t.Fatalf("compact footer omitted a control: %q", view)
	}
}

func TestASCIIViewUsesPlainProgressCharacters(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.total = 10 * time.Second
	m.tier = render.TierASCII
	m.start = time.Now().Add(-5 * time.Second)
	view := sgrRe.ReplaceAllString(m.View(), "")
	if !strings.Contains(view, "====") || !strings.Contains(view, "----") {
		t.Fatalf("ASCII progress bar missing plain fill and remainder: %q", view)
	}
}

func TestTallLayoutUsesWideVideoAndUpperThirdAnchor(t *testing.T) {
	m := newTestModel(t, 90, 70)
	m.cellW, m.cellH = 8, 16
	lines := strippedLines(m.View())
	firstContent := -1
	barWidth := 0
	for i, line := range lines {
		if firstContent < 0 && strings.TrimSpace(line) != "" {
			firstContent = i
		}
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && strings.Trim(trimmed, "━╸─") == "" {
			barWidth = len([]rune(trimmed))
		}
	}
	if firstContent < 0 || firstContent >= 70/3 {
		t.Fatalf("player begins at row %d, want upper third", firstContent)
	}
	if barWidth != 88 {
		t.Fatalf("progress bar width = %d, want video width 88", barWidth)
	}
}

func TestViewUsesDetectedGraphicProtocol(t *testing.T) {
	for _, tt := range []struct {
		tier   render.Tier
		marker string
	}{
		{render.TierKitty, "\x1b_Ga=T,"},
		{render.TierSixel, "\x1bP"},
		{render.TierITerm2, "\x1b]1337;File=inline=1:"},
	} {
		t.Run(tt.marker, func(t *testing.T) {
			m := newTestModel(t, 80, 24)
			m.frames = []image.Image{image.NewRGBA(image.Rect(0, 0, 8, 8))}
			m.tier = tt.tier
			view := m.View()
			if !strings.Contains(view, tt.marker) || !strings.Contains(view, "\u2800") {
				t.Errorf("graphic view: protocol=%v placeholders=%v prefix=%q",
					strings.Contains(view, tt.marker), strings.Contains(view, "\u2800"), view[:min(len(view), 100)])
			}
		})
	}
}

func TestPopupOverlaysKeepPlayerVisible(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	view := m.View()
	if !strings.Contains(view, "Ad") || !strings.Contains(view, "0:01") {
		t.Error("reward overlay removed the underlying player")
	}
	if !strings.Contains(view, "Continue") || !strings.Contains(view, "Skip") {
		t.Error("reward overlay is missing Continue and Skip actions")
	}
	if !strings.Contains(view, "╭") || !strings.Contains(view, "loses your reward") {
		t.Error("reward overlay is missing its rounded modal")
	}
}

func TestGraphicPopupHidesFrameWhileOpen(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.tier = render.TierKitty
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	view := m.View()
	if strings.Contains(view, "\x1b_Ga=T,") {
		t.Fatal("Kitty frame was shown while the popup was open")
	}
}

func TestPopupKeepsCompletePlayerGridAtTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{90, 30}, {90, 70}, {40, 20}, {30, 10}} {
		t.Run(itoa(size[0])+"x"+itoa(size[1]), func(t *testing.T) {
			m := newTestModel(t, size[0], size[1])
			m.total = time.Minute
			before := render.ParseANSIGrid(m.View(), size[0], size[1], func(r rune) int { return lipgloss.Width(string(r)) })
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			view := m.View()
			after := render.ParseANSIGrid(view, size[0], size[1], func(r rune) int { return lipgloss.Width(string(r)) })
			if after.Width != size[0] || after.Height != size[1] {
				t.Fatalf("popup screen grid = %dx%d, want %dx%d", after.Width, after.Height, size[0], size[1])
			}
			if !strings.Contains(sgrRe.ReplaceAllString(render.RenderGrid(m.popupBase), ""), "Ad · paused") {
				t.Fatal("cached player background is missing the paused label")
			}
			panel := m.popupView()
			left, top := m.popupOrigin(lipgloss.Width(panel), lipgloss.Height(panel))
			for y := 0; y < size[1]; y++ {
				if y == m.playerBlockTop {
					continue
				}
				for x := 0; x < size[0]; x++ {
					if x >= left && x < left+lipgloss.Width(panel) && y >= top && y < top+lipgloss.Height(panel) {
						continue
					}
					index := y*size[0] + x
					if before.Cells[index].Rune != after.Cells[index].Rune {
						t.Fatalf("background cell (%d,%d) changed outside the popup", x, y)
					}
				}
			}
		})
	}
}

func TestBothPopupsUseSameDimmedBackground(t *testing.T) {
	makePopup := func(key tea.KeyMsg) render.CellGrid {
		m := newTestModel(t, 80, 24)
		m.Update(key)
		m.View()
		return m.popupBase
	}
	reward := makePopup(tea.KeyMsg{Type: tea.KeyCtrlC})
	report := makePopup(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if len(reward.Cells) != len(report.Cells) {
		t.Fatalf("dimmed backgrounds have different cell counts: %d and %d", len(reward.Cells), len(report.Cells))
	}
	for i := range reward.Cells {
		if reward.Cells[i] != report.Cells[i] {
			t.Fatalf("dimmed popup backgrounds differ at cell %d", i)
		}
	}
}

func TestPopupResizeRebuildsAndRecentersGrid(t *testing.T) {
	m := newTestModel(t, 90, 70)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.View()
	oldPanel := m.popupView()
	oldLeft, oldTop := m.popupOrigin(lipgloss.Width(oldPanel), lipgloss.Height(oldPanel))
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	if m.popupBaseValid {
		t.Fatal("resize retained the stale popup background")
	}
	view := m.View()
	grid := render.ParseANSIGrid(view, 40, 20, func(r rune) int { return lipgloss.Width(string(r)) })
	panel := m.popupView()
	left, top := m.popupOrigin(lipgloss.Width(panel), lipgloss.Height(panel))
	if grid.Width != 40 || grid.Height != 20 || (left == oldLeft && top == oldTop) {
		t.Fatalf("resized popup grid is stale: size=%dx%d origin=(%d,%d)", grid.Width, grid.Height, left, top)
	}
}

func TestPopupWidthAndNarrowButtonLayout(t *testing.T) {
	for _, tc := range []struct {
		cols int
		want int
	}{{90, 60}, {40, 38}, {30, 28}} {
		m := newTestModel(t, tc.cols, 20)
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		panel := m.popupView()
		if got := lipgloss.Width(panel); got != tc.want {
			t.Fatalf("%d-column popup width = %d, want %d", tc.cols, got, tc.want)
		}
		plain := sgrRe.ReplaceAllString(panel, "")
		continueAt := strings.Index(plain, "[ Continue ]")
		skipAt := strings.Index(plain, "[ Skip ]")
		if continueAt < 0 || skipAt < 0 {
			t.Fatalf("%d-column popup missing action buttons: %q", tc.cols, plain)
		}
		continueRow := strings.Count(plain[:continueAt], "\n")
		skipRow := strings.Count(plain[:skipAt], "\n")
		if tc.cols < 44 && skipRow == continueRow {
			t.Fatalf("%d-column popup buttons were not stacked: %q", tc.cols, plain)
		}
		if tc.cols >= 44 && skipRow != continueRow {
			t.Fatalf("%d-column popup buttons were stacked: %q", tc.cols, plain)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Shrinking into one-line territory while the TUI is up keeps the compact
// tier and stays inside the new size.
func TestResizeShrinksCleanly(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.Update(tea.WindowSizeMsg{Width: 12, Height: 4})
	if m.mode != render.ModeCompact {
		t.Errorf("mode = %v, want compact (never back to one-line in the TUI)", m.mode)
	}
	for i, l := range strippedLines(m.View()) {
		if n := len([]rune(l)); n > 12 {
			t.Errorf("line %d is %d cells wide after shrink: %q", i, n, l)
		}
	}
}

func TestTickFinishesAtTotal(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.start = time.Now().Add(-m.total - time.Millisecond)

	_, cmd := m.Update(tickMsg{})
	if !m.result.Finished {
		t.Error("Finished = false after total elapsed")
	}
	if m.result.Duration != m.total {
		t.Errorf("Duration = %v, want %v", m.result.Duration, m.total)
	}
	if cmd == nil {
		t.Error("expected a quit command after finish")
	}
}

func TestTickAdvancesFrames(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.start = time.Now().Add(-3 * frameInterval)

	_, _ = m.Update(tickMsg{})
	if m.frameIx != 3 {
		t.Errorf("frameIx = %d, want 3", m.frameIx)
	}
	if m.result.Finished {
		t.Error("Finished = true before total elapsed")
	}
}

func TestPlaybackAndChromeTickRates(t *testing.T) {
	if frameInterval != 100*time.Millisecond {
		t.Errorf("frame interval = %v, want 100ms", frameInterval)
	}
	if chromeInterval != time.Second/30 {
		t.Errorf("chrome interval = %v, want %v", chromeInterval, time.Second/30)
	}
}

func TestGraphicPayloadCacheSurvivesChromeRefresh(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.frames = []image.Image{image.NewRGBA(image.Rect(0, 0, 8, 8))}
	m.tier = render.TierKitty
	firstView := m.View()
	firstPayload := m.graphicData
	if firstPayload == "" || !strings.Contains(firstView, firstPayload) {
		t.Fatal("first view did not encode and include the Kitty frame")
	}
	m.start = time.Now().Add(-20 * time.Millisecond)
	secondView := m.View()
	if m.graphicData != firstPayload || !strings.Contains(secondView, firstPayload) {
		t.Error("chrome refresh changed the cached frame payload")
	}
}

func TestPopupStopsRenderTicksUntilDismissed(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, cmd := m.Update(chromeTickMsg(time.Now())); cmd != nil {
		t.Fatal("chrome tick continued while modal was open")
	}
	if _, cmd := m.Update(tickMsg(time.Now())); cmd != nil {
		t.Fatal("frame tick continued while modal was open")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("dismissing modal did not restart playback ticks")
	}
}

func TestRasterPopupReusesUndimmedSourceFrame(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.View()
	if !m.popupBaseValid || m.popupBase.Width != 80 || m.popupBase.Height != 24 {
		t.Fatal("popup did not cache the dimmed player grid")
	}
	firstCell := &m.popupBase.Cells[0]
	m.View()
	if &m.popupBase.Cells[0] != firstCell {
		t.Fatal("popup rebuilt the frozen background grid")
	}
}

func TestKeys(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.rewardLines = []string{"Reward line one", "Reward line two"}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.result.Cancelled {
		t.Fatal("Ctrl+C cancelled before the popup was confirmed")
	}
	if !strings.Contains(m.View(), "loses your reward") {
		t.Fatalf("Ctrl+C did not show the reward popup: %q", m.View())
	}

	started := m.start
	paused := m.paused
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.result.Cancelled {
		t.Error("Enter on default Continue cancelled the install")
	}
	if m.popup != popupNone || !m.start.After(started) || m.start.Sub(started) < time.Since(paused)-time.Second {
		t.Error("Continue did not resume the same paused ad")
	}

	m = newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.result.Cancelled {
		t.Error("choosing Skip did not cancel the install")
	}

	m = newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !strings.Contains(m.View(), "Report this ad") || !strings.Contains(m.View(), "Why are you reporting this ad?") {
		t.Fatalf("r did not show the report form: %q", m.View())
	}
	oldStart := m.start
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.reportSubmitted {
		t.Error("empty report submitted")
	}
	m.reportText = "This ad is misleading"
	m.reportFocus = reportFocusSubmit
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmation := sgrRe.ReplaceAllString(m.View(), "")
	if !m.reportSubmitted || !strings.Contains(confirmation, "Thank you") ||
		!strings.Contains(confirmation, "business") || !strings.Contains(confirmation, "years.") {
		t.Errorf("valid report did not show parody confirmation: submitted=%v view=%q", m.reportSubmitted, confirmation)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.start.After(oldStart) || m.popup != popupNone {
		t.Error("report confirmation did not restart the ad")
	}
}

func TestReportValidation(t *testing.T) {
	for _, tt := range []struct {
		text string
		want bool
	}{
		{"", false}, {"         ", false}, {"short", false}, {"1234567890", true},
	} {
		if got := validReport(tt.text); got != tt.want {
			t.Errorf("validReport(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestReportInputAcceptsSpacesAndEnterMovesToSubmit(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	for _, r := range []rune("This is bad ad") {
		if r == ' ' {
			m.Update(tea.KeyMsg{Type: tea.KeySpace})
		} else {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
	if m.reportText != "This is bad ad" {
		t.Fatalf("report text = %q, want %q", m.reportText, "This is bad ad")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.reportFocus != reportFocusSubmit || m.reportSubmitted {
		t.Fatalf("Enter from input should focus Submit, focus=%d submitted=%v", m.reportFocus, m.reportSubmitted)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.reportSubmitted {
		t.Fatal("Enter on focused Submit did not submit the valid report")
	}
}

func TestReportInputLimitAndModalShortcuts(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	for i := 0; i < reportMaxLength+10; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	}
	if len([]rune(m.reportText)) != reportMaxLength {
		t.Fatalf("report length = %d, want %d", len([]rune(m.reportText)), reportMaxLength)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.popup != popupNone || m.result.Finished {
		t.Fatal("Escape did not dismiss the report form without submitting")
	}
}

func TestEscapeResumesRewardWithoutResettingPlayback(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.total = 20 * time.Second
	m.start = time.Now().Add(-7 * time.Second)
	m.frameIx = 7
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.popup != popupNone || m.frameIx != 7 {
		t.Fatalf("Escape changed ad state: popup=%v frame=%d", m.popup, m.frameIx)
	}
	if got := m.elapsedAt(time.Now()); got < 7*time.Second {
		t.Fatal("Escape did not resume elapsed playback from its paused position")
	}
}

func TestReportDialogTabCyclesInputAndActions(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.reportFocus != reportFocusInput {
		t.Fatal("report input did not receive focus on open")
	}
	for _, want := range []int{reportFocusSubmit, reportFocusCancel, reportFocusInput} {
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if m.reportFocus != want {
			t.Fatalf("report focus = %d, want %d", m.reportFocus, want)
		}
	}
}

func TestReportInputShowsUnderlineAndCursor(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	view := m.View()
	if !strings.Contains(view, "▏") {
		t.Fatal("focused report field has no visible cursor")
	}
	grid := render.ParseANSIGrid(view, 80, 24, func(r rune) int { return lipgloss.Width(string(r)) })
	for _, cell := range grid.Cells {
		if cell.Rune == '▏' && cell.Underline {
			return
		}
	}
	t.Fatal("focused report cursor is not underlined")
}

func TestInputReadyReadsBufferedAndPolledBytes(t *testing.T) {
	input, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer output.Close()
	reader := bufio.NewReader(input)
	if inputReady(reader, int(input.Fd()), 0) {
		t.Fatal("empty input was reported as ready")
	}
	if _, err := output.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if !inputReady(reader, int(input.Fd()), 100) {
		t.Fatal("pipe input was not reported as ready")
	}
	if _, err := reader.ReadByte(); err != nil {
		t.Fatal(err)
	}
	if !inputReady(reader, int(input.Fd()), 0) {
		t.Fatal("buffered input was not reported as ready")
	}
}

func TestSkipFooterChangesAtDelay(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.total = 10 * time.Second
	if !strings.Contains(m.View(), "Skip in") {
		t.Fatalf("early footer does not show skip countdown: %q", m.View())
	}
	m.start = time.Now().Add(-4*time.Second - time.Millisecond)
	if !strings.Contains(m.View(), "[ Skip ▶ ]") {
		t.Fatalf("unlocked footer does not show skip button: %q", m.View())
	}
}

func TestRewardPopupPausesAdUntilDismissed(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.start = time.Now().Add(-m.total - time.Second)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	wantElapsed := m.paused.Sub(m.start)
	if got := m.elapsedAt(m.paused.Add(5 * time.Second)); got != wantElapsed {
		t.Fatalf("elapsed during popup = %v, want frozen value %v", got, wantElapsed)
	}
	_, cmd := m.Update(tickMsg(time.Now()))
	if m.result.Finished || cmd != nil {
		t.Fatal("the paused ad finished or kept ticking while the reward popup was open")
	}
}

func TestCtrlZStillSuspends(t *testing.T) {
	m := newTestModel(t, 80, 24)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if !m.result.Suspended || cmd == nil {
		t.Fatal("Ctrl+Z did not suspend the player")
	}
}

func TestRewardPopupFitsCompactTerminals(t *testing.T) {
	for _, size := range [][2]int{{40, 20}, {20, 6}} {
		m := newTestModel(t, size[0], size[1])
		m.rewardLines = []string{"You won't get your reward if you skip this ad."}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		lines := strippedLines(m.View())
		if len(lines) > size[1] {
			t.Errorf("%dx%d popup has %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if width := len([]rune(line)); width > size[0] {
				t.Errorf("%dx%d line %d has width %d: %q", size[0], size[1], i, width, line)
			}
		}
	}
}

func TestReportPopupFitsCompactTerminal(t *testing.T) {
	m := newTestModel(t, 20, 6)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	lines := strippedLines(m.View())
	if len(lines) > 6 {
		t.Fatalf("report popup has %d lines in 6-row terminal", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 20 {
			t.Errorf("line %d has width %d in 20-column terminal: %q", i, width, line)
		}
	}
}

func TestOneLineControlsStayVisibleWithinTerminalWidth(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.total = 10 * time.Second
	line := oneLineView(m, 18, time.Now())
	if len([]rune(line)) > 18 || line == "" {
		t.Errorf("one-line countdown = %q", line)
	}
	m.rewardLines = []string{"Reward"}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	line = oneLineView(m, 18, time.Now())
	if len([]rune(line)) > 18 || line == "" {
		t.Errorf("one-line reward popup = %q", line)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	line = oneLineView(m, 18, time.Now())
	if len([]rune(line)) > 18 || line == "" {
		t.Errorf("one-line OK choice = %q", line)
	}
}

func TestEnterRestartsBeforeSkipDelayAndFinishesAfter(t *testing.T) {
	for _, tt := range []struct {
		name       string
		total      time.Duration
		wait       time.Duration
		wantFinish bool
	}{
		{"3 second ad waits 2 seconds", 3 * time.Second, 1500 * time.Millisecond, false},
		{"10 second ad waits 4 seconds", 10 * time.Second, 3 * time.Second, false},
		{"20 second ad waits 5 seconds", 20 * time.Second, 5 * time.Second, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t, 80, 24)
			m.total = tt.total
			m.start = time.Now().Add(-tt.wait)
			_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.result.Finished != tt.wantFinish {
				t.Errorf("Finished = %v, want %v", m.result.Finished, tt.wantFinish)
			}
			if !tt.wantFinish && !m.start.After(time.Now().Add(-time.Second)) {
				t.Error("early Enter did not restart the ad")
			}
		})
	}
}

// A forced panic must still put the terminal back: leave the alt screen and
// show the cursor.
func TestForcedPanicRestoresTerminal(t *testing.T) {
	out := captureStdout(t, func() {
		_, err := withCleanup(func() (Result, error) {
			panic("forced panic")
		})
		if err == nil {
			t.Error("panic was not converted to an error")
		} else if !strings.Contains(err.Error(), "forced panic") {
			t.Errorf("error = %q, want it to mention the panic", err)
		}
	})
	if !strings.Contains(out, "\x1b[?1049l") {
		t.Errorf("output missing leave-alt-screen sequence: %q", out)
	}
	if !strings.Contains(out, "\x1b[?25h") {
		t.Errorf("output missing show-cursor sequence: %q", out)
	}
	if !strings.Contains(out, "\x1b_Ga=d,d=A\x1b\\") {
		t.Error("cleanup did not delete Kitty images")
	}
}

// The one-line tier runs without the alt screen, prints one line, and ends
// finished.
func TestPlayOneLine(t *testing.T) {
	out := captureStdout(t, func() {
		res, err := playOneLine(Options{Title: "Synergy™"}, 40, 300*time.Millisecond)
		if err != nil {
			t.Fatalf("playOneLine: %v", err)
		}
		if !res.Finished {
			t.Error("Finished = false")
		}
		if res.Duration != 300*time.Millisecond {
			t.Errorf("Duration = %v", res.Duration)
		}
	})
	if !strings.Contains(out, "AD · Synergy™") {
		t.Errorf("output missing ad line: %q", out)
	}
	if strings.Contains(out, "\x1b[?1049l") {
		t.Errorf("one-line tier must not touch the alt screen: %q", out)
	}
}

// captureStdout redirects os.Stdout into a temp file while fn runs and
// returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old }()

	fn()

	os.Stdout = old
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
