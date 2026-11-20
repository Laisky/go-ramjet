package password_test

import (
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/password"
	"github.com/Laisky/go-ramjet/library/log"
)

// TestGeneratePasswdByDate verifies the password rotates with the month and
// the secret, and stays constant within a month.
func TestGeneratePasswdByDate(t *testing.T) {
	now, err := time.Parse("20060102 15", "20060101 12")
	require.NoError(t, err)
	secret := "fjfjkejflewjfjewfiwefijweifj"
	passwd1 := password.GeneratePasswdByDate(now, secret)                      // origin
	passwd2 := password.GeneratePasswdByDate(now, secret+"1")                  // secret change
	passwd3 := password.GeneratePasswdByDate(now.Add(24*40*time.Hour), secret) // new month
	passwd4 := password.GeneratePasswdByDate(now.Add(-24*time.Hour), secret)   // previous month
	passwd5 := password.GeneratePasswdByDate(now.Add(40*time.Hour), secret)    // same month

	require.NotEqual(t, passwd1, passwd2, "secret change must rotate the password")
	require.NotEqual(t, passwd1, passwd3, "new month must rotate the password")
	require.NotEqual(t, passwd1, passwd4, "previous month must differ")
	require.Equal(t, passwd1, passwd5, "same month must keep the password")
}

// TestGeneratePasswdByDateDerivationIsFrozen pins the exact derivation,
// hex(SHA1(YYYYMM || secret))[:15], with vectors computed independently
// (Python hashlib).
//
// CodeQL go/weak-sensitive-data-hashing #67 flags the SHA-1 call, but this is
// not password storage: the server-side rotation secret (operator config, never
// request input) is expanded into the Elasticsearch password itself, which is
// pushed to the configured password APIs; nothing here stores or verifies a
// hash. Recovering the secret from an output means guessing the secret, and
// SHA-1's collision weakness does not apply to this keyed derivation. Swapping the algorithm would silently change the password
// every consumer derives for the current month, so the format must not change.
func TestGeneratePasswdByDateDerivationIsFrozen(t *testing.T) {
	const secret = "synthetic-es-rotation-secret"

	nov := time.Date(2026, time.November, 19, 6, 0, 0, 0, time.UTC)
	require.Equal(t, "8cd77763dfa13f4", password.GeneratePasswdByDate(nov, secret))

	dec := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, "3b128d6917b80b0", password.GeneratePasswdByDate(dec, secret))
}

func init() {
	if err := log.Logger.ChangeLevel("debug"); err != nil {
		log.Logger.Panic("change log level", zap.Error(err))
	}
}
