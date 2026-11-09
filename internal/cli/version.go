package cli

import (
	"runtime/debug"
	"strings"
)

// version is the release version: set by the release build, or the tag for
// "go install ...@v1.2.3" builds, or "dev".
func version(buildVersion string) string {
	if buildVersion != "" {
		return buildVersion
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		return releaseVersion(bi.Main.Version)
	}
	return "dev"
}

// releaseVersion keeps real release tags and turns Go's automatic build stamps
// (pseudo-versions such as v0.0.0-20261109153144-bb496671b87e+dirty) into "dev".
func releaseVersion(v string) string {
	if v == "" || v == "(devel)" || strings.Contains(v, "+dirty") ||
		strings.HasPrefix(v, "v0.0.0-") || strings.Contains(v, "-0.") {
		return "dev"
	}
	return v
}
