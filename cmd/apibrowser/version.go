package main

import "runtime/debug"

// version is set at build time with
//
//	-ldflags "-X main.version=v1.2.3"
//
// or, when installed with `go install …@vX.Y.Z`, taken from the module
// build info. A plain `go build` from a checkout reports "(devel)".
var version = ""

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}
