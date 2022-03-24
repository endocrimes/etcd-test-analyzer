package version

import (
	"bytes"
	"fmt"
)

var (
	// GitCommit is the commit that was compiled. This will be filled in by the compiler.
	GitCommit string

	// GitDescribe is populated by the compiler (and make).
	GitDescribe string

	// Version is the current version number
	Version = "0.0.1"

	// VersionPrerelease is a pre-release marker for the version. If this is ""
	// (empty string) then it means that it is a final release. Otherwise, this
	// is a pre-release such as "dev" (in development), "beta", "rc1", etc.
	VersionPrerelease = "dev"

	// VersionMetadata is metadata further describing the build type.
	VersionMetadata = ""
)

// Info is...
type Info struct {
	Revision          string
	Version           string
	VersionPrerelease string
	VersionMetadata   string
}

// GetVersion compiles the global version info into a usable struct.
func GetVersion() *Info {
	ver := Version
	rel := VersionPrerelease
	md := VersionMetadata
	if GitDescribe != "" {
		ver = GitDescribe
	}
	if GitDescribe == "" && rel == "" && VersionPrerelease != "" {
		rel = "dev"
	}

	return &Info{
		Revision:          GitCommit,
		Version:           ver,
		VersionPrerelease: rel,
		VersionMetadata:   md,
	}
}

// FullVersionNumber renders the version info into a human readable string.
func (c *Info) FullVersionNumber(rev bool) string {
	var versionString bytes.Buffer

	fmt.Fprintf(&versionString, "v%s", c.Version)
	if c.VersionPrerelease != "" {
		fmt.Fprintf(&versionString, "-%s", c.VersionPrerelease)
	}

	if c.VersionMetadata != "" {
		fmt.Fprintf(&versionString, "+%s", c.VersionMetadata)
	}

	if rev && c.Revision != "" {
		fmt.Fprintf(&versionString, " (%s)", c.Revision)
	}

	return versionString.String()
}
