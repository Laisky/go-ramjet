package postgres

import (
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/stretchr/testify/require"
)

// TestBuildKeyPrefixAndBase verifies the managed S3 prefix used for uploads and retention cleanup.
func TestBuildKeyPrefixAndBase(t *testing.T) {
	t.Run("directory-like prefix", func(t *testing.T) {
		keyPrefix, base := buildKeyPrefixAndBase(cfgDB{
			Database:         "appdb",
			BackupFilePrefix: "backup/postgres/prod/appdb/",
		})

		require.Equal(t, "backup/postgres/prod/appdb/appdb-", keyPrefix)
		require.Equal(t, "appdb", base)
	})

	t.Run("file-like prefix", func(t *testing.T) {
		keyPrefix, base := buildKeyPrefixAndBase(cfgDB{
			Database:         "appdb",
			BackupFilePrefix: "backup/postgres/prod/appdb/custom",
		})

		require.Equal(t, "backup/postgres/prod/appdb/custom-", keyPrefix)
		require.Equal(t, "custom", base)
	})
}

// TestSelectExpiredBackupKeys verifies that retention keeps only the newest managed backups.
func TestSelectExpiredBackupKeys(t *testing.T) {
	keys := []string{
		"backup/postgres/prod/appdb/appdb-20260101.gz",
		"backup/postgres/prod/appdb/appdb-20260102.gz",
		"backup/postgres/prod/appdb/appdb-20260103.gz",
		"backup/postgres/prod/appdb/appdb-20260104.gz",
		"backup/postgres/prod/appdb/appdb-latest.gz",
		"backup/postgres/prod/appdb/other-20260101.gz",
	}

	expiredKeys := selectExpiredBackupKeys("backup/postgres/prod/appdb/appdb-", keys, 2)
	require.Equal(t, []string{
		"backup/postgres/prod/appdb/appdb-20260102.gz",
		"backup/postgres/prod/appdb/appdb-20260101.gz",
	}, expiredKeys)
}

// TestLoadCfgBackupRetention verifies configured and default retention values.
func TestLoadCfgBackupRetention(t *testing.T) {
	gconfig.Shared.Set("tasks.postgres.s3.keep_last", 0)
	require.Equal(t, defaultBackupHistoryLimit, loadCfg().S3.KeepLast)

	gconfig.Shared.Set("tasks.postgres.s3.keep_last", 7)
	require.Equal(t, 7, loadCfg().S3.KeepLast)
}
