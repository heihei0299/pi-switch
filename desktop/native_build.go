package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

func nativeBuildIdentityText() string {
	version, commit, dirty := "unknown", "unknown", "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			version = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
			case "vcs.modified":
				dirty = setting.Value
			}
		}
	}
	return fmt.Sprintf("Native binary · version %s · target %s/%s · commit %s · dirty %s", version, runtime.GOOS, runtime.GOARCH, commit, dirty)
}
