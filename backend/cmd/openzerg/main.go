// Package main is the entrypoint for the openzerg control-plane CLI.
package main

import (
	"fmt"
	"os"
)

// version is the build-time version string. It is overridden at link time via
// -ldflags "-X main.version=..." in CI; the default below is for local dev.
const version = "0.1.0-dev"

func main() {
	// Minimal M0 surface: print the version and exit. Subcommands (run,
	// doctor, version) are wired in M1+. We accept "version" as a no-op
	// convenience so the M0 verify line `./bin/openzerg version` works.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("openzerg %s\n", version)
		os.Exit(0)
	}
	fmt.Printf("openzerg %s\n", version)
}
