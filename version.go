// Package multi2fa exposes the release version, read from the VERSION file at compile time.
package multi2fa

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version returns the release version, e.g. "1.0.0".
func Version() string { return strings.TrimSpace(version) }
