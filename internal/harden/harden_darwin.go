//go:build darwin && !ios

package harden

import "golang.org/x/sys/unix"

func platform() {
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{})
	// Refuse debugger attachment (lldb, dtrace) for the lifetime of the process.
	_ = unix.PtraceDenyAttach()
	var r unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_MEMLOCK, &r) == nil && r.Cur < r.Max {
		r.Cur = r.Max
		_ = unix.Setrlimit(unix.RLIMIT_MEMLOCK, &r)
	}
}
