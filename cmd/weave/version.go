package main

import "runtime/debug"

// version is set at release builds with -ldflags "-X main.version=…".
var version string

// buildVersion is weave's version: the one stamped at release builds, else
// the module version `go install …@v1.2.3` recorded, else "dev".
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
