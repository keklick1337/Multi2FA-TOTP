//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !windows

package vault

func lockMemory([]byte)   {}
func unlockMemory([]byte) {}
