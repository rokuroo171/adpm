package typo

import "testing"

func TestMatchUsesDamerauLevenshteinDistance(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "pacman", want: "pacman", ok: true},
		{input: "pamcan", want: "pacman", ok: true},
		{input: "pakman", want: "pacman", ok: true},
		{input: "yaa", want: "yay", ok: true},
		{input: "par", want: "paru", ok: true},
		{input: "aptt", want: "apt", ok: true},
		{input: "dnff", want: "dnf", ok: true},
		{input: "breww", want: "brew", ok: true},
		{input: "flatpka", want: "flatpak", ok: true},
		{input: "nix-eenv", want: "nix-env", ok: true},
		{input: "wingte", want: "winget", ok: true},
		{input: "chcco", want: "choco", ok: true},
		{input: "python", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := Match(tt.input)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Match(%q) = (%q, %t), want (%q, %t)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDistanceAllowsUnrestrictedTranspositions(t *testing.T) {
	if got := distance("CA", "ABC"); got != 2 {
		t.Fatalf("distance(CA, ABC) = %d, want 2", got)
	}
}
