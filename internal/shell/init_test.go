package shell

import (
	"strings"
	"testing"
)

func TestSnippetIncludesHooksAliasesAndSudoWrapper(t *testing.T) {
	tests := []struct {
		name  string
		hook  string
		guard string
		stop  string
	}{
		{name: "bash", hook: "command_not_found_handle", guard: "command -v adpm", stop: "result == 127"},
		{name: "zsh", hook: "command_not_found_handler", guard: "command -v adpm", stop: "result == 127"},
		{name: "fish", hook: "fish_command_not_found", guard: "type -q adpm", stop: "test $result -eq 127"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Snippet(tt.name)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tt.hook, tt.guard, "adpm --typo", "adpm --ad-only", "command sudo"} {
				if !strings.Contains(got, want) {
					t.Errorf("Snippet(%q) does not contain %q", tt.name, want)
				}
			}
			if !strings.Contains(got, tt.stop) {
				t.Errorf("Snippet(%q) does not stop sudo after a typo is handled", tt.name)
			}
			for _, manager := range []string{"pacman", "yay", "paru", "apt", "dnf", "brew", "flatpak"} {
				if !strings.Contains(got, "adpm "+manager) {
					t.Errorf("Snippet(%q) does not contain alias for %s", tt.name, manager)
				}
			}
		})
	}
}

func TestSnippetRejectsUnsupportedShell(t *testing.T) {
	if _, err := Snippet("powershell"); err == nil {
		t.Fatal("Snippet(powershell) returned no error")
	}
}
