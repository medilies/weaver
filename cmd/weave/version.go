package main

import "runtime/debug"

// version is set at release builds with -ldflags "-X main.version=…".
var version string

// buildVersion is weave's version: the one stamped at release builds, else
// the module version Go recorded (`go install …@v1.2.3`, or since Go 1.24
// a pseudo-version for `go build` in a git checkout), else "dev", as
// without version control information (-buildvcs=false).
func buildVersion() string {
	info, _ := debug.ReadBuildInfo()
	return resolveVersion(version, info)
}

func resolveVersion(stamped string, info *debug.BuildInfo) string {
	if stamped != "" {
		return stamped
	}
	if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
