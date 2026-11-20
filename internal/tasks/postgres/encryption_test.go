package postgres

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/stretchr/testify/require"
)

const syntheticSQL = "SELECT 'fixture-only-private-content';\n"

// syntheticDump writes a small gzipped SQL fixture without using PostgreSQL.
func syntheticDump(_ context.Context, _ cfgDB, w io.Writer) error {
	gz := gzip.NewWriter(w)
	if _, err := io.WriteString(gz, syntheticSQL); err != nil {
		return errors.Wrap(err, "write synthetic SQL")
	}
	return errors.Wrap(gz.Close(), "finish synthetic SQL gzip")
}

// decodeSyntheticSQL checks gzip readability and returns the recovered fixture.
func decodeSyntheticSQL(t *testing.T, raw io.Reader) string {
	t.Helper()
	gz, err := gzip.NewReader(raw)
	require.NoError(t, err)
	data, err := io.ReadAll(gz)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return string(data)
}

// TestBackupEncryptedUploadRoundTrip verifies both upload modes store an age
// envelope, each with its own password, never a raw dump or gzip object.
func TestBackupEncryptedUploadRoundTrip(t *testing.T) {
	for _, mode := range []string{"stream", "temp file"} {
		t.Run(mode, func(t *testing.T) {
			cli := &fakeUploadClient{keep: true, readLimit: -1}
			stubStreamPipeline(t, cli, syntheticDump)
			tmp := t.TempDir()
			db := cfgDB{Database: "fileio"}
			pass := "age-fixture-stream-password"
			if mode == "stream" {
				db.BackupEncryptionPassword = pass
			} else {
				pass = "different-age-fixture-file-password"
				secretFile := filepath.Join(t.TempDir(), "fixture-secret")
				require.NoError(t, os.WriteFile(secretFile, []byte(pass+"\r\n"), 0o600))
				db.BackupEncryptionPasswordFile = secretFile
			}
			key, name := buildKeyAndFilename(db, "20261120")
			require.Equal(t, "fileio-20261120.gz.age", key)
			require.Equal(t, key, name)

			if mode == "stream" {
				require.NoError(t, streamBackupToS3(t.Context(), db, cfgS3{}, key))
			} else {
				require.NoError(t, backupViaTempFile(t.Context(), db, cfgS3{}, name, key, tmp))
				remaining, err := os.ReadDir(tmp)
				require.NoError(t, err)
				require.Empty(t, remaining)
			}

			got := cli.received.Bytes()
			require.True(t, bytes.HasPrefix(got, []byte("age-encryption.org/v1\n")))
			require.NotContains(t, string(got), syntheticSQL)
			require.Empty(t, cli.opts.ContentEncoding)
			require.Equal(t, "application/octet-stream", cli.opts.ContentType)

			identity, err := age.NewScryptIdentity(pass)
			require.NoError(t, err)
			raw, err := age.Decrypt(bytes.NewReader(got), identity)
			require.NoError(t, err)
			require.Equal(t, syntheticSQL, decodeSyntheticSQL(t, raw))

			if mode == "stream" {
				wrong, err := age.NewScryptIdentity("wrong-age-fixture-password")
				require.NoError(t, err)
				_, err = age.Decrypt(bytes.NewReader(got), wrong)
				require.Error(t, err)
			}
		})
	}
}

