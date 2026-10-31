package shell

import (
	"embed"
	"fmt"
)

//go:embed bash.sh zsh.sh fish.fish
var snippets embed.FS

func Snippet(name string) (string, error) {
	file := name
	if name == "fish" {
		file = "fish.fish"
	} else if name != "bash" && name != "zsh" {
		return "", fmt.Errorf("adpm: unsupported shell %q", name)
	} else {
		file += ".sh"
	}
	data, err := snippets.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
