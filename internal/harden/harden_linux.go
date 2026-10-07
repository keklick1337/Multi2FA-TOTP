//go:build linux

package harden

import "golang.org/x/sys/unix"

func platform() {
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{})
	// Not dumpable: no core files and other processes of the same user cannot ptrace us
	// or read /proc/<pid>/mem.
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
	raiseMemlock()
}

func raiseMemlock() {
	var r unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_MEMLOCK, &r) == nil && r.Cur < r.Max {
		r.Cur = r.Max
		_ = unix.Setrlimit(unix.RLIMIT_MEMLOCK, &r)
	}
}
