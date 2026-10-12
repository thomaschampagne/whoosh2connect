// Package version holds build-time version metadata.
package version

// Version is the application version. It can be overridden at build time with
// -ldflags "-X whoosh2connect/internal/version.Version=...".
var Version = "dev"

// Commit is the git commit the binary was built from, set at build time.
var Commit = "none"
