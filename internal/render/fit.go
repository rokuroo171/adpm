// Package render turns video frames into terminal output: the pure fit
// function, the half-block renderer, and the layout mode thresholds.
package render

import (
	"fmt"
	"math"
)

// Max ad box dimensions, per spec: centered with letterbox beyond these.
const (
	maxBoxCols = 100
	maxBoxRows = 36
)

// Mode is the layout tier for a given terminal size.
type Mode int

const (
	// ModeFull shows the frame image centered in the alt screen.
	ModeFull Mode = iota
	// ModeCompact is a text-only ad box for terminals under ~40x12.
	ModeCompact
	// ModeOneLine is a single plain line plus countdown, no TUI, for
	// terminals under ~20x6.
	ModeOneLine
)

// ModeFor picks the layout tier from the terminal size.
func ModeFor(cols, rows int) Mode {
	if cols < 20 || rows < 6 {
		return ModeOneLine
	}
	if cols < 40 || rows < 12 {
		return ModeCompact
	}
	return ModeFull
}

// Fit computes the ad box size in cells for a frame of frameW x frameH
// pixels inside a cols x rows terminal whose cells are cellW x cellH pixels.
// Both dimensions are fitted preserving the frame's aspect ratio, the box is
// capped at maxBoxCols x maxBoxRows, and every result is clamped to at least 1.
func Fit(frameW, frameH, cols, rows, cellW, cellH int) (int, int) {
	// Terminals that report no pixel size fall back to a 1:2 cell aspect.
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = 1, 2
	}
	if frameW < 1 {
		frameW = 1
	}
	if frameH < 1 {
		frameH = 1
	}
	maxCols := min(cols, maxBoxCols)
	maxRows := min(rows, maxBoxRows)
	if maxCols < 1 {
		maxCols = 1
	}
	if maxRows < 1 {
		maxRows = 1
	}

	scale := math.Min(
		float64(maxCols*cellW)/float64(frameW),
		float64(maxRows*cellH)/float64(frameH),
	)
	boxCols := int(math.Ceil(float64(frameW) * scale / float64(cellW)))
	boxRows := int(math.Ceil(float64(frameH) * scale / float64(cellH)))
	return clamp(boxCols, 1, maxCols), clamp(boxRows, 1, maxRows)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Truncate cuts s to at most w display cells, appending an ellipsis when it
// had to cut.
func Truncate(s string, w int) string {
	if w < 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

// CompactLine is the text-only ad box line for small terminals.
func CompactLine(title string, remaining int) string {
	return fmt.Sprintf("AD · %s · %ds", title, remaining)
}
