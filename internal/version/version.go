package version

import (
	"fmt"
	"runtime"
)

var (
	// These variables are set at build time via -ldflags
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
	GoVersion = runtime.Version()
)

// Info contains version information
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// Get returns version information
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
	}
}

// GetVersionString returns a formatted version string
func GetVersionString() string {
	shortCommit := GitCommit
	if GitCommit != "unknown" && len(GitCommit) > 7 {
		shortCommit = GitCommit[:7] // Shorten commit hash
	}

	if Version == "dev" {
		return fmt.Sprintf("v%s-%s", Version, shortCommit)
	}

	// Remove the 'v' prefix if it already exists to avoid double 'v'
	version := Version
	if len(version) > 0 && version[0] == 'v' {
		return version
	}

	return fmt.Sprintf("v%s", version)
}

// GetFullVersionString returns detailed version information
func GetFullVersionString() string {
	return fmt.Sprintf("OpenStack Reporter v%s (commit %s, built %s, %s)",
		Version, GitCommit, BuildTime, GoVersion)
}
