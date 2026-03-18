package http

import (
	"crypto/rand"
	"math/big"
	"testing"
)

func TestCryptoRandSeedGeneration(t *testing.T) {
	t.Parallel()

	// Verify crypto/rand generates valid seeds in expected range
	maxSeed := big.NewInt(1<<31 - 1)

	seeds := make(map[int]bool)
	for i := 0; i < 100; i++ {
		seed, err := rand.Int(rand.Reader, maxSeed)
		if err != nil {
			t.Fatalf("crypto/rand.Int failed: %v", err)
		}

		val := int(seed.Int64())
		if val < 0 || val >= 1<<31-1 {
			t.Errorf("seed %d out of expected range [0, 2^31-1)", val)
		}

		seeds[val] = true
	}

	// With 100 random values in range [0, 2^31), we should have
	// nearly all unique values (collision probability negligible)
	if len(seeds) < 95 {
		t.Errorf("expected mostly unique seeds, got only %d unique out of 100", len(seeds))
	}
}
