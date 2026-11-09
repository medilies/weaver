package infra_test

import (
	"os"
	"os/exec"
	"testing"
)

// requireTool skips the test when name isn't installed, except in CI,
// where every test must run.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s is required in CI", name)
		}
		t.Skipf("%s is not installed", name)
	}
}
