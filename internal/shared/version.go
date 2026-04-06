package shared

import (
	"runtime/debug"
	"strings"
)

var Version string // set via -ldflags

func normalize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "refs/tags/")
	v = strings.TrimPrefix(v, "v")
	return v
}

func AppVersion() string {
	if v := normalize(Version); v != "" && v != "(devel)" {
		return v
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if v := normalize(info.Main.Version); v != "" && v != "(devel)" {
			return v
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.tag" {
				if v := normalize(setting.Value); v != "" && v != "(devel)" {
					return v
				}
			}
		}
	}

	return "dev"
}
