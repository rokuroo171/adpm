// Command adpm wraps package managers and plays a skippable terminal ad
// before install-ish invocations. Any adpm failure falls through to the real
// command: the joke must never break a real install.
package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/rokuroo171/adpm/internal/pm"
	"github.com/rokuroo171/adpm/internal/tui"
	"golang.org/x/term"
)

const usage = `usage: adpm <pm> <args...>

Wrap a package manager and play an ad before install-ish invocations.`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	name, rest := args[0], args[1:]

	if !isTTY() || os.Getenv("ADPM_DISABLE") == "1" {
		return execReal(name, rest)
	}
	if pm.IsInstallish(name, rest) {
		playAd()
	}
	return execReal(name, rest)
}

// execReal replaces the process with the real manager so exit codes and
// signals pass through untouched.
func execReal(name string, args []string) int {
	self, _ := os.Executable()
	path, err := pm.Find(name, self)
	if err != nil {
		fmt.Fprintf(os.Stderr, "adpm: %s: command not found\n", name)
		return 127
	}
	argv := append([]string{name}, args...)
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "adpm: %s: %v\n", name, err)
		return 127
	}
	return 0 // unreachable
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// playAd runs the ad player. Best effort: any failure (including a panic)
// drops the ad and the install proceeds. The error banner lands in Milestone 6.
func playAd() {
	defer func() { _ = recover() }()

	frames := devFrames()
	if len(frames) == 0 {
		return
	}
	// Ad metadata comes from ad.toml in Milestone 6; until then the dev ad
	// uses the spec's placeholder title.
	_, _ = tui.Play(tui.Options{Frames: frames, Title: "Synergy™"})
}

// devFrames loads frames from ads-dev/ when ADPM_DEV=1. The real ad pool
// (ads/, go:embed) lands in Milestone 6.
func devFrames() []image.Image {
	if os.Getenv("ADPM_DEV") != "1" {
		return nil
	}
	fsys := os.DirFS("ads-dev")
	entries, err := fs.ReadDir(fsys, "test/frames")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".jpg") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var frames []image.Image
	for _, n := range names {
		f, err := fsys.Open(filepath.Join("test", "frames", n))
		if err != nil {
			return nil
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return nil
		}
		frames = append(frames, img)
	}
	return frames
}
