package pm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsInstallish(t *testing.T) {
	tests := []struct {
		pm   string
		args []string
		want bool
	}{
		// pacman / yay / paru
		{"pacman", []string{"-S", "vim"}, true},
		{"pacman", []string{"-Syu"}, true},
		{"pacman", []string{"-S"}, true},
		{"pacman", []string{"-U", "./pkg.tar.zst"}, true},
		{"pacman", []string{"-Ss", "vim"}, false},
		{"pacman", []string{"-Si", "vim"}, false},
		{"pacman", []string{"-Sl"}, false},
		{"pacman", []string{"-Sg", "gnome"}, false},
		{"pacman", []string{"-Sp", "vim"}, false},
		{"pacman", []string{"-Q"}, false},
		{"pacman", []string{"-Ql", "vim"}, false},
		{"pacman", []string{"-R", "vim"}, false},
		{"pacman", []string{"-Rs", "vim"}, false},
		{"pacman", []string{"-h"}, false},
		{"pacman", nil, false},
		{"pacman", []string{"--sync", "vim"}, false},
		{"pacman", []string{"--", "-S"}, false},
		{"yay", []string{"-S", "vim"}, true},
		{"yay", []string{"-Ss", "vim"}, false},
		{"paru", []string{"-U", "pkg"}, true},
		{"paru", []string{"-Q"}, false},

		// apt / dnf / brew
		{"apt", []string{"install", "curl"}, true},
		{"apt", []string{"reinstall", "curl"}, true},
		{"apt", []string{"-y", "install", "curl"}, true},
		{"apt", []string{"update"}, false},
		{"apt", []string{"search", "curl"}, false},
		{"apt", nil, false},
		{"dnf", []string{"install", "htop"}, true},
		{"dnf", []string{"reinstall", "htop"}, true},
		{"dnf", []string{"remove", "htop"}, false},
		{"dnf", []string{"upgrade"}, false},
		{"brew", []string{"install", "jq"}, true},
		{"brew", []string{"reinstall", "jq"}, true},
		{"brew", []string{"list"}, false},
		{"brew", []string{"uninstall", "jq"}, false},

		// flatpak
		{"flatpak", []string{"install", "org.gnome.Calculator"}, true},
		{"flatpak", []string{"update"}, false},
		{"flatpak", []string{"list"}, false},

		// unknown managers never trigger
		{"pamcan", []string{"install", "x"}, false},
		{"pamcan", []string{"-S", "x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.pm+" "+fmtArgs(tt.args), func(t *testing.T) {
			if got := IsInstallish(tt.pm, tt.args); got != tt.want {
				t.Errorf("IsInstallish(%q, %v) = %v, want %v", tt.pm, tt.args, got, tt.want)
			}
		})
	}
}

func fmtArgs(args []string) string {
	if len(args) == 0 {
		return "(none)"
	}
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}

func writeExec(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindSkipsOwnPath(t *testing.T) {
	dir := t.TempDir()
	self := writeExec(t, dir, "adpm", "#!/bin/sh\nexit 0\n")
	// A symlink named pacman pointing back at adpm: exec.LookPath resolves to
	// it, and Find must skip it rather than recurse.
	link := filepath.Join(dir, "pacman")
	if err := os.Symlink(self, link); err != nil {
		t.Fatal(err)
	}

	// Only adpm's directory on PATH: nothing left to find.
	t.Setenv("PATH", dir)
	if got, err := Find("pacman", self); err == nil {
		t.Errorf("Find returned %q, want error (recursion guard)", got)
	}

	// A real pacman later on PATH wins over the symlink.
	other := t.TempDir()
	realPacman := writeExec(t, other, "pacman", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+other)
	got, err := Find("pacman", self)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != realPacman {
		t.Errorf("Find = %q, want %q", got, realPacman)
	}
}

func TestFindResolvesNormally(t *testing.T) {
	dir := t.TempDir()
	fake := writeExec(t, dir, "pacman", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	got, err := Find("pacman", "/some/other/adpm")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != fake {
		t.Errorf("Find = %q, want %q", got, fake)
	}

	if _, err := Find("definitely-not-a-real-pm", ""); err == nil {
		t.Error("Find for missing binary: want error")
	}
}
