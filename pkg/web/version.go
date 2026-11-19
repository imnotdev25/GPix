package web

import (
	"runtime/debug"
	"sync"
)

// Version is the release this binary was built from, set at build time:
//
//	go build -ldflags "-X gpix/pkg/web.Version=v1.2.3"
//
// The Dockerfile and the release workflow set it. Without it, AppVersion
// falls back to the git commit Go records in local builds.
var Version = ""

var appVersion = sync.OnceValue(func() string {
	if Version != "" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if len(rev) > 7 {
			rev = rev[:7]
		}
		if rev != "" {
			if dirty {
				rev += "-dirty"
			}
			return "dev-" + rev
		}
	}
	return "dev"
})

// AppVersion is the version shown in the web app's sidebar and the startup log.
func AppVersion() string { return appVersion() }
