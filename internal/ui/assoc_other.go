//go:build !windows

package ui

import "errors"

// Linux packages register the MIME types; macOS and mobile have no per-user association API we use.
const fileAssocSupported = false

func associateFiles() error { return errors.New("not supported") }
