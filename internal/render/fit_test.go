package render

import (
	"fmt"
	"image"
	"image/color"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFit(t *testing.T) {
	tests := []struct {
		name               string
		frameW, frameH     int
		cols, rows         int
		cellW, cellH       int
		wantCols, wantRows int
	}{
		{"80x24", 320, 180, 80, 24, 8, 16, 80, 23},
		{"40x20", 320, 180, 40, 20, 8, 16, 40, 12},
		{"30x10", 320, 180, 30, 10, 8, 16, 30, 9},
		{"20x6", 320, 180, 20, 6, 8, 16, 20, 6},
		{"1x1", 320, 180, 1, 1, 8, 16, 1, 1},
		{"0x0", 320, 180, 0, 0, 8, 16, 1, 1},
		{"90x70 fills width without exceeding height cap", 320, 180, 90, 70, 8, 16, 90, 26},
		{"400x100 caps box at 100x36", 320, 180, 400, 100, 8, 16, 100, 29},
		{"300x8 fits width only", 320, 180, 300, 8, 8, 16, 29, 8},
		{"1000x1000", 320, 180, 1000, 1000, 8, 16, 100, 29},
		{"pixel size reported 0 falls back to 1:2", 320, 180, 80, 24, 0, 0, 80, 23},
		{"cell aspect 1:2 fallback also for negative", 320, 180, 40, 20, -1, -1, 40, 12},
		{"zero frame dimensions are treated as 1x1", 0, 0, 80, 24, 8, 16, 48, 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCols, gotRows := Fit(tt.frameW, tt.frameH, tt.cols, tt.rows, tt.cellW, tt.cellH)
			if gotCols != tt.wantCols || gotRows != tt.wantRows {
				t.Errorf("Fit(%d, %d, %d, %d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.frameW, tt.frameH, tt.cols, tt.rows, tt.cellW, tt.cellH,
					gotCols, gotRows, tt.wantCols, tt.wantRows)
			}
		})
	}
}

// The fitted box must stay inside the terminal and the 100x36 cap.
func TestFitStaysInsideBounds(t *testing.T) {
	type tc struct{ frameW, frameH, cols, rows, cellW, cellH int }
	cases := []tc{
		{320, 180, 80, 24, 8, 16}, {320, 240, 40, 20, 8, 16},
		{320, 180, 120, 40, 10, 20}, {320, 180, 5, 3, 4, 8},
		{1, 1, 60, 18, 8, 16}, {320, 180, 3, 1, 1, 2},
	}
	for _, c := range cases {
		gotCols, gotRows := Fit(c.frameW, c.frameH, c.cols, c.rows, c.cellW, c.cellH)
		if gotCols < 1 || gotRows < 1 {
			t.Errorf("Fit(%v) = (%d,%d): below minimum 1", c, gotCols, gotRows)
		}
		wantMaxCols := min(c.cols, 100)
		wantMaxRows := min(c.rows, 36)
		if wantMaxCols < 1 {
			wantMaxCols = 1
		}
		if wantMaxRows < 1 {
			wantMaxRows = 1
		}
		if gotCols > wantMaxCols || gotRows > wantMaxRows {
			t.Errorf("Fit(%v) = (%d,%d): exceeds cap (%d,%d)", c, gotCols, gotRows, wantMaxCols, wantMaxRows)
		}
	}
}

