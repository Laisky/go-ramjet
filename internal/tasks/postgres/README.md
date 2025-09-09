# PostgreSQL backup and restore

This task performs automated PostgreSQL backups and uploads them to S3-compatible storage. Dumps are plain SQL compressed with gzip.

## What it does

- Runs `pg_dump` against one or more databases
- Compresses the stream with gzip
- Uploads the object to S3 (MinIO-compatible)
- Skips uploading if today’s object already exists (idempotent per day)

Default per-DB timeout: 30 minutes.

---

## Prerequisites

- `pg_dump` available in PATH (from the PostgreSQL client tools)
- Network connectivity to your PostgreSQL server(s)
- S3/MinIO endpoint reachable with access key/secret and write permissions to the bucket

---

## Configuration

YAML keys under `tasks.postgres` in your main settings file:

```yaml
tasks:
	postgres:
		enable: true            # enable/disable this task
		interval: 86400         # run interval in seconds (default 1 day)
		dbs:                    # list of databases to back up
			- host: "127.0.0.1"
				port: 5432
				user: "postgres"
				password: "secret"
				database: "appdb"
				# Naming semantics explained below
				backup_file_prefix: "backups/postgres/prod/appdb/"
		s3:
			enable: true
			endpoint: "s3.example.com"
			bucket: "private"
			access_key: "readwrite"
			access_secret: "xxx"

		# Optional
		use_temp_file: false    # if true, write dump->gzip to a temp file, then upload
		temp_dir: "/var/tmp"   # where temp files live when use_temp_file=true (defaults to system temp)
```

## Naming and object keys

The `backup_file_prefix` controls the generated S3 key and the local filename used for logs/temp files.

- If `backup_file_prefix` ends with `/` (directory-like):
	- S3 key: `<prefix><database>-YYYYMMDD.gz`
	- Local filename: `<database>-YYYYMMDD.gz`
	- Example: `backup_file_prefix: "backup/postgre/prod/oneapi/"` produces
		- S3: `backup/postgre/prod/oneapi/oneapi-20250909.gz`
		- Local filename: `oneapi-20250909.gz`

- If `backup_file_prefix` does NOT end with `/` (file-like):
	- Interpreted as `<optional_dir>/<basename>`
	- S3 key: `<optional_dir>/<basename>-YYYYMMDD.gz`
	- Local filename: `<basename>-YYYYMMDD.gz`
	- Example: `backup_file_prefix: "backup/postgre/prod/oneapi/oneapi"` produces
		- S3: `backup/postgre/prod/oneapi/oneapi-20250909.gz`
		- Local filename: `oneapi-20250909.gz`

Notes:
- Leading `/` is removed, and path is normalized with forward slashes.
- If no usable prefix/basename is present, the database name is used; otherwise falls back to `pg-backup`.

---

## Running the task

One-off run (using your own settings file):

```bash
go run main.go -c /path/to/settings.yml -t postgres
```

As a service, the task will self-schedule based on `interval` and run daily by default. First run happens immediately when enabled.

## Modes

- Streaming (default):
	- Pipeline: `pg_dump | gzip | S3` (no local files; object size unknown until upload completes)
- Temp file mode (`use_temp_file: true`):
	- Pipeline: `pg_dump | gzip > temp file`, then upload file to S3
	- Useful when you need the size beforehand or want to inspect the artifact

---

## Restore

Backups are gzip-compressed plain SQL dumps. Restore via `psql`.

### 1) Download from S3

Use your preferred S3 client. Examples:

Using AWS CLI:

```bash
aws s3 cp s3://private/backup/postgre/prod/oneapi/oneapi-20250909.gz ./
```

Using MinIO Client (mc):

```bash
mc cp myminio/private/backup/postgre/prod/oneapi/oneapi-20250909.gz ./
```

### 2) Prepare database

Create the target database if it does not exist, and ensure required roles exist (pg_dump of a single DB does not include global objects like roles/tablespaces):

```bash
psql -h 127.0.0.1 -U postgres -c 'CREATE DATABASE oneapi;' || true
```

If restoring into an existing DB, consider dropping/recreating or restoring into a fresh DB to avoid conflicts:

```bash
psql -h 127.0.0.1 -U postgres -c 'DROP DATABASE IF EXISTS oneapi_restore;'
psql -h 127.0.0.1 -U postgres -c 'CREATE DATABASE oneapi_restore;'
```

### 3) Restore data

Since the dump is plain SQL compressed with gzip, use `gunzip -c` (or `zcat`) piped to `psql`:

```bash
gunzip -c oneapi-20250909.gz | psql -h 127.0.0.1 -U postgres -d oneapi_restore
```

Alternatively:

```bash
zcat oneapi-20250909.gz | psql -h 127.0.0.1 -U postgres -d oneapi
```

### 4) Verify

- Check schema and counts of critical tables
- Run application smoke tests against the restored database

---

## How it works (under the hood)

- The task builds the `pg_dump` command with `-h`, `-p`, `-U`, `-d` from your config
- Password is passed via `PGPASSWORD` env var for non-interactive auth
- Output is compressed via `gzip` and then uploaded to S3
- If an object with the computed key already exists for today, the run is skipped for that DB

---

## Troubleshooting

- `pg_dump: command not found`
	- Install PostgreSQL client tools and ensure `pg_dump` is in PATH

- Authentication failures to PostgreSQL
	- Verify host/port/user/password; ensure network and pg_hba.conf allow the connection

- S3 upload errors (403/NoSuchBucket)
	- Check credentials, bucket existence, and permissions; verify endpoint URL and TLS settings

- Backup skipped unexpectedly ("already exists")
	- The task is idempotent per day. Delete or move the object if you want to re-run for the same date

- Large databases timing out
	- Increase available network bandwidth, or adjust the per-DB timeout in code if needed (default 30m)

---

## Notes and limitations

- Dumps are per-database and plain SQL; global objects (roles, tablespaces) are not included
- No retention policy is enforced by this task; manage retention on the bucket side
- No built-in encryption of artifacts; rely on S3 server-side encryption, bucket policies, or network security
- Object naming is date-based (YYYYMMDD) and computed from `backup_file_prefix` and the DB name (see rules above)

---

## Examples

Given:

```yaml
backup_file_prefix: "backup/postgre/prod/oneapi/"
database: "oneapi"
```

Generates:

- S3 key: `backup/postgre/prod/oneapi/oneapi-YYYYMMDD.gz`
- Local filename: `oneapi-YYYYMMDD.gz`

Given:

```yaml
backup_file_prefix: "backup/postgre/prod/oneapi/oneapi"
database: "oneapi"
```

Generates:

- S3 key: `backup/postgre/prod/oneapi/oneapi-YYYYMMDD.gz`
- Local filename: `oneapi-YYYYMMDD.gz`
