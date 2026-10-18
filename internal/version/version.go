// Package version holds the release version, written by build.py, and the build ID.
package version

const Version = "0.1.0"

// Build is the UTC build time as yyyyMMddHHmmss, set by build.py with
// -ldflags "-X github.com/SiddheshNan/moonshed/internal/version.Build=...".
var Build = "dev"

// Full is the version with its build ID as semver build metadata, e.g. 0.1.0+20261019143005.
func Full() string { return Version + "+" + Build }
