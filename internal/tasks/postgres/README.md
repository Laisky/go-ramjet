# PostgreSQL backup and restore

This task performs automated PostgreSQL backups and uploads them to S3-compatible storage. Dumps contain gzip-compressed SQL and can be encrypted with a different age passphrase for each database.

## What it does

- Runs `pg_dump` against one or more databases
- Compresses the stream with gzip
- Optionally encrypts each gzip stream with age scrypt before writing it to disk or S3
- Uploads the object to S3 (MinIO-compatible)
- Skips uploading if today’s object already exists (idempotent per day)
- Starts a background retention cleanup after each successful upload

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
    # Older Ramjet releases ignore this entire list.
    encrypted_dbs:
      - host: "pg-mcp.internal"
        port: 5432
        user: "backup_mcp"
        password: "replace-db-connection-password"
        database: "mcp"
        backup_file_prefix: "backups/postgres/prod/mcp/"
        backup_encryption_password_file: "/run/secrets/postgres-mcp-backup"
    s3:
      enable: true
      endpoint: "s3.example.com"
      bucket: "private"
      access_key: "readwrite"
      access_secret: "xxx"
      keep_last: 14        # keep the latest 14 backups, including the new upload

    # Optional
    use_temp_file: false    # if true, write dump->gzip to a temp file, then upload
    temp_dir: "/var/tmp"   # where temp files live when use_temp_file=true (defaults to system temp)
```

Each `dbs` entry has its own PostgreSQL connection settings. `password` is for
PostgreSQL authentication only. Set exactly one of
`backup_encryption_password_file` (preferred: a read-only mounted secret file)
or `backup_encryption_password` (only in access-controlled configuration) to encrypt
that database's backup. Use distinct, strong random passphrases, not database or
S3 credentials, and keep recovery copies outside the backup bucket. A secret file
must be a readable regular file up to 4 KiB; Ramjet removes one trailing LF or CRLF.
Omit both encryption fields to keep plaintext gzip. An explicitly blank,
malformed, unreadable or ambiguous encryption setting rejects that database
instead of uploading plaintext. Errors do not stop independent entries.
Keep a single `dbs` list entry for an existing one-database deployment.

Put any databases that **must never be backed up unencrypted** in
`tasks.postgres.encrypted_dbs` only, not in `dbs`. The new binary requires a
valid encryption source on every entry there. Older binaries do not read this
namespace, so rollback pauses those backups instead of uploading plaintext.
The lists run on the same schedule and bucket; backup prefixes must be unique
across both. If both lists are absent or empty, no databases are backed up.

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

- Encrypted archives append `.age` to the same name (for example,
  `mcp-20261120.gz.age`), without HTTP gzip content encoding. Missing encryption
  fields preserve the existing `.gz` filename and upload metadata.
- Every database must have a distinct managed prefix, including databases on
  different hosts with the same name. All configured entries sharing the same
  generated prefix are skipped and logged, even across formats. Unrelated entries
  still run. Assign distinct prefixes before enabling backups for both databases.
- The encrypted suffix creates its own daily and retention series. Turning
  encryption on does not delete, migrate or retroactively encrypt older plaintext
  objects. Inventory and deliberately retire them after verifying an encrypted
  restore. `keep_last` applies to the format the job currently writes.
- Leading `/` is removed, and path is normalized with forward slashes.
- If no usable prefix/basename is present, the database name is used; otherwise falls back to `pg-backup`.

---

## Running the task

One-off run (using your own settings file):

```bash
go run main.go -c /path/to/settings.yml -t postgres
```

As a service, the task will self-schedule based on `interval` and run daily by default. First run happens immediately when enabled. Databases run sequentially and an
overlapping tick in the same process is skipped. Use only one active runner per
backup series across deployments; the task does not take a distributed lock.

## Modes

- Streaming (default):
  - Pipeline: `pg_dump | gzip | [age] | S3` (no local files)
- Temp file mode (`use_temp_file: true`):
  - Pipeline: `pg_dump | gzip | [age] > temp file`, then upload to S3. When encryption is enabled, the private temporary file contains only ciphertext.
  - Useful when you need the size beforehand. Allocate disk space for a complete archive and protect the directory for any unencrypted backups.

Age uses its standard scrypt work factor (about 256 MiB of temporary memory per
encrypted database), and streaming uploads use 64 MiB multipart buffers. Allow
that headroom in the container along with memory for Ramjet, compression and
`pg_dump`. The pipeline does not load the entire dump into memory.

---

## Restore

Backups contain gzip-compressed SQL. Decrypt `.gz.age` archives first and restore the resulting gzip, or restore unchanged legacy `.gz` files directly.

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

### 2) Decrypt encrypted backups

Use the standard [age CLI](https://github.com/FiloSottile/age). Verify that the
downloaded file decrypts completely and its gzip checksum passes before opening
a database connection. Use a private directory on protected storage, and remove
any decrypted output once the recovery verification is complete.

```bash
umask 077
workdir=$(mktemp -d)
age --decrypt -o "$workdir/restore.sql.gz" ./mcp-20261120.gz.age &&
  gzip -t "$workdir/restore.sql.gz"
