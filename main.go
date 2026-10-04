// Command discogsctl is a command line client for the Discogs API.
package main

import (
	"os"

	"go.hasteful.org/discogsctl/cmd"
)

// Set at link time with -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cmd.Execute(cmd.VersionInfo{Version: version, Commit: commit, Date: date}))
}
