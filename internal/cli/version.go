package cli

import (
	"fmt"
	"runtime"
)

// Application metadata.
const (
	AppName    = "lifelog"
	AppVersion = "0.1.0"
)

// VersionString returns the formatted application name, version, and platform information.
func VersionString() string {
	return fmt.Sprintf("%s version %s (%s/%s)", AppName, AppVersion, runtime.GOOS, runtime.GOARCH)
}
