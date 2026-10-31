package render

import "strings"

func ParseReplies(response string) (kitty bool, da1 string) {
	kitty = strings.Contains(response, "\x1b_Gi=31;OK\x1b\\")
	start := strings.Index(response, "\x1b[?")
	if start < 0 {
		return kitty, ""
	}
	end := strings.IndexByte(response[start:], 'c')
	if end < 0 {
		return kitty, ""
	}
	return kitty, response[start : start+end+1]
}