```

Enter the passphrase for that database and backup generation at the age prompt,
not as a shell command-line argument. Continue to the restore only if **both**
commands succeeded. For a legacy `.gz` object, skip age and run `gzip -t` on the
download. Changing a backup password affects only future uploads; retain older
passphrases until their backup objects expire.

### 3) Prepare database

Create the target database if it does not exist, and ensure required roles exist (pg_dump of a single DB does not include global objects like roles/tablespaces):

```bash
psql -h 127.0.0.1 -U postgres -c 'CREATE DATABASE oneapi;' || true
```

If restoring into an existing DB, consider dropping/recreating or restoring into a fresh DB to avoid conflicts:

```bash
psql -h 127.0.0.1 -U postgres -c 'DROP DATABASE IF EXISTS oneapi_restore;'
psql -h 127.0.0.1 -U postgres -c 'CREATE DATABASE oneapi_restore;'
```

### 4) Restore data

Restore into an isolated, newly created database first. For an encrypted
backup, substitute `"$workdir/restore.sql.gz"` for the legacy filename below.
Use Bash and set `pipefail` so a decompression or SQL failure is surfaced:

```bash
set -o pipefail
gunzip -c oneapi-20250909.gz | psql -X -v ON_ERROR_STOP=1 --single-transaction -h 127.0.0.1 -U postgres -d oneapi_restore
```

### 5) Verify

- Check schema and counts of critical tables
- Run application smoke tests against the restored database

---

## How it works (under the hood)

- The task builds the `pg_dump` command with `-h`, `-p`, `-U`, `-d` from your config
- Password is passed via `PGPASSWORD` env var for non-interactive auth
- Output is compressed via `gzip`, optionally streamed through age encryption, then uploaded to S3
- If an object with the computed key already exists for today, the run is skipped for that DB
- After a successful upload, the task lists objects with the same managed key prefix and deletes the oldest ones beyond `s3.keep_last`

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
- Retention is enforced per managed backup series and keeps the latest `s3.keep_last` objects (default: 14)
- `.gz` archives have no built-in encryption. Use `.gz.age` for client-side encryption and HTTPS for transport; bucket controls apply to either mode.
- Object naming is date-based (YYYYMMDD) and computed from `backup_file_prefix` and the DB name (see rules above)

---

## Examples

Given:

```yaml
backup_file_prefix: 'backup/postgre/prod/oneapi/'
database: 'oneapi'
```

Generates:

- S3 key: `backup/postgre/prod/oneapi/oneapi-YYYYMMDD.gz`
- Local filename: `oneapi-YYYYMMDD.gz`

Given:

```yaml
backup_file_prefix: 'backup/postgre/prod/oneapi/oneapi'
database: 'oneapi'
```

Generates:

- S3 key: `backup/postgre/prod/oneapi/oneapi-YYYYMMDD.gz`
- Local filename: `oneapi-YYYYMMDD.gz`
