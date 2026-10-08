# Hosting Songstead

Songstead is an independent AGPL-3.0-or-later Go binary with SQLite and embedded
HTML, HTMX, CSS and mascot assets. There is no asset server or required external
service. Go 1.26 is needed to build; static binaries have no C library requirement.

`make build` prepares an unpublished 0.1.0 binary in `bin/songstead`. Run
`bin/songstead --version` to inspect its build stamp. Published source builds use Comfylib v0.1.1; see
[the release installation guide](releases.md).

Create a service account and group named `songstead`, install the binary, and
create `/var/lib/songstead` with that owner and mode 0700. Run account provisioning
as that account, passing a password on stdin:

```sh
songstead create-user --data-dir /var/lib/songstead --username alice --password-stdin
```

The command reads one password line from stdin. Use a protected pipe or input
file; do not place a password in command arguments or shell history. Passwords
need at least 12 characters and at most 72 bytes. `set-password` uses the same
arguments and revokes the account's sessions atomically.

The systemd unit is `contrib/systemd/songstead.service`; install it under the
appropriate system unit directory and its environment file at
`/etc/songstead/songstead.env` (mode 0600). The source unit uses
`/usr/local/bin/songstead`; Gentoo adjusts this to `/usr/bin/songstead`.
OpenRC uses `contrib/openrc/songstead` and `songstead.confd`, with configuration
at `/etc/conf.d/songstead` (mode 0600). Its logrotate example uses copytruncate,
so rotation does not restart the application.

Native services bind loopback `127.0.0.1:8083`. Put the Caddy example in front,
configure `SONGSTEAD_BASE_URL` with the public address, and set
`SONGSTEAD_SECURE_COOKIES=true` for HTTPS. List only proxies you operate in
`SONGSTEAD_TRUSTED_PROXIES`; by default no forwarded address is trusted.
`SONGSTEAD_WITMOOT_URL` is optional and causes no startup network request.

Startup applies migrations atomically and refuses a database from a newer
binary. Stop the old service and back up the data directory before upgrading.
Do not run two services against the same directory; the server takes a file lock.
`backup --output PATH` creates a consistent SQLite snapshot without overwriting
an existing path. `restore --input PATH --data-dir EMPTY_DIRECTORY` validates the
schema, integrity and foreign keys before installing it. Restore requires a
snapshot matching this binary's schema; use the old binary to restore a v1 or v2
snapshot, then start the new binary to migrate it.

Schema 2 adds groups, canonical music identity, private music organization,
recordings, annotations, manual positions and discussion links. Duplicate v1
media rows merge without deleting recommendations, comments or original URLs.
If old duplicate recommendations have different listening states, the most
recent explicit choice wins (recommendation ID breaks time ties). Existing
personal notes and ratings remain distinct. Existing track comments receive
structured timestamp references during migration; ambiguous album comments remain prose. Account exports include the user's
annotation preference, positions, recording references, groups, discussion
links and authored structured annotations, without other people's private state.

Schema 3 adds explicit instance sharing and the Recent index. Every existing
recommendation defaults to private; neither direct nor group recommendations
are published to the shared feed by an upgrade. New browser forms choose
everyone here or a private audience explicitly. Older clients keep private
delivery. Once upgraded, restore requires a schema 3 snapshot; the account
export format remains version 2 with an additive Visibility field on each
recommendation. Back up before upgrading; reverting the binary alone cannot
downgrade the database.

The Comfyware Gentoo overlay provides versioned and live packages with native
services and private storage. Versioned packages use published, checksummed
source and dependency archives; live packages follow upstream `master`.
Follow [the release installation guide](releases.md) for Gentoo package setup.

## Example service configuration

Use your own public hostname in the service settings. These reserved domains
illustrate a deployment with an optional Witmoot discussion connection:

```sh
SONGSTEAD_ADDR="127.0.0.1:8083"
SONGSTEAD_DATA_DIR="/var/lib/songstead"
SONGSTEAD_BASE_URL="https://songstead.example.com"
SONGSTEAD_WITMOOT_URL="https://boards.example.com"
SONGSTEAD_SECURE_COOKIES="true"
SONGSTEAD_TRUSTED_PROXIES="127.0.0.1/32,::1/128"
```

Keep machine-specific settings and deployment notes outside this repository.
