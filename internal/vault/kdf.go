package vault

import (
	"crypto/sha512"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

// KDFKind selects the password hashing function.
type KDFKind uint8

const (
	KDFArgon2id KDFKind = 1
	KDFScrypt   KDFKind = 2
	KDFPBKDF2   KDFKind = 3 // PBKDF2-HMAC-SHA512
)

// KDFs lists the supported functions, strongest first.
var KDFs = []KDFKind{KDFArgon2id, KDFScrypt, KDFPBKDF2}

func (k KDFKind) String() string {
	switch k {
	case KDFArgon2id:
		return "Argon2id"
	case KDFScrypt:
		return "scrypt"
	case KDFPBKDF2:
		return "PBKDF2-SHA512"
	}
	return "unknown"
}

// KDFParams describes one password hashing setup. Unused fields are zero.
type KDFParams struct {
	Kind       KDFKind
	Iterations uint32 // Argon2id passes or PBKDF2 rounds
	MemoryKiB  uint32 // Argon2id memory
	Threads    uint8  // Argon2id lanes or scrypt parallelism p
	LogN       uint8  // scrypt cost: N = 2^LogN
	BlockSize  uint32 // scrypt r
}

const (
	minIterations = 3
	maxMemoryKiB  = 4 * 1024 * 1024
)

// pack maps the parameters onto the three header fields.
func (p KDFParams) pack() (uint32, uint32, uint8) {
	switch p.Kind {
	case KDFScrypt:
		return uint32(p.LogN), p.BlockSize, p.Threads
	case KDFPBKDF2:
		return p.Iterations, 0, 0
	}
	return p.Iterations, p.MemoryKiB, p.Threads
}

func unpackKDF(kind KDFKind, p1, p2 uint32, p3 uint8) KDFParams {
	switch kind {
	case KDFScrypt:
		return KDFParams{Kind: kind, LogN: uint8(min(p1, 255)), BlockSize: p2, Threads: p3}
	case KDFPBKDF2:
		return KDFParams{Kind: kind, Iterations: p1}
	}
	return KDFParams{Kind: kind, Iterations: p1, MemoryKiB: p2, Threads: p3}
}

func (p KDFParams) validate() error {
	bad := func(what string) error {
		return fmt.Errorf("%w: unreasonable %s parameters (%s)", ErrNotVault, p.Kind, what)
	}
	switch p.Kind {
	case KDFArgon2id:
		if p.Iterations < 1 || p.Iterations > 100000 {
			return bad("iterations")
		}
		if p.MemoryKiB < 8*1024 || p.MemoryKiB > maxMemoryKiB {
			return bad("memory")
		}
		if p.Threads < 1 {
			return bad("threads")
		}
	case KDFScrypt:
		if p.LogN < 10 || p.LogN > 24 || p.BlockSize < 1 || p.BlockSize > 64 || p.Threads < 1 || p.Threads > 16 {
			return bad("N, r or p")
		}
		if p.MemoryBytes() > uint64(maxMemoryKiB)*1024 {
			return bad("memory")
		}
	case KDFPBKDF2:
		if p.Iterations < 10000 || p.Iterations > 1_000_000_000 {
			return bad("iterations")
		}
	default:
		return fmt.Errorf("%w: unknown key derivation %d", ErrUnsupported, p.Kind)
	}
	return nil
}

// MemoryBytes is the RAM one derivation needs.
func (p KDFParams) MemoryBytes() uint64 {
	switch p.Kind {
	case KDFArgon2id:
		return uint64(p.MemoryKiB) * 1024
	case KDFScrypt:
		return 128 * uint64(p.BlockSize) * (uint64(1) << p.LogN) * uint64(max(p.Threads, 1))
	}
	return 0
}

func (p KDFParams) MemoryMiB() uint64 { return p.MemoryBytes() / (1024 * 1024) }

// derive hashes secret with salt into a 32 byte key. The result is owned by the caller.
func (p KDFParams) derive(secret, salt []byte) []byte {
	switch p.Kind {
	case KDFScrypt:
		k, err := scrypt.Key(secret, salt, 1<<p.LogN, int(p.BlockSize), int(p.Threads), keySize)
		if err != nil {
			panic(err)
		}
		return k
	case KDFPBKDF2:
		return pbkdf2.Key(secret, salt, int(p.Iterations), keySize, sha512.New)
	}
	return argon2.IDKey(secret, salt, p.Iterations, p.MemoryKiB, p.Threads, keySize)
}

// Benchmark measures one derivation with these parameters.
func (p KDFParams) Benchmark() time.Duration {
	start := time.Now()
	k := p.derive([]byte("benchmark"), make([]byte, saltSize))
	wipe(k)
	return time.Since(start)
}

// Level is a protection preset: how long one unlock should take and how much memory Argon2id uses.
// Like KeePassXC, the work factor is calibrated on the current machine.
type Level struct {
	Name      string
	Target    time.Duration
	MemoryKiB uint32
}

var Levels = []Level{
	{"fast", 500 * time.Millisecond, 128 * 1024},
	{"balanced", time.Second, 256 * 1024},
	{"strong", 2 * time.Second, 512 * 1024},
	{"paranoid", 5 * time.Second, 1024 * 1024},
}

// DefaultLevel is the preset used for new vaults.
const DefaultLevel = 1

// DefaultKDF is used when no calibrated parameters are given (tests, fallbacks).
var DefaultKDF = KDFParams{Kind: KDFArgon2id, Iterations: 4, MemoryKiB: 256 * 1024, Threads: 4}

// DefaultSecurity is the setup of new vaults unless the user chooses otherwise.
var DefaultSecurity = Security{KDF: DefaultKDF, Cipher: CipherXChaCha20}

func threads() uint8 {
	return uint8(min(max(runtime.NumCPU(), 1), 8))
}

// Calibrate picks the work factor of kind so that one derivation takes about level.Target.
func Calibrate(kind KDFKind, level Level) KDFParams {
	target := float64(level.Target)
	switch kind {
	case KDFScrypt:
		p := KDFParams{Kind: kind, LogN: 14, BlockSize: 8, Threads: 1}
		one := float64(max(p.Benchmark(), time.Millisecond))
		for p.LogN < 22 && one*2 <= target {
			p.LogN++
			one *= 2
		}
		return p
	case KDFPBKDF2:
		p := KDFParams{Kind: kind, Iterations: 100000}
		one := float64(max(p.Benchmark(), time.Millisecond))
		p.Iterations = uint32(min(max(100000*target/one, 100000), 1_000_000_000))
		return p
	}
	p := KDFParams{Kind: KDFArgon2id, Iterations: 1, MemoryKiB: level.MemoryKiB, Threads: threads()}
	one := float64(max(p.Benchmark(), time.Millisecond))
	p.Iterations = uint32(min(max(target/one, minIterations), 100000))
	return p
}

// ValidateKDF reports whether the parameters are accepted for a vault.
func ValidateKDF(p KDFParams) error { return p.validate() }
