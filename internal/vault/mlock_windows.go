//go:build windows

package vault

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func lockMemory(b []byte) {
	if len(b) > 0 {
		_ = windows.VirtualLock(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	}
}

func unlockMemory(b []byte) {
	if len(b) > 0 {
		_ = windows.VirtualUnlock(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	}
}
