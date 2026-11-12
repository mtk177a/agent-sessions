package buildinfo

import "runtime/debug"

// Version is an optional override set by the linker.
var Version string

func ExecutableVersion() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
