package version

// BuildVersion is the build-time version, set with ldflags.
var BuildVersion = "dev"

// Get returns the current build version.
func Get() string {
	return BuildVersion
}
