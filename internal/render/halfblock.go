package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"
)

type ColorMode uint8

const (
	ColorDefault ColorMode = iota
	ColorANSI
	ColorPalette
	ColorRGB
)

type Color struct {
	Mode  ColorMode
	Index uint8
	R     uint8
	G     uint8
	B     uint8
}

type Cell struct {
	Rune       rune
	Foreground Color
	Background Color
	Bold       bool
	Faint      bool
	Reverse    bool
	Underline  bool
	Continue   bool
}

type CellGrid struct {
	Width  int
	Height int
	Cells  []Cell
}

func NewCellGrid(width, height int) CellGrid {
	if width < 1 || height < 1 {
		return CellGrid{}
	}
	g := CellGrid{Width: width, Height: height, Cells: make([]Cell, width*height)}
	for i := range g.Cells {
		g.Cells[i].Rune = ' '
	}
	return g
}

func ParseANSIGrid(s string, width, height int, runeWidth func(rune) int) CellGrid {
	g := NewCellGrid(width, height)
	if g.Width == 0 || runeWidth == nil {
		return g
	}
	style := Cell{}
	x, y := 0, 0
	for i := 0; i < len(s) && y < g.Height; {
		if s[i] == '\x1b' {
			end := ansiEnd(s, i)
			if i+1 < len(s) && s[i+1] == '[' && end > i+2 && s[end-1] == 'm' {
				applySGR(&style, s[i+2:end-1])
			}
			i = end
			continue
		}
		if s[i] == '\n' {
			y++
			x = 0
			i++
			continue
		}
		if s[i] == '\r' {
			x = 0
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := runeWidth(r)
		if w < 0 {
			w = 1
		}
		if w > 0 {
			if x >= 0 && x < g.Width {
				cell := style
				cell.Rune = r
				g.Cells[y*g.Width+x] = cell
				for offset := 1; offset < w && x+offset < g.Width; offset++ {
					continuation := style
					continuation.Continue = true
					g.Cells[y*g.Width+x+offset] = continuation
				}
			}
			x += w
		}
		i += size
	}
	return g
}

func OverlayGrid(base, overlay CellGrid, x, y int) CellGrid {
	out := NewCellGrid(base.Width, base.Height)
	if out.Width == 0 {
		return out
	}
	copy(out.Cells, base.Cells[:min(len(base.Cells), len(out.Cells))])
	if overlay.Width < 1 || overlay.Height < 1 {
		return out
	}
	for oy := 0; oy < overlay.Height; oy++ {
		dy := y + oy
		if dy < 0 || dy >= out.Height {
			continue
		}
		for ox := 0; ox < overlay.Width; ox++ {
			dx := x + ox
			if dx < 0 || dx >= out.Width {
				continue
			}
			out.Cells[dy*out.Width+dx] = overlay.Cells[oy*overlay.Width+ox]
		}
	}
	return out
}

func DimGrid(grid CellGrid, factor float64, faintOnly bool) CellGrid {
	out := CellGrid{Width: grid.Width, Height: grid.Height, Cells: append([]Cell(nil), grid.Cells...)}
	factor = min(max(factor, 0), 1)
	for i := range out.Cells {
		cell := &out.Cells[i]
		if faintOnly {
			cell.Faint = true
			continue
		}
		if cell.Foreground.Mode == ColorANSI {
			cell.Faint = true
		} else {
			cell.Foreground = dimColor(cell.Foreground, factor)
		}
		cell.Background = dimColor(cell.Background, factor)
		if cell.Foreground.Mode == ColorDefault && cell.Rune != ' ' && !cell.Continue {
			cell.Faint = true
		}
	}
	return out
}

func RenderGrid(grid CellGrid) string {
	if grid.Width < 1 || grid.Height < 1 || len(grid.Cells) < grid.Width*grid.Height {
		return ""
	}
	var out strings.Builder
	var previous Cell
	styleSet := false
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			cell := grid.Cells[y*grid.Width+x]
			if !styleSet || !sameStyle(previous, cell) {
				out.WriteString(cellSGR(cell))
				previous = cell
				styleSet = true
			}
			if !cell.Continue {
				r := cell.Rune
				if r == 0 {
					r = ' '
				}
				out.WriteRune(r)
			}
		}
		out.WriteString("\x1b[0m")
		styleSet = false
		if y < grid.Height-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func ansiEnd(s string, start int) int {
	if start+1 >= len(s) {
		return len(s)
	}
	if s[start+1] == '[' {
		for i := start + 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
	}
	return min(start+2, len(s))
}

func applySGR(style *Cell, params string) {
	values := []int{0}
	if params != "" {
		values = values[:0]
		for _, part := range strings.Split(params, ";") {
			value := 0
			for _, digit := range part {
				if digit < '0' || digit > '9' {
					value = 0
					break
				}
				value = value*10 + int(digit-'0')
			}
			values = append(values, value)
		}
	}
	for i := 0; i < len(values); i++ {
		value := values[i]
		switch {
		case value == 0:
			*style = Cell{}
		case value == 1:
			style.Bold = true
		case value == 2:
			style.Faint = true
		case value == 4:
			style.Underline = true
		case value == 7:
			style.Reverse = true
		case value == 22:
			style.Bold, style.Faint = false, false
		case value == 24:
			style.Underline = false
		case value == 27:
			style.Reverse = false
		case value >= 30 && value <= 37:
			style.Foreground = Color{Mode: ColorANSI, Index: uint8(value - 30)}
		case value >= 90 && value <= 97:
			style.Foreground = Color{Mode: ColorANSI, Index: uint8(value - 90 + 8)}
		case value == 39:
			style.Foreground = Color{}
		case value >= 40 && value <= 47:
			style.Background = Color{Mode: ColorANSI, Index: uint8(value - 40)}
		case value >= 100 && value <= 107:
			style.Background = Color{Mode: ColorANSI, Index: uint8(value - 100 + 8)}
		case value == 49:
			style.Background = Color{}
		case (value == 38 || value == 48) && i+1 < len(values):
			target := &style.Foreground
			if value == 48 {
				target = &style.Background
			}
			switch values[i+1] {
			case 5:
				if i+2 < len(values) {
					*target = Color{Mode: ColorPalette, Index: uint8(values[i+2])}
					i += 2
				}
			case 2:
				if i+4 < len(values) {
					*target = Color{Mode: ColorRGB, R: uint8(values[i+2]), G: uint8(values[i+3]), B: uint8(values[i+4])}
					i += 4
				}
			}
		}
	}
}

func sameStyle(a, b Cell) bool {
	return a.Foreground == b.Foreground && a.Background == b.Background &&
		a.Bold == b.Bold && a.Faint == b.Faint && a.Reverse == b.Reverse && a.Underline == b.Underline
}

func cellSGR(cell Cell) string {
	values := []string{"0"}
	if cell.Bold {
		values = append(values, "1")
	}
	if cell.Faint {
		values = append(values, "2")
	}
	if cell.Underline {
		values = append(values, "4")
	}
	if cell.Reverse {
		values = append(values, "7")
	}
	values = appendColorSGR(values, cell.Foreground, false)
	values = appendColorSGR(values, cell.Background, true)
	if len(values) == 0 {
		return "\x1b[0m"
	}
	return "\x1b[" + strings.Join(values, ";") + "m"
}

func appendColorSGR(values []string, c Color, background bool) []string {
	base := 38
	if background {
		base = 48
	}
	switch c.Mode {
	case ColorANSI:
		if c.Index < 8 {
			code := 30 + int(c.Index)
			if background {
				code = 40 + int(c.Index)
			}
			return append(values, fmt.Sprint(code))
		}
		code := 90 + int(c.Index) - 8
		if background {
			code = 100 + int(c.Index) - 8
		}
		return append(values, fmt.Sprint(code))
	case ColorPalette:
		return append(values, fmt.Sprint(base), "5", fmt.Sprint(c.Index))
	case ColorRGB:
		return append(values, fmt.Sprint(base), "2", fmt.Sprint(c.R), fmt.Sprint(c.G), fmt.Sprint(c.B))
	default:
		return values
	}
}

func dimColor(c Color, factor float64) Color {
	if c.Mode == ColorDefault {
		return c
	}
	r, g, b := colorRGB(c)
	r = uint8(math.Round(float64(r) * factor))
	g = uint8(math.Round(float64(g) * factor))
	b = uint8(math.Round(float64(b) * factor))
	switch c.Mode {
	case ColorRGB:
		return Color{Mode: ColorRGB, R: r, G: g, B: b}
	case ColorPalette:
		return Color{Mode: ColorPalette, Index: XTerm256(color.RGBA{R: r, G: g, B: b, A: 255})}
	default:
		return Color{Mode: ColorANSI, Index: nearestANSI(r, g, b)}
	}
}

func colorRGB(c Color) (uint8, uint8, uint8) {
	if c.Mode == ColorRGB {
		return c.R, c.G, c.B
	}
	if c.Mode == ColorANSI && c.Index < 16 {
		return ansiPalette[c.Index][0], ansiPalette[c.Index][1], ansiPalette[c.Index][2]
	}
	if c.Index < 16 {
		return ansiPalette[c.Index][0], ansiPalette[c.Index][1], ansiPalette[c.Index][2]
	}
	if c.Index < 232 {
		v := int(c.Index) - 16
		return cubeLevel(v / 36), cubeLevel(v / 6 % 6), cubeLevel(v % 6)
	}
	gray := uint8(8 + (int(c.Index)-232)*10)
	return gray, gray, gray
}

var ansiPalette = [16][3]uint8{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0},
	{0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
	{92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

func cubeLevel(value int) uint8 {
	if value == 0 {
		return 0
	}
	return uint8(55 + 40*value)
}

func nearestANSI(r, g, b uint8) uint8 {
	best, distance := uint8(0), int(^uint(0)>>1)
	for i, c := range ansiPalette {
		dr, dg, db := int(r)-int(c[0]), int(g)-int(c[1]), int(b)-int(c[2])
		if current := dr*dr + dg*dg + db*db; current < distance {
			best, distance = uint8(i), current
		}
	}
	return best
}

// HalfBlock renders frame scaled down into a cols x rows cell grid using the
// upper half block glyph: each cell's foreground is the top half, its
// background the bottom half, giving two vertical pixels per row of cells.
func HalfBlock(frame image.Image, cols, rows int) string {
	return halfBlock(frame, cols, rows, false)
}

func HalfBlock256(frame image.Image, cols, rows int) string {
	return halfBlock(frame, cols, rows, true)
}

func halfBlock(frame image.Image, cols, rows int, indexed bool) string {
	if frame == nil || cols < 1 || rows < 1 {
		return ""
	}
	b := frame.Bounds()
	if _, ok := frame.(*image.Uniform); ok {
		b = image.Rect(0, 0, 1, 1)
	}
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return ""
	}

	var sb strings.Builder
	for y := 0; y < rows; y++ {
		var previousTop, previousBottom [3]uint8
		hasForeground, hasBackground := false, false
		for x := 0; x < cols; x++ {
			top := sampleRegion(frame, b,
				b.Min.X+x*w/cols, b.Min.Y+y*h/(2*rows),
				b.Min.X+(x+1)*w/cols, b.Min.Y+(2*y+1)*h/(2*rows))
			bot := sampleRegion(frame, b,
				b.Min.X+x*w/cols, b.Min.Y+(2*y+1)*h/(2*rows),
				b.Min.X+(x+1)*w/cols, b.Min.Y+(2*y+2)*h/(2*rows))
			if indexed {
				top = [3]uint8{XTerm256(color.RGBA{top[0], top[1], top[2], 255})}
				bot = [3]uint8{XTerm256(color.RGBA{bot[0], bot[1], bot[2], 255})}
			}
			sameColor := top == bot
			foregroundChanged := !hasForeground || top != previousTop
			backgroundChanged := !hasBackground || bot != previousBottom
			if sameColor {
				if backgroundChanged {
					sb.WriteString("\x1b[")
					writeHalfBlockColor(&sb, bot, indexed, true)
					sb.WriteByte('m')
				}
				sb.WriteByte(' ')
				previousBottom, hasBackground = bot, true
				continue
			}
			if foregroundChanged || backgroundChanged {
				sb.WriteString("\x1b[")
				if foregroundChanged {
					writeHalfBlockColor(&sb, top, indexed, false)
				}
				if backgroundChanged {
					if foregroundChanged {
						sb.WriteByte(';')
					}
					writeHalfBlockColor(&sb, bot, indexed, true)
				}
				sb.WriteByte('m')
			}
			sb.WriteRune('▀')
			previousTop, previousBottom = top, bot
			hasForeground, hasBackground = true, true
		}
		sb.WriteString("\x1b[0m")
		if y < rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func writeHalfBlockColor(out *strings.Builder, c [3]uint8, indexed, background bool) {
	code := 38
	if background {
		code = 48
	}
	if indexed {
		fmt.Fprintf(out, "%d;5;%d", code, c[0])
		return
	}
	fmt.Fprintf(out, "%d;2;%d;%d;%d", code, c[0], c[1], c[2])
}

func sampleRegion(img image.Image, b image.Rectangle, x0, y0, x1, y1 int) [3]uint8 {
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x1 <= x0 || y1 <= y0 {
		x := min(max((x0+x1)/2, b.Min.X), b.Max.X-1)
		y := min(max((y0+y1)/2, b.Min.Y), b.Max.Y-1)
		r, g, bl, _ := img.At(x, y).RGBA()
		return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
	}
	var rSum, gSum, bSum, samples uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			rSum += uint64(r >> 8)
			gSum += uint64(g >> 8)
			bSum += uint64(bl >> 8)
			samples++
		}
	}
	return [3]uint8{uint8(rSum / samples), uint8(gSum / samples), uint8(bSum / samples)}
}
