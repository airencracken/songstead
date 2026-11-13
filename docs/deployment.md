# Hosting Songstead

This guide is for operators running Songstead as a native service. For release
downloads, see [installation](releases.md). For Docker, see
[containers](containers.md).

## Service account and data

Create a `songstead` user and group, install the binary at
`/usr/local/bin/songstead`, and create `/var/lib/songstead` owned by that user
with mode 0700. Then create accounts:

```sh
songstead create-owner --data-dir /var/lib/songstead --username alex
songstead create-user --data-dir /var/lib/songstead --username freya
songstead list-users --data-dir /var/lib/songstead
```

Each command asks for the password twice with echo off. For scripts, pass
`--password-stdin` or `--password-file`; never put a password in arguments.
Account and backup commands find the installed service's data directory unless
`--data-dir` or `SONGSTEAD_DATA_DIR` says otherwise, and run as the service
user when started as root. `set-password` takes the same options and signs out
the account everywhere.

## Service managers

- systemd: install `contrib/systemd/songstead.service` and its environment file
  at `/etc/songstead/songstead.env` (mode 0600). Gentoo packages use
  `/usr/bin/songstead` in place of `/usr/local/bin/songstead`.
- OpenRC: install `contrib/openrc/songstead` and `songstead.confd` as
  `/etc/conf.d/songstead` (mode 0600). The logrotate example uses copytruncate,
  so rotation does not restart Songstead.

Optional confinement is described in [Bubblewrap service isolation](sandbox.md).

## Reverse proxy

The service listens on `127.0.0.1:8083`. Put a proxy such as the Caddy example
in `contrib/` in front, set `SONGSTEAD_BASE_URL` to the public address, and set
`SONGSTEAD_SECURE_COOKIES` to `true` for HTTPS. List only proxies you run in
`SONGSTEAD_TRUSTED_PROXIES`; by default no forwarded address is trusted.
`SONGSTEAD_WITMOOT_URL` is optional and makes no request at startup.

Use your own hostname. With these reserved example domains:

```sh
SONGSTEAD_ADDR="127.0.0.1:8083"
SONGSTEAD_DATA_DIR="/var/lib/songstead"
SONGSTEAD_BASE_URL="https://songstead.example.com"
SONGSTEAD_WITMOOT_URL="https://boards.example.com"
SONGSTEAD_SECURE_COOKIES="true"
SONGSTEAD_TRUSTED_PROXIES="127.0.0.1/32,::1/128"
```

Quote every value, including numbers and booleans, and leave out `export`.
OpenRC reads `/etc/conf.d/songstead`; systemd reads
`/etc/songstead/songstead.env`. Restart after editing. Older unquoted values
still work. The [shared configuration convention](https://github.com/airencracken/comfylib/blob/master/docs/configuration.md)
covers quoting and escaping. Keep machine-specific notes outside this
repository.

After the first start, sign in as an owner and open **Admin** to set the public
address and optional Witmoot address. Saved values override the service
defaults. See [administration](administration.md).

## Upgrades

Stop the service and back up the data directory before upgrading. Startup
applies migrations in one transaction and refuses a database from a newer
binary; an older binary cannot open a migrated database. Start the updated
server once before running account or backup commands. Two servers cannot share
one data directory.

`restore` accepts snapshots from schema 3 onward; the server migrates them on
its next start. Restore a schema 1 or 2 snapshot with its original binary
first. See the [changelog](../CHANGELOG.md) for what each release migrates, and
[automatic backups](automatic-backups.md) for scheduled copies.

## Gentoo

The Comfyware overlay provides versioned packages built from published,
checksummed sources, and a live package that follows `master`. See
[installation](releases.md#gentoo).
