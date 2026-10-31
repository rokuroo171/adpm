package render

import (
	"fmt"
	"image"
	"image/color"
	"strings"
)

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
		for x := 0; x < cols; x++ {
			top := sampleRegion(frame, b,
				b.Min.X+x*w/cols, b.Min.Y+y*h/(2*rows),
				b.Min.X+(x+1)*w/cols, b.Min.Y+(2*y+1)*h/(2*rows))
			bot := sampleRegion(frame, b,
				b.Min.X+x*w/cols, b.Min.Y+(2*y+1)*h/(2*rows),
				b.Min.X+(x+1)*w/cols, b.Min.Y+(2*y+2)*h/(2*rows))
			if indexed {
				fmt.Fprintf(&sb, "\x1b[38;5;%d;48;5;%dm▀",
					XTerm256(color.RGBA{top[0], top[1], top[2], 255}),
					XTerm256(color.RGBA{bot[0], bot[1], bot[2], 255}))
			} else {
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀",
					top[0], top[1], top[2], bot[0], bot[1], bot[2])
			}
		}
		sb.WriteString("\x1b[0m")
		if y < rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
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
