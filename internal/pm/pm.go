// Package pm decides whether a wrapped package-manager invocation is
// install-ish and resolves the real manager binary.
package pm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// wrapped lists the managers adpm wraps, per the spec's fixed decisions.
var wrapped = map[string]struct{}{
	"pacman": {}, "yay": {}, "paru": {},
	"apt": {}, "dnf": {}, "brew": {}, "flatpak": {},
}

// IsInstallish reports whether invoking the named manager with args will
// install or remove packages. Anything else (queries, upgrades, help) is not
// install-ish and must exec immediately with no ad.
func IsInstallish(name string, args []string) bool {
	if _, ok := wrapped[name]; !ok {
		return false
	}
	switch name {
	case "pacman", "yay", "paru":
		return pacmanInstallish(args)
	case "apt", "dnf", "brew":
		return subcommandIn(args, "install", "reinstall")
	case "flatpak":
		return subcommandIn(args, "install")
	}
	return false
}

// pacmanInstallish implements the -S/-U rule: the operation letter must be S
// or U and must not carry a query modifier (s, i, l, g, p). -Q* and -R* never
// trigger.
func pacmanInstallish(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if strings.HasPrefix(a, "--") {
			continue // long options never carry the operation
		}
		if len(a) < 2 || a[0] != '-' {
			continue // positional (the package) comes after the operation
		}
		switch a[1] {
		case 'S', 'U':
			return !strings.ContainsAny(a[1:], "silgp")
		default:
			return false // -Q, -R, -F, -h, -V, ...
		}
	}
	return false
}

// subcommandIn reports whether the first positional argument is one of ops.
// Flags (-y, --refresh, ...) are skipped wherever they appear.
func subcommandIn(args []string, ops ...string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			continue
		}
		for _, op := range ops {
			if a == op {
				return true
			}
		}
		return false
	}
	return false
}

// Find resolves the real binary for name, skipping adpm's own executable so a
// symlink or PATH entry pointing back at adpm cannot recurse.
func Find(name, self string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if self == "" || !sameFile(path, self) {
		return path, nil
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		cand := filepath.Join(dir, name)
		if sameFile(cand, self) {
			continue
		}
		st, err := os.Stat(cand)
		if err != nil || st.IsDir() || st.Mode().Perm()&0o111 == 0 {
			continue
		}
		return cand, nil
	}
	return "", exec.ErrNotFound
}

// sameFile compares two paths after resolving symlinks, so a symlink to adpm
// counts as adpm's own path.
func sameFile(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = a
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = b
	}
	if filepath.Clean(ra) == filepath.Clean(rb) {
		return true
	}
	sa, err := os.Stat(ra)
	if err != nil {
		return false
	}
	sb, err := os.Stat(rb)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}
