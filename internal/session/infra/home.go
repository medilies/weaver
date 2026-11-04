package infra

import (
	"os"
	"path/filepath"
	"strings"
)

// expandHome turns a leading "~" into the user's home directory, so
// configs can say "~/dev/weaver" and stay portable across machines.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
