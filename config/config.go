package config

import (
	"fmt"
	"runtime"
)

var (
	RuntimeVer = fmt.Sprintf(runtime.Version())
	Version    = ""
	CommitSHA  = ""
	BuildTime  = ""
	BuildArch  = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	Repo       = ""
)
