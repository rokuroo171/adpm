package render

import (
	"image"
	"image/color"
	"io"
	"strings"

	"github.com/kenshaw/rasterm"
)

type Tier uint8

const (
	TierKitty Tier = iota
	TierSixel
	TierITerm2
	TierHalfBlock
	TierASCII
)

type Capabilities struct {
	KittyReply  bool
	DA1         string
	TermProgram string
	Term        string
	ColorTerm   string
	NoColor     bool
	NoImages    bool
}

func SelectTier(caps Capabilities) Tier {
	if caps.NoImages {
		return TierHalfBlock
	}
	if caps.NoColor || caps.Term == "linux" || caps.Term == "dumb" {
		return TierASCII
	}
	if caps.KittyReply {
		return TierKitty
	}
	if supportsSixel(caps.DA1) {
		return TierSixel
	}
	if strings.EqualFold(caps.TermProgram, "iTerm.app") ||
		strings.EqualFold(caps.TermProgram, "iTerm2") ||
		strings.EqualFold(caps.TermProgram, "WezTerm") {
		return TierITerm2
	}
	return TierHalfBlock
}

func EncodeImage(w io.Writer, tier Tier, frame image.Image) error {
	var encoder rasterm.Encoder
	switch tier {
	case TierKitty:
		encoder = rasterm.KittyEncoder{NoNewline: true}
	case TierSixel:
		encoder = rasterm.SixelEncoder{NoNewline: true}
	case TierITerm2:
		encoder = rasterm.ITermEncoder{NoNewline: true}
	default:
		return rasterm.ErrTermGraphicsNotAvailable
	}
	return encoder.Encode(w, frame)
}

func supportsSixel(da1 string) bool {
	if !strings.HasPrefix(da1, "\x1b[?") || !strings.HasSuffix(da1, "c") {
		return false
	}
	for _, parameter := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(da1, "\x1b[?"), "c"), ";") {
		if parameter == "4" {
			return true
		}
	}
	return false
}

func ASCII(frame image.Image, cols, rows int) string {
	if frame == nil || cols < 1 || rows < 1 {
		return ""
	}
	const ramp = " .:-=+*#%@"
	bounds := frame.Bounds()
	if _, ok := frame.(*image.Uniform); ok {
		bounds = image.Rect(0, 0, 1, 1)
	}
	w, h := bounds.Dx(), bounds.Dy()
	if w < 1 || h < 1 {
		return ""
	}
	var out strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			pixel := sampleRegion(frame, bounds,
				bounds.Min.X+x*w/cols, bounds.Min.Y+y*h/rows,
				bounds.Min.X+(x+1)*w/cols, bounds.Min.Y+(y+1)*h/rows)
			r, g, b := uint32(pixel[0])*257, uint32(pixel[1])*257, uint32(pixel[2])*257
			luminance := (299*r + 587*g + 114*b) / 1000
			index := int(luminance * uint32(len(ramp)-1) / 65535)
			out.WriteByte(ramp[index])
		}
		if y < rows-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func XTerm256(c color.Color) uint8 {
	r16, g16, b16, _ := c.RGBA()
	r, g, b := int(r16>>8), int(g16>>8), int(b16>>8)
	levels := [6]int{0, 95, 135, 175, 215, 255}
	nearest := func(v int) int {
		ix := (v - 55) / 40
		if v < 48 {
			return 0
		}
		if ix < 0 {
			return 0
		}
		if ix >= 5 {
			return 5
		}
		if v-levels[ix] > levels[ix+1]-v {
			return ix + 1
		}
		return ix
	}
	ri, gi, bi := nearest(r), nearest(g), nearest(b)
	cube := [3]int{levels[ri], levels[gi], levels[bi]}
	cubeDistance := squaredDistance(r, g, b, cube[0], cube[1], cube[2])
	gray := (r + g + b) / 3
	grayIndex := (gray - 8 + 5) / 10
	if grayIndex < 0 {
		grayIndex = 0
	}
	if grayIndex > 23 {
		grayIndex = 23
	}
	grayValue := 8 + 10*grayIndex
	if squaredDistance(r, g, b, grayValue, grayValue, grayValue) < cubeDistance {
		return uint8(232 + grayIndex)
	}
	return uint8(16 + 36*ri + 6*gi + bi)
}

func squaredDistance(r, g, b, pr, pg, pb int) int {
	dr, dg, db := r-pr, g-pg, b-pb
	return dr*dr + dg*dg + db*db
}
