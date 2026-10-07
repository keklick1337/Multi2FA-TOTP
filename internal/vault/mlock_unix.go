//go:build linux || darwin || freebsd || openbsd || netbsd

package vault

import "golang.org/x/sys/unix"

func lockMemory(b []byte) {
	if len(b) > 0 {
		_ = unix.Mlock(b)
	}
}

func unlockMemory(b []byte) {
	if len(b) > 0 {
		_ = unix.Munlock(b)
	}
}
