package postgres

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDumpCredentialProbe is a subprocess-only stand-in for pg_dump. It checks
// the actual child's argv and environment without printing credential values;
// success emits ordinary synthetic SQL, not Go test runner output.
func TestDumpCredentialProbe(t *testing.T) {
	if os.Getenv("RAMJET_DUMP_CREDENTIAL_PROBE") != "1" {
		return
	}
	var expected []string
	if json.Unmarshal([]byte(os.Getenv("RAMJET_DUMP_EXPECTED_ARGS")), &expected) != nil {
		os.Exit(41)
	}
	var actual []string
	for i, arg := range os.Args {
		if arg == "--" {
			actual = os.Args[i+1:]
			break
		}
	}
	if !reflect.DeepEqual(expected, actual) {
		os.Exit(42)
	}
	if subtle.ConstantTimeCompare([]byte(os.Getenv("PGPASSWORD")), []byte(os.Getenv("RAMJET_DUMP_EXPECTED_PASSWORD"))) != 1 {
		os.Exit(43)
	}
	if os.Getenv("RAMJET_DUMP_PARENT_SENTINEL") != "ordinary inherited value" {
		os.Exit(44)
	}
	if !strings.HasSuffix(os.Getenv("PGPASSFILE"), "/absent-pgpass") {
		os.Exit(45)
	}
	if _, err := io.WriteString(os.Stdout, "SELECT 1;\n"); err != nil {
		os.Exit(46)
	}
	os.Exit(0)
}

// TestDumpAndGzipUsesIndependentDatabaseCredentials exercises the real command
// assembly with two independent login identities, then the original one-api
// identity again. Child passwords override an ambient password only when set;
// unrelated parent environment and legacy empty-password behavior are preserved.
// It verifies argv/environment and gzip behavior, not actual database login.
func TestDumpAndGzipUsesIndependentDatabaseCredentials(t *testing.T) {
	t.Setenv("RAMJET_DUMP_CREDENTIAL_PROBE", "1")
	t.Setenv("RAMJET_DUMP_PARENT_SENTINEL", "ordinary inherited value")
	t.Setenv("PGPASSWORD", "synthetic ambient login")
	t.Setenv("PGPASSFILE", t.TempDir()+"/absent-pgpass")
	original := pgDumpCommand
	t.Cleanup(func() { pgDumpCommand = original })
	pgDumpCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		require.True(t, name == "pg_dump", "backup must invoke the configured dump program")
		childArgs := append([]string{"-test.run=^TestDumpCredentialProbe$", "--"}, args...)
		return exec.CommandContext(ctx, os.Args[0], childArgs...)
	}
	oneAPI := cfgDB{Host: "synthetic-oneapi.invalid", Port: 5432, User: "synthetic-oneapi-user", Password: "synthetic one-api login !", Database: "oneapi"}
	mcp := cfgDB{Host: "synthetic-mcp.invalid", Port: 5544, User: "synthetic-mcp-user", Password: "synthetic mcp login ?", Database: "mcp", BackupEncryptionPassword: "synthetic archive password excluded from login"}
	cases := []struct {
		name string
		db   cfgDB
	}{
		{name: "one-api", db: oneAPI}, {name: "independent MCP", db: mcp}, {name: "one-api remains unchanged", db: oneAPI},
		{name: "legacy ambient authentication", db: cfgDB{Host: "synthetic-legacy.invalid", Port: 5432, User: "synthetic-legacy-user", Database: "legacy"}},
	}
	for i := range cases {
		tc := &cases[i]
		t.Run(tc.name, func(t *testing.T) {
			expected := []string{"-h", tc.db.Host, "-p", fmt.Sprint(tc.db.Port), "-U", tc.db.User, "-d", tc.db.Database}
			encoded, err := json.Marshal(expected)
			require.NoError(t, err)
			t.Setenv("RAMJET_DUMP_EXPECTED_ARGS", string(encoded))
			password := tc.db.Password
			if password == "" {
				password = os.Getenv("PGPASSWORD")
			}
			t.Setenv("RAMJET_DUMP_EXPECTED_PASSWORD", password)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var compressed bytes.Buffer
			require.NoError(t, dumpAndGzipToWriter(ctx, tc.db, &compressed), "synthetic subprocess credential contract failed")
			reader, err := gzip.NewReader(&compressed)
			require.NoError(t, err)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			// The subprocess also runs package initialization, which may prepend
			// task-registration logs. The final SQL marker belongs to this probe.
			require.True(t, bytes.HasSuffix(body, []byte("SELECT 1;\n")), "synthetic dump marker missing")
			require.True(t, subtle.ConstantTimeCompare([]byte(os.Getenv("PGPASSWORD")), []byte("synthetic ambient login")) == 1, "child override must not mutate parent authentication")
			require.True(t, os.Getenv("RAMJET_DUMP_PARENT_SENTINEL") == "ordinary inherited value", "parent environment changed")
		})
	}
}
