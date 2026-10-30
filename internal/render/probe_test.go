package render

import "testing"

func TestParseReplies(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantKitty bool
		wantDA1   string
	}{
		{"kitty and DA1", "\x1b_Gi=31;OK\x1b\\\x1b[?62;4c", true, "\x1b[?62;4c"},
		{"kitty rejection with DA1", "\x1b_Gi=31;ERR\x1b\\\x1b[?62;1c", false, "\x1b[?62;1c"},
		{"unrelated response", "\x1b[0n", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kitty, da1 := ParseReplies(tt.response)
			if kitty != tt.wantKitty || da1 != tt.wantDA1 {
				t.Errorf("ParseReplies(%q) = (%v, %q), want (%v, %q)",
					tt.response, kitty, da1, tt.wantKitty, tt.wantDA1)
			}
		})
	}
}