// TestBadEncryptionSourceDoesNotUploadOrBlockOtherDatabases checks that a
// missing secret fails closed in both upload modes without skipping a later DB.
func TestBadEncryptionSourceDoesNotUploadOrBlockOtherDatabases(t *testing.T) {
	for _, mode := range []string{"stream", "temp file"} {
		t.Run(mode, func(t *testing.T) {
			cli := &fakeUploadClient{keep: true, readLimit: -1}
			var dumped []string
			stubStreamPipeline(t, cli, func(ctx context.Context, db cfgDB, w io.Writer) error {
				dumped = append(dumped, db.Database)
				return syntheticDump(ctx, db, w)
			})
			tmp := t.TempDir()
			missingSecret := filepath.Join(tmp, "secret-path-must-not-leak")
			cfg := &cfgBackup{DBs: []cfgDB{
				{Database: "fileio", BackupEncryptionPasswordFile: missingSecret},
				{Database: "legacy", Password: "db-connection-only"},
			}}
			var attempted []string
			backupDatabases(cfg, "20261120", func(ctx context.Context, db cfgDB, _ *cfgBackup, key, name string) error {
				attempted = append(attempted, key)
				var err error
				if mode == "stream" {
					err = streamBackupToS3(ctx, db, cfgS3{}, key)
				} else {
					err = backupViaTempFile(ctx, db, cfgS3{}, name, key, tmp)
				}
				if db.Database == "fileio" {
					require.ErrorContains(t, err, "load backup encryption password")
					require.NotContains(t, err.Error(), missingSecret)
					require.Zero(t, cli.consumed, "a missing encryption password must not emit any bytes")
				} else {
					require.NoError(t, err)
				}
				return errors.Wrap(err, "run test backup")
			})
			require.Equal(t, []string{"fileio-20261120.gz.age", "legacy-20261120.gz"}, attempted)
			require.Equal(t, []string{"legacy"}, dumped)
			require.Equal(t, "application/gzip", cli.opts.ContentType)
			require.Equal(t, "gzip", cli.opts.ContentEncoding)
			require.Equal(t, syntheticSQL, decodeSyntheticSQL(t, bytes.NewReader(cli.received.Bytes())))
		})
	}
}

// loadBackupTestYAML loads a synthetic settings file through the production
// YAML loader and resolves its PostgreSQL backup configuration.
func loadBackupTestYAML(t *testing.T, yaml string) *cfgBackup {
	t.Helper()
	name := filepath.Join(t.TempDir(), "settings.yml")
	require.NoError(t, os.WriteFile(name, []byte(yaml), 0o600))
	config := gconfig.New()
	require.NoError(t, config.LoadFromFile(name))
	return loadCfg(config)
}

// TestBackupLoadIndependentPasswords uses the real config reader to check the
// deployed YAML shape, exact passwords, isolation and fail-closed validation.
func TestBackupLoadIndependentPasswords(t *testing.T) {
	cfg := loadBackupTestYAML(t, `
tasks:
 postgres:
  enable: true
  dbs:
   - host: db-a
     port: 5432
     user: alice
     database: oneapi
     password: db-a-password
   - database: wrong-type
     backup_encryption_password: 1234
   - database: wrong-empty
     backup_encryption_password: ""
   - database: wrong-file
     backup_encryption_password_file: null
   - database: conflicting-options
     backup_encryption_password: literal
     backup_encryption_password_file: /fixture/conflicting
   - host: db-b
     port: 5450
     user: bob
     database: mcp
     password: db-b-password
     backup_encryption_password: ' fixture-b-an-independent-password '
   - database: separate
     backup_encryption_password_file: /fixture/separate
`)
	require.Equal(t, []cfgDB{
		{Host: "db-a", Port: 5432, User: "alice", Database: "oneapi", Password: "db-a-password"},
		{Host: "db-b", Port: 5450, User: "bob", Database: "mcp", Password: "db-b-password", BackupEncryptionPassword: " fixture-b-an-independent-password "},
		{Database: "separate", BackupEncryptionPasswordFile: "/fixture/separate"},
	}, cfg.DBs)
}

