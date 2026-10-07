// Package harden applies process level protections against memory inspection.
package harden

import "github.com/awnumar/memguard"

// Apply should run first in main. It disables core dumps, blocks debuggers where the OS allows
// it and makes memguard wipe its secrets on interrupt.
func Apply() {
	platform()
	memguard.CatchInterrupt()
}

// Purge wipes all memguard managed secrets; call it on exit.
func Purge() { memguard.Purge() }
