package typo

var known = []string{
	"pacman", "yay", "paru", "apt", "dnf", "brew", "flatpak",
	"nix-env", "winget", "choco",
}

func Match(input string) (string, bool) {
	if input == "" {
		return "", false
	}
	best, bestDistance := "", 3
	for _, candidate := range known {
		if abs(len([]rune(input))-len([]rune(candidate))) > 2 {
			continue
		}
		if d := distance(input, candidate); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	return best, bestDistance <= 2
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
