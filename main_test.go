package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHelperAdpm is not a real test: when ADPM_TEST_HELPER=1 the test binary
// acts as adpm itself, so run() can be exercised end to end (syscall.Exec
// included) without shipping a separate fixture binary.
func TestHelperAdpm(t *testing.T) {
	if os.Getenv("ADPM_TEST_HELPER") != "1" {
		t.Skip("helper process entry point")
	}
	for i, a := range os.Args {
		if a == "--" {
			os.Exit(run(os.Args[i+1:]))
		}
	}
	os.Exit(2)
}

// buildHelper re-executes this test binary as adpm with a controlled PATH.
func buildHelper(t *testing.T, env ...string) (*exec.Cmd, func() (string, int)) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperAdpm", "--")
	cmd.Env = append(os.Environ(), "ADPM_TEST_HELPER=1")
	cmd.Env = append(cmd.Env, env...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	runFn := func() (string, int) {
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run helper: %v", err)
		}
		return out.String(), code
	}
	return cmd, runFn
}

// fakePM writes a fake manager that echoes its args, one per line, then exits
// with the given code.
func fakePM(t *testing.T, dir, name string, exitCode int) string {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\"\nexit " + itoa(exitCode) + "\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
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

// The helper's stdin/stdout are pipes, so this also proves the no-TTY path:
// exec immediately, no ad output, real exit code passes through.
func TestPassthroughExitCodeAndArgs(t *testing.T) {
	dir := t.TempDir()
	fakePM(t, dir, "pacman", 42)

	cmd, runFn := buildHelper(t, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Args = append(cmd.Args, "pacman", "-S", "vim")
	out, code := runFn()

	if code != 42 {
		t.Errorf("exit code = %d, want 42", code)
	}
	want := "-S\nvim\n"
	if out != want {
		t.Errorf("output = %q, want %q (no ad output expected on no-TTY)", out, want)
	}
}

func TestPassthroughInstallishNoOutputNoTTY(t *testing.T) {
	dir := t.TempDir()
	fakePM(t, dir, "apt", 0)

	cmd, runFn := buildHelper(t, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Args = append(cmd.Args, "apt", "install", "curl")
	out, code := runFn()

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if out != "install\ncurl\n" {
		t.Errorf("output = %q, want fake manager's args only", out)
	}
}

func TestADPMDisableSkipsAd(t *testing.T) {
	dir := t.TempDir()
	fakePM(t, dir, "pacman", 7)

	cmd, runFn := buildHelper(t,
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ADPM_DISABLE=1",
		"ADPM_DEV=1",
	)
	cmd.Args = append(cmd.Args, "pacman", "-S", "vim")
	out, code := runFn()

	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
	if out != "-S\nvim\n" {
		t.Errorf("output = %q, want fake manager's args only", out)
	}
}

func TestMissingBinaryExits127(t *testing.T) {
	cmd, runFn := buildHelper(t, "PATH="+t.TempDir())
	cmd.Args = append(cmd.Args, "pacman", "-Q")
	out, code := runFn()

	if code != 127 {
		t.Errorf("exit code = %d, want 127", code)
	}
	if !strings.Contains(out, "command not found") {
		t.Errorf("output = %q, want command not found message", out)
	}
}

func TestNoArgsUsage(t *testing.T) {
	if os.Getenv("ADPM_TEST_HELPER") == "1" {
		t.Skip("helper")
	}
	if code := run(nil); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
