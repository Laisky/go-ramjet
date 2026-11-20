package postgres

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/require"
)

// restoreFixtureCommand builds a command inside the explicitly owned disposable
// PostgreSQL fixture, using its local database credential without exposing it.
func restoreFixtureCommand(ctx context.Context, container string, args ...string) *exec.Cmd {
	prefix := []string{"exec", "-i", container, "sh", "-c", `PGPASSWORD="$POSTGRES_PASSWORD" exec "$@"`, "sh"}
	return exec.CommandContext(ctx, "docker", append(prefix, args...)...)
}

// TestBackupPostgresRestore verifies real pg_dump, authenticated decryption and
// isolated SQL restore for both secret sources and upload modes. It only runs
// against a caller-owned, labeled, disposable fixture; ordinary tests skip it.
func TestBackupPostgresRestore(t *testing.T) {
	container := os.Getenv("RAMJET_BACKUP_RESTORE_CONTAINER")
	if container == "" {
		t.Skip("requires an explicitly owned disposable PostgreSQL fixture")
	}
	owner := os.Getenv("RAMJET_BACKUP_RESTORE_OWNER")
	require.NotEmpty(t, owner)
	ctx := t.Context()
	for key, expected := range map[string]string{"codex.owner": owner, "codex.task": "ramjet-backup-restore-test"} {
		label, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "`+key+`"}}`, container).Output()
		require.NoError(t, err)
		require.Equal(t, expected, strings.TrimSpace(string(label)), "fixture ownership must match")
	}
	psql := func(check *testing.T, database, sql string) []byte {
		check.Helper()
		cmd := restoreFixtureCommand(ctx, container, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "ramjet_fixture", "-d", database)
		cmd.Stdin = strings.NewReader(sql)
		out, err := cmd.Output()
		require.NoError(check, err, "owned fixture SQL command failed")
		return out
	}
	psql(t, "ramjet_source", "CREATE TABLE backup_restore_probe (id integer PRIMARY KEY); INSERT INTO backup_restore_probe VALUES (1), (2);")
	original := pgDumpCommand
	t.Cleanup(func() { pgDumpCommand = original })
	pgDumpCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return restoreFixtureCommand(ctx, container, append([]string{"pg_dump"}, args...)...)
	}
	for _, mode := range []string{"stream", "temp file"} {
		for _, source := range []string{"inline", "file"} {
			t.Run(mode+"/"+source, func(t *testing.T) {
				cli := &fakeUploadClient{keep: true, readLimit: -1}
				stubStreamPipeline(t, cli, dumpAndGzipToWriter)
				pass := "synthetic-restore-archive-only"
				db := cfgDB{Host: "127.0.0.1", Port: 5432, User: "ramjet_fixture", Database: "ramjet_source"}
				if source == "inline" {
					db.BackupEncryptionPassword = pass
				} else {
					db.BackupEncryptionPasswordFile = filepath.Join(t.TempDir(), "synthetic-passphrase")
					require.NoError(t, os.WriteFile(db.BackupEncryptionPasswordFile, []byte(pass+"\n"), 0o600))
				}
				key, name := buildKeyAndFilename(db, "20261120")
				if mode == "stream" {
					require.NoError(t, streamBackupToS3(ctx, db, cfgS3{}, key))
				} else {
					require.NoError(t, backupViaTempFile(ctx, db, cfgS3{}, name, key, t.TempDir()))
				}
				ciphertext := cli.received.Bytes()
				require.True(t, bytes.HasPrefix(ciphertext, []byte("age-encryption.org/v1\n")))
				identity, err := age.NewScryptIdentity(pass)
				require.NoError(t, err)
				decrypted, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
				require.NoError(t, err)
				// Authenticate the complete archive before any SQL restore writes.
				compressed, err := io.ReadAll(decrypted)
				require.NoError(t, err)
				sql := decodeSyntheticSQL(t, bytes.NewReader(compressed))
				psql(t, "postgres", "DROP DATABASE IF EXISTS ramjet_restore;")
				psql(t, "postgres", "CREATE DATABASE ramjet_restore;")
				psql(t, "ramjet_restore", "BEGIN;\n"+sql+"\nCOMMIT;")
				require.Equal(t, "2", strings.TrimSpace(string(psql(t, "ramjet_restore", "SELECT count(*) FROM backup_restore_probe;"))))
			})
		}
	}
}
