package main

import (
	"runtime/debug"
	"testing"
)

func TestReleaseBuildsReportTheStampedVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}
	if got := resolveVersion("0.2.0", info); got != "0.2.0" {
		t.Fatalf("got %q, want the stamped 0.2.0", got)
	}
}

func TestGoInstallBuildsReportTheModuleVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}
	if got := resolveVersion("", info); got != "v0.1.0" {
		t.Fatalf("got %q, want the module's v0.1.0", got)
	}
}

func TestBuildsWithoutAVersionReportDev(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	if got := resolveVersion("", info); got != "dev" {
		t.Fatalf("got %q, want dev", got)
	}
}