// TestBackupLegacySingleEntry preserves the established one-entry list and
// verifies that absent or malformed lists do not activate a flat configuration.
func TestBackupLegacySingleEntry(t *testing.T) {
	for _, tc := range []struct {
		name string
		list string
		want []cfgDB
	}{
		{name: "legacy one-item list", list: "  dbs: [{host: db, port: 5432, user: legacy, password: connection-only, database: existing}]\n", want: []cfgDB{{Host: "db", Port: 5432, User: "legacy", Password: "connection-only", Database: "existing"}}},
		{name: "absent list"},
		{name: "explicit empty list", list: "  dbs: []\n"},
		{name: "explicit null list", list: "  dbs: null\n"},
		{name: "invalid mapping", list: "  dbs: {database: incorrect}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := loadBackupTestYAML(t, `
tasks:
 postgres:
  host: db
  port: 5432
  user: legacy
  password: connection-only
  database: existing
`+tc.list)
			if len(tc.want) == 0 {
				require.Empty(t, cfg.DBs)
			} else {
				require.Equal(t, tc.want, cfg.DBs)
			}
		})
	}
}

// TestBackupMixedFormatRetention ensures neither object format deletes the
// other format and encrypted archives enforce the existing retention count.
func TestBackupMixedFormatRetention(t *testing.T) {
	keys := []string{
		"sample-20261118.gz.age", "sample-20261119.gz.age", "sample-20261120.gz.age",
		"sample-20261118.gz", "sample-20261119.gz", "sample-20261120.gz",
		"sample-bogus.gz.age", "sample-20261101.gz.age.tmp", "unmanaged-20261101.gz.age",
	}
	require.Equal(t, []string{"sample-20261118.gz.age"}, selectExpiredBackupKeys("sample-", encryptedBackupSuffix, keys, 2))
	require.Equal(t, []string{"sample-20261118.gz"}, selectExpiredBackupKeys("sample-", gzipBackupSuffix, keys, 2))
}

// TestBackupDuplicateSeriesProtectsOtherDatabases ensures colliding series are
// not silently shared, even when their encryption formats differ.
func TestBackupDuplicateSeriesProtectsOtherDatabases(t *testing.T) {
	cfg := &cfgBackup{DBs: []cfgDB{
		{Database: "alpha", BackupFilePrefix: "shared"},
		{Database: "beta", BackupFilePrefix: "shared", BackupEncryptionPassword: "another-fixture-secret"},
		{Database: "independent"},
	}}
	var keys []string
	backupDatabases(cfg, "20261120", func(_ context.Context, _ cfgDB, _ *cfgBackup, key, _ string) error {
		keys = append(keys, key)
		return nil
	})
	require.Equal(t, []string{"independent-20261120.gz"}, keys)
}

// TestEncryptedOnlyDatabaseList rejects missing encryption and preserves both
// the ordinary list and the encrypted-only namespace through YAML decoding.
func TestEncryptedOnlyDatabaseList(t *testing.T) {
	cfg := loadBackupTestYAML(t, `
tasks:
 postgres:
  dbs:
   - database: oneapi
  encrypted_dbs:
   - database: missing
   - database: invalid
     backup_encryption_password: " "
   - database: mcp
     backup_encryption_password_file: /fixture/mcp-age
   - database: third
     backup_encryption_password: independent-fixture-passphrase
`)
	require.Equal(t, []cfgDB{
		{Database: "oneapi"},
		{Database: "mcp", BackupEncryptionPasswordFile: "/fixture/mcp-age"},
		{Database: "third", BackupEncryptionPassword: "independent-fixture-passphrase"},
	}, cfg.DBs)
}

// TestMalformedBackupDatabaseListDoesNotBlockOtherList checks that each
// namespace can run when the other is configured incorrectly.
func TestMalformedBackupDatabaseListDoesNotBlockOtherList(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		want   cfgDB
	}{
		{name: "invalid encrypted list", config: "  dbs: [{database: oneapi}]\n  encrypted_dbs: bad\n", want: cfgDB{Database: "oneapi"}},
		{name: "invalid ordinary list", config: "  dbs: bad\n  encrypted_dbs: [{database: mcp, backup_encryption_password_file: /fixture/mcp-age}]\n", want: cfgDB{Database: "mcp", BackupEncryptionPasswordFile: "/fixture/mcp-age"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := loadBackupTestYAML(t, "tasks:\n postgres:\n"+tc.config)
			require.Equal(t, []cfgDB{tc.want}, cfg.DBs)
		})
	}
}
