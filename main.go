// Command adpm wraps package managers and plays a skippable terminal ad
// before install-ish invocations. Any adpm failure falls through to the real
// command: the joke must never break a real install.
package main

import (
	"bufio"
	"embed"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/rokuroo171/adpm/internal/pm"
	"github.com/rokuroo171/adpm/internal/shell"
	"github.com/rokuroo171/adpm/internal/tui"
	"github.com/rokuroo171/adpm/internal/typo"
	"golang.org/x/term"
)

//go:embed messages.toml
var messageFS embed.FS

const usage = `usage:
  adpm <pm> <args...>
  adpm --typo <name> [args...]
  adpm --ad-only <pm> <args...>
  adpm init bash|zsh|fish

Wrap package managers and play an ad before install-ish invocations.`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "--typo":
		return runTypo(args[1:])
	case "--ad-only":
		return runAdOnly(args[1:])
	}
	name, rest := args[0], args[1:]

	if !isTTY() || os.Getenv("ADPM_DISABLE") == "1" {
		return execReal(name, rest)
	}
	if pm.IsInstallish(name, rest) {
		result := playAd()
		if result.Cancelled {
			return cancelInstall(name)
		}
	}
	return execReal(name, rest)
}

func runInit(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	snippet, err := shell.Snippet(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Fprint(os.Stdout, snippet)
	return 0
}

func runAdOnly(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	name, rest := args[0], args[1:]
	if os.Getenv("ADPM_DISABLE") == "1" || !isTTY() || !pm.IsInstallish(name, rest) {
		return 0
	}
	if playAd().Cancelled {
		return cancelInstall(name)
	}
	return 0
}

func runTypo(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	command := args[0]
	matched := false
	if os.Getenv("ADPM_MERCY") != "1" && os.Getenv("ADPM_DISABLE") != "1" && isTTY() {
		_, matched = typo.Match(command)
	}
	if matched {
		playTypoAd()
		fmt.Fprintln(os.Stderr, commandNotFoundText(currentShell(), command, true))
		return 127
	}
	fmt.Fprintln(os.Stderr, commandNotFoundText(currentShell(), command, false))
	return 127
}

func commandNotFoundText(shellName, command string, sponsored bool) string {
	if sponsored {
		return fmt.Sprintf("%s: %s: command not found. (Sponsored by adpm.)", shellName, command)
	}
	switch shellName {
	case "zsh":
		return fmt.Sprintf("zsh: command not found: %s", command)
	case "fish":
		return fmt.Sprintf("fish: Unknown command: %s", command)
	default:
		return fmt.Sprintf("bash: %s: command not found", command)
	}
}

func currentShell() string {
	name := os.Getenv("ADPM_SHELL")
	if name == "" {
		name = filepath.Base(os.Getenv("SHELL"))
	}
	switch name {
	case "bash", "zsh", "fish":
		return name
	default:
		return "bash"
	}
}

func playTypoAd() {
	frames := devFrames()
	if len(frames) == 0 {
		return
	}
	_, _ = tui.Play(tui.Options{Frames: frames, Title: "Synergy™", Typo: true})
}

func cancelInstall(name string) int {
	messages, err := loadMessages()
	message := "error: operation aborted (user lost interest)"
	if err == nil && messages["abort"][name] != "" {
		message = messages["abort"][name]
	}
	fmt.Fprintln(os.Stderr, message)
	return 130
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

// playAd runs the ad player. Any failure, including a panic, drops the ad and
// lets the install proceed.
func playAd() (result tui.Result) {
	defer func() { _ = recover() }()

	frames := devFrames()
	if len(frames) == 0 {
		return tui.Result{}
	}
	// The development ad uses a placeholder title until ad metadata is loaded.
	messages, err := loadMessages()
	if err != nil {
		return tui.Result{}
	}
	rewardLines := messageLines(messages["reward"])
	result, _ = tui.Play(tui.Options{
		Frames: frames, Title: "Synergy™",
		RewardLines: rewardLines, ReportMessage: messages["report"]["message"],
	})
	return result
}

func loadMessages() (map[string]map[string]string, error) {
	f, err := messageFS.Open("messages.toml")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseMessages(f)
}

func parseMessages(r io.Reader) (map[string]map[string]string, error) {
	messages := make(map[string]map[string]string)
	section := ""
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			messages[section] = make(map[string]string)
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section == "" {
			return nil, fmt.Errorf("invalid messages.toml line: %q", line)
		}
		value, err := strconv.Unquote(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid messages.toml value: %w", err)
		}
		messages[section][strings.TrimSpace(key)] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func messageLines(section map[string]string) []string {
	keys := make([]string, 0, len(section))
	for key := range section {
		if strings.HasPrefix(key, "line") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, section[key])
	}
	return lines
}

// devFrames loads frames from ads-dev/ only when ADPM_DEV=1.
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
