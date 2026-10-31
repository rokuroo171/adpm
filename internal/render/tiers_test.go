package render

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestSelectTier(t *testing.T) {
	tests := []struct {
		name string
		caps Capabilities
		want Tier
	}{
		{"kitty reply wins", Capabilities{KittyReply: true, DA1: "?4;1c"}, TierKitty},
		{"sixel DA1 reply", Capabilities{DA1: "\x1b[?4;1c"}, TierSixel},
		{"iTerm hint", Capabilities{TermProgram: "iTerm.app"}, TierITerm2},
		{"WezTerm sixel reply", Capabilities{TermProgram: "WezTerm", DA1: "\x1b[?65;4;6;22c"}, TierSixel},
		{"WezTerm iTerm hint", Capabilities{TermProgram: "WezTerm"}, TierITerm2},
		{"Alacritty fallback", Capabilities{Term: "alacritty", DA1: "\x1b[?62;1;6;22c"}, TierHalfBlock},
		{"linux console", Capabilities{Term: "linux", ColorTerm: "truecolor"}, TierASCII},
		{"dumb terminal", Capabilities{Term: "dumb"}, TierASCII},
		{"NO_COLOR", Capabilities{NoColor: true}, TierASCII},
		{"no images forces half-block", Capabilities{NoImages: true, KittyReply: true}, TierHalfBlock},
		{"unknown terminal defaults to half-block", Capabilities{ColorTerm: "truecolor"}, TierHalfBlock},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SelectTier(tt.caps); got != tt.want {
				t.Errorf("SelectTier(%+v) = %v, want %v", tt.caps, got, tt.want)
			}
		})
	}
}

func TestASCII(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 1))
	img.Set(0, 0, color.Black)
	img.Set(1, 0, color.Gray{Y: 85})
	img.Set(2, 0, color.Gray{Y: 170})
	img.Set(3, 0, color.White)

	got := ASCII(img, 4, 1)
	if got != " -*@" {
		t.Errorf("ASCII = %q, want %q", got, " -*@")
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("ASCII output contains ANSI escapes: %q", got)
	}
}

func TestXTerm256(t *testing.T) {
	for _, tt := range []struct {
		color color.RGBA
		want  uint8
	}{
		{color.RGBA{255, 0, 0, 255}, 196},
		{color.RGBA{0, 0, 255, 255}, 21},
		{color.RGBA{128, 128, 128, 255}, 244},
	} {
		if got := XTerm256(tt.color); got != tt.want {
			t.Errorf("XTerm256(%v) = %d, want %d", tt.color, got, tt.want)
		}
	}
}

func TestHalfBlock256(t *testing.T) {
	img := image.NewUniform(color.RGBA{255, 0, 0, 255})
	got := HalfBlock256(img, 1, 1)
	if !strings.Contains(got, "\x1b[38;5;196;48;5;196m▀") {
		t.Errorf("HalfBlock256 missing indexed color sequence: %q", got)
	}
}

func TestEncodeImageProtocols(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for _, tt := range []struct {
		tier  Tier
		start string
	}{
		{TierKitty, "\x1b_Ga=T,"},
		{TierSixel, "\x1bP"},
		{TierITerm2, "\x1b]1337;File=inline=1:"},
	} {
		t.Run(tt.start, func(t *testing.T) {
			var output bytes.Buffer
			err := EncodeImage(&output, tt.tier, frame)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(output.String(), tt.start) {
				t.Errorf("image encoding = %q, want prefix %q", output.String()[:min(output.Len(), 40)], tt.start)
			}
		})
	}
}