func TestModeFor(t *testing.T) {
	tests := []struct {
		cols, rows int
		want       Mode
	}{
		{80, 24, ModeFull},
		{40, 20, ModeFull},
		{39, 20, ModeCompact},
		{40, 11, ModeCompact},
		{30, 10, ModeCompact},
		{20, 6, ModeCompact},
		{19, 6, ModeOneLine},
		{20, 5, ModeOneLine},
		{1, 1, ModeOneLine},
		{0, 0, ModeOneLine},
	}
	for _, tt := range tests {
		if got := ModeFor(tt.cols, tt.rows); got != tt.want {
			t.Errorf("ModeFor(%d, %d) = %v, want %v", tt.cols, tt.rows, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		w    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"Synergy™ · 12s", 8, "Synergy…"},
		{"hi", 1, "…"},
		{"hi", 0, ""},
	}
	for _, tt := range tests {
		if got := Truncate(tt.s, tt.w); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
		}
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestHalfBlock(t *testing.T) {
	// Each cell row is red on top, blue on the bottom: the color boundary
	// sits at every quarter of the image height.
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		c := color.RGBA{255, 0, 0, 255}
		if y%8 >= 4 {
			c = color.RGBA{0, 0, 255, 255}
		}
		for x := 0; x < 32; x++ {
			img.Set(x, y, c)
		}
	}

	got := HalfBlock(img, 4, 2)
	if !strings.Contains(got, "\x1b[38;2;255;0;0;48;2;0;0;255m▀") {
		t.Errorf("expected red-over-blue cells, got %q", got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Errorf("got %d lines, want 2", len(lines))
	}
	// One reset sequence per line, 4 cells per line, nothing outside cells.
	for i, line := range lines {
		if !strings.HasSuffix(line, "\x1b[0m") {
			t.Errorf("line %d missing reset: %q", i, line)
		}
		cells := strings.Count(line, "▀")
		if cells != 4 {
			t.Errorf("line %d has %d cells, want 4", i, cells)
		}
		if ansiRe.ReplaceAllString(line, "") != strings.Repeat("▀", 4) {
			t.Errorf("line %d has stray content: %q", i, line)
		}
	}
}

func TestHalfBlockOptimizedSerializerPreservesVirtualCellGrid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 12, 8))
	colors := [][3]uint8{
		{240, 20, 10}, {15, 210, 30}, {25, 40, 230},
		{230, 200, 15}, {180, 25, 210},
	}
	for row := 0; row < 4; row++ {
		for col := 0; col < 6; col++ {
			top := colors[(row+col)%3]
			bottom := top
			if col%3 != 0 {
				bottom = colors[(row+col+2)%len(colors)]
			}
			for x := col * 2; x < col*2+2; x++ {
				img.Set(x, row*2, color.RGBA{R: top[0], G: top[1], B: top[2], A: 255})
				img.Set(x, row*2+1, color.RGBA{R: bottom[0], G: bottom[1], B: bottom[2], A: 255})
			}
		}
	}

	const cols, rows = 6, 4
	legacy := legacyHalfBlock(img, cols, rows)
	optimized := HalfBlock(img, cols, rows)
	if got, want := renderHalfBlockPixels(optimized, cols, rows), renderHalfBlockPixels(legacy, cols, rows); !equalHalfBlockPixels(got, want) {
		t.Fatal("optimized half-block output changes the rendered cell grid")
	}
	if len(optimized) >= len(legacy) {
		t.Fatalf("optimized output is %d bytes, legacy output is %d bytes", len(optimized), len(legacy))
	}
}

func legacyHalfBlock(frame image.Image, cols, rows int) string {
	b := frame.Bounds()
	w, h := b.Dx(), b.Dy()
	var out strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			top := sampleRegion(frame, b, b.Min.X+x*w/cols, b.Min.Y+y*h/(2*rows),
				b.Min.X+(x+1)*w/cols, b.Min.Y+(2*y+1)*h/(2*rows))
			bottom := sampleRegion(frame, b, b.Min.X+x*w/cols, b.Min.Y+(2*y+1)*h/(2*rows),
				b.Min.X+(x+1)*w/cols, b.Min.Y+(2*y+2)*h/(2*rows))
			fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", top[0], top[1], top[2], bottom[0], bottom[1], bottom[2])
		}
		out.WriteString("\x1b[0m")
		if y < rows-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func renderHalfBlockPixels(s string, cols, rows int) [][3]uint8 {
	pixels := make([][3]uint8, cols*rows*2)
	var foreground, background [3]uint8
	x, y := 0, 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				break
			}
			applyHalfBlockSGR(s[i+2:i+end], &foreground, &background)
			i += end + 1
			continue
		}
		if s[i] == '\n' {
			x, y = 0, y+1
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '▀':
			if x < cols && y < rows {
				at := (y*cols + x) * 2
				pixels[at], pixels[at+1] = foreground, background
			}
			x++
		case ' ':
			if x < cols && y < rows {
				at := (y*cols + x) * 2
				pixels[at], pixels[at+1] = background, background
			}
			x++
		}
		i += size
	}
	return pixels
}

func applyHalfBlockSGR(params string, foreground, background *[3]uint8) {
	values := strings.Split(params, ";")
	for i := 0; i < len(values); i++ {
		if values[i] == "0" {
			*foreground, *background = [3]uint8{}, [3]uint8{}
			continue
		}
		if (values[i] != "38" && values[i] != "48") || i+4 >= len(values) || values[i+1] != "2" {
			continue
		}
		c := [3]uint8{uint8(parseHalfBlockByte(values[i+2])), uint8(parseHalfBlockByte(values[i+3])), uint8(parseHalfBlockByte(values[i+4]))}
		if values[i] == "38" {
			*foreground = c
		} else {
			*background = c
		}
		i += 4
	}
}

