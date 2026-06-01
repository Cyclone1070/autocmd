package cmd

import buildinfo "runtime/debug"

func init() {
	if info, ok := buildinfo.ReadBuildInfo(); ok && info.Main.Version != "" {
		rootCmd.Version = info.Main.Version
	}
}
