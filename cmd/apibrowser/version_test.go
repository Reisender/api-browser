package main

import "testing"

func TestVersionString(t *testing.T) {
	old := version
	defer func() { version = old }()
	version = "v9.9.9"
	if got := versionString(); got != "v9.9.9" {
		t.Errorf("ldflags override: got %q", got)
	}
	version = ""
	if got := versionString(); got == "" {
		t.Error("fallback must not be empty")
	}
}
