// Package buildinfo holds build metadata injected via ldflags.
package buildinfo

import (
	"fmt"
	"runtime"
)

// Set via -ldflags "-X .../buildinfo.Version=..." at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// UserAgent returns the HTTP User-Agent value of the CLI.
func UserAgent() string {
	return fmt.Sprintf("papierkram-cli/%s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH)
}