func parseHalfBlockByte(value string) int {
	var out int
	for _, digit := range value {
		out = out*10 + int(digit-'0')
	}
	return out
}

func equalHalfBlockPixels(a, b [][3]uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHalfBlockEdgeCases(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for _, tt := range []struct {
		f    image.Image
		cols int
		rows int
	}{
		{nil, 4, 4},
		{img, 0, 4},
		{img, 4, 0},
		{img, -1, -1},
	} {
		if got := HalfBlock(tt.f, tt.cols, tt.rows); got != "" {
			t.Errorf("HalfBlock(nil/zero size) = %q, want empty", got)
		}
	}
}

func TestSampleRegionAveragesDownscaledPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		img.SetRGBA(0, y, color.RGBA{R: 255, A: 255})
		img.SetRGBA(1, y, color.RGBA{R: 255, A: 255})
		img.SetRGBA(2, y, color.RGBA{B: 255, A: 255})
		img.SetRGBA(3, y, color.RGBA{B: 255, A: 255})
	}
	got := sampleRegion(img, img.Bounds(), 0, 0, 4, 2)
	if got != ([3]uint8{127, 0, 127}) {
		t.Fatalf("sampleRegion = %v, want 127/0/127", got)
	}
}

func TestOverlayGridClipsAtScreenEdges(t *testing.T) {
	makeGrid := func(rows ...string) CellGrid {
		grid := CellGrid{Width: len([]rune(rows[0])), Height: len(rows)}
		for _, row := range rows {
			for _, r := range row {
				grid.Cells = append(grid.Cells, Cell{Rune: r})
			}
		}
		return grid
	}
	text := func(grid CellGrid) string {
		var out strings.Builder
		for y := 0; y < grid.Height; y++ {
			if y > 0 {
				out.WriteByte('\n')
			}
			for x := 0; x < grid.Width; x++ {
				out.WriteRune(grid.Cells[y*grid.Width+x].Rune)
			}
		}
		return out.String()
	}

	base := makeGrid("...", "...", "...")
	panel := makeGrid("AB", "CD")
	tests := []struct {
		name  string
		base  CellGrid
		panel CellGrid
		x, y  int
		want  string
	}{
		{"touch top", base, panel, 0, 0, "AB.\nCD.\n..."},
		{"touch right", base, panel, 1, 0, ".AB\n.CD\n..."},
		{"touch bottom", base, panel, 0, 1, "...\nAB.\nCD."},
		{"touch left", base, panel, 0, 0, "AB.\nCD.\n..."},
		{"panel larger than screen", makeGrid("..", ".."), makeGrid("ABCD", "EFGH", "IJKL"), -1, -1, "FG\nJK"},
		{"one cell", makeGrid("."), makeGrid("X"), 0, 0, "X"},
		{"zero sized screen", CellGrid{}, panel, 0, 0, ""},
		{"zero sized panel", base, CellGrid{}, 0, 0, "...\n...\n..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := text(OverlayGrid(tt.base, tt.panel, tt.x, tt.y)); got != tt.want {
				t.Fatalf("OverlayGrid() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestANSIGridPreservesColorsAndAttributes(t *testing.T) {
	grid := ParseANSIGrid("\x1b[1;38;2;12;34;56;48;5;17mA\x1b[0mB", 2, 1, func(rune) int { return 1 })
	got := RenderGrid(grid)
	if !strings.Contains(got, "\x1b[0;1;38;2;12;34;56;48;5;17mA") {
		t.Fatalf("rendered grid lost cell style: %q", got)
	}
	if strings.ReplaceAll(ansiRe.ReplaceAllString(got, ""), "\x1b[0m", "") != "AB" {
		t.Fatalf("rendered grid text = %q, want AB", got)
	}
}

func TestDimGridScalesColorsAndFaintsASCII(t *testing.T) {
	colorGrid := ParseANSIGrid("\x1b[38;2;200;100;40mX", 1, 1, func(rune) int { return 1 })
	dim := DimGrid(colorGrid, 0.4, false)
	if got := dim.Cells[0].Foreground; got != (Color{Mode: ColorRGB, R: 80, G: 40, B: 16}) {
		t.Fatalf("dimmed foreground = %+v, want 80/40/16", got)
	}
	ascii := DimGrid(ParseANSIGrid("x", 1, 1, func(rune) int { return 1 }), 0.4, true)
	if !ascii.Cells[0].Faint {
		t.Fatal("ASCII grid was not fainted")
	}
}
