package render

import (
	"fmt"
	"image"
	"strings"
)

// HalfBlock renders frame scaled down into a cols x rows cell grid using the
// upper half block glyph: each cell's foreground is the top half, its
// background the bottom half, giving two vertical pixels per row of cells.
func HalfBlock(frame image.Image, cols, rows int) string {
	if frame == nil || cols < 1 || rows < 1 {
		return ""
	}
	b := frame.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return ""
	}

	var sb strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			top := sampleAt(frame, b,
				b.Min.X+(2*x+1)*w/(2*cols), b.Min.Y+(4*y+1)*h/(4*rows))
			bot := sampleAt(frame, b,
				b.Min.X+(2*x+1)*w/(2*cols), b.Min.Y+(4*y+3)*h/(4*rows))
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀",
				top[0], top[1], top[2], bot[0], bot[1], bot[2])
		}
		sb.WriteString("\x1b[0m")
		if y < rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func sampleAt(img image.Image, b image.Rectangle, x, y int) [3]uint8 {
	if x < b.Min.X {
		x = b.Min.X
	}
	if x >= b.Max.X {
		x = b.Max.X - 1
	}
	if y < b.Min.Y {
		y = b.Min.Y
	}
	if y >= b.Max.Y {
		y = b.Max.Y - 1
	}
	r, g, bl, _ := img.At(x, y).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
}
