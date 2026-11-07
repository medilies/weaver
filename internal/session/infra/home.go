package infra

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome turns a leading "~" into the user's home directory, so
// configs can say "~/dev/weaver" and stay portable across machines. The
// forms and `weave freeze --root=~/…`, which the shell leaves as typed,
// read it the same way.
func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}

// contractHome turns the user's home directory at the start of path back
// into "~", the inverse of ExpandHome.
func contractHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+"/"); ok {
		return "~/" + rest
	}
	return path
}
