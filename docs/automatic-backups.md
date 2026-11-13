# Automatic backups

The running service makes a verified, compressed backup every 24 hours. You do
not need cron or a timer. The first attempt starts one minute after startup.
Later attempts follow the last successful backup, across restarts. A failed
attempt retries after 15 minutes.

Backups go in `backups/` inside the data directory. Each snapshot directory holds
`manifest.json` and `snapshot.tar.zst` when `zstd` is on the service's PATH, or
`snapshot.tar.gz` otherwise. If zstd is installed but fails, the backup fails;
Songstead does not switch to gzip.

## Limits

Set these in `/etc/conf.d/songstead` (OpenRC) or `/etc/songstead/songstead.env`
(systemd), then restart:

```sh
SONGSTEAD_BACKUP_INTERVAL="24h"
SONGSTEAD_BACKUP_KEEP="7"
SONGSTEAD_BACKUP_MAX_BYTES="10737418240"
SONGSTEAD_BACKUP_COPY_MAX_BYTES="5368709120"
```

- Interval: 1 hour to 365 days. `0` turns automatic backups off.
- Copies kept: 1 to 30, default 7.
- Total compressed archives: default 10 GiB.
- One uncompressed snapshot: default 5 GiB.

Rotation keeps the newest snapshot and removes older ones to stay within both
limits. A snapshot too large for the archive limit is rejected and nothing is
deleted. Manual backups and pre-migration snapshots are never rotated. A failed
backup leaves older copies alone. Leave free space for one uncompressed snapshot
and its archive while a backup runs.

Owners see the status, destination, limits, last success, last error and next
attempt in **Admin → Instance settings**. Errors also go to the service log.
Backups do not include service configuration; keep that separately.

## Recovery

Copy completed snapshots to another machine. Local copies do not survive a lost
disk, and they contain password hashes and private data.

To test a restore, compare the archive's `sha256sum` with `manifest.json`, then
extract into a new private directory:

```sh
umask 077
mkdir /private/restore-drill
tar -xf /private/backups/SNAPSHOT/snapshot.tar.zst -C /private/restore-drill
songstead restore --input /private/restore-drill/songstead.db --data-dir /private/songstead-restored
```

Use `snapshot.tar.gz` for gzip copies. Never extract over a running data
directory. The restore target must be empty.
