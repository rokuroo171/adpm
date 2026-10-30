package tui

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
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

// The three sizes from the spec's done-when must render without garbling:
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
	if !strings.Contains(view, "^C ?") || !strings.Contains(view, "r report") {
		t.Fatalf("narrow footer missing short controls: %q", view)
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

func TestRewardPopupClearsKittyImages(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.tier = render.TierKitty
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	view := m.View()
	if !strings.Contains(view, "\x1b_Ga=d,d=A\x1b\\") {
		t.Error("reward popup did not clear Kitty images")
	}
	if strings.Contains(view, "\x1b_Ga=T,") {
		t.Error("reward popup redraw contains a Kitty image")
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

func TestKeys(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.rewardLines = []string{"Reward line one", "Reward line two"}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.result.Cancelled {
		t.Fatal("Ctrl+C cancelled before the popup was confirmed")
	}
	if !strings.Contains(m.View(), "Reward line one") {
		t.Fatalf("Ctrl+C did not show the reward popup: %q", m.View())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.result.Cancelled {
		t.Error("Enter on the default Cancel button did not cancel")
	}

	m = newTestModel(t, 80, 24)
	m.rewardLines = []string{"Reward line one", "Reward line two"}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.result.Cancelled {
		t.Error("choosing OK cancelled the install")
	}
	if strings.Contains(m.View(), "You won't get your reward") {
		t.Errorf("choosing OK left the popup open")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(m.View(), "Reward line two") {
		t.Errorf("reward popup did not rotate its message: %q", m.View())
	}

	m = newTestModel(t, 80, 24)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !strings.Contains(m.View(), "Thank you. Your report will be reviewed") {
		t.Fatalf("r did not show the report popup: %q", m.View())
	}
	oldStart := m.start
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.start.After(oldStart) {
		t.Error("dismissing the report did not restart the ad")
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
	if m.result.Finished || cmd == nil {
		t.Fatal("the ad finished while the reward popup was open")
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
