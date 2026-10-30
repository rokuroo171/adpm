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
		frames: testFrames(10),
		title:  "Synergy™",
		total:  10 * frameInterval,
		start:  time.Now(),
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
		{20, 6, render.ModeCompact, "AD · "},
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

func TestKeys(t *testing.T) {
	tests := []struct {
		name       string
		key        tea.KeyType
		wantCancel bool
		wantSuspen bool
	}{
		{"ctrl+c ends the ad early", tea.KeyCtrlC, true, false},
		{"ctrl+z suspends", tea.KeyCtrlZ, false, true},
		{"enter does nothing yet", tea.KeyEnter, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t, 80, 24)
			_, cmd := m.Update(tea.KeyMsg{Type: tt.key})
			if m.result.Cancelled != tt.wantCancel {
				t.Errorf("Cancelled = %v, want %v", m.result.Cancelled, tt.wantCancel)
			}
			if m.result.Suspended != tt.wantSuspen {
				t.Errorf("Suspended = %v, want %v", m.result.Suspended, tt.wantSuspen)
			}
			wantCmd := tt.key != tea.KeyEnter
			if (cmd != nil) != wantCmd {
				t.Errorf("cmd present = %v, want %v", cmd != nil, wantCmd)
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
}

// The one-line tier runs without the alt screen, prints one line, and ends
// finished.
func TestPlayOneLine(t *testing.T) {
	out := captureStdout(t, func() {
		res, err := playOneLine("Synergy™", 40, 300*time.Millisecond)
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
