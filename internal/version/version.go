package version

// These values are overridden at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

// Info describes the running build.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// Get returns the current build information.
func Get() Info {
	return Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
}
