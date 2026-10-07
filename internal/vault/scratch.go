package vault

// scratch holds the decrypted or to-be-encrypted payload. Its size grows with the vault, so it
// is not taken from memguard (which aborts when RLIMIT_MEMLOCK is exceeded); instead it is
// mlocked when possible and always wiped on release.
type scratch struct{ b []byte }

func newScratch(n int) *scratch {
	s := &scratch{b: make([]byte, n)}
	lockMemory(s.b)
	return s
}

func (s *scratch) destroy() {
	wipe(s.b)
	unlockMemory(s.b)
	s.b = nil
}
