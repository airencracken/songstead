# Running your gathering

Build with `make build`. Run as an unprivileged account with a private data
directory. In the coordinated development workspace, the local Comfylib worktree
is required until v0.1.1 is released. The resulting binary runs independently.

For systemd, create a `songstead` service account, install the binary at
`/usr/local/bin/songstead`, and install `contrib/songstead.service` as
`/etc/systemd/system/songstead.service`. Provision accounts while running as the
service account, with `--data-dir /var/lib/songstead` and password input from
standard input. The directory must belong to the service account with mode 0700.
The service unit also provisions that state directory when first started.

Adapt `contrib/Caddyfile` to your domain. Caddy supplies HTTPS and proxies to
`127.0.0.1:8083`. Set `SONGSTEAD_SECURE_COOKIES=true` for the production service;
the example unit does this. Validate Caddy's complete configuration before
reloading it. Keep the internal listener on localhost.

Start with `systemctl enable --now songstead`. Review logs with
`journalctl -u songstead`. SIGTERM allows active requests to finish and stops
metadata retrieval cleanly. Health is available at `/healthz`. The native
listener port differs from Imvault's and Witmoot's defaults.

The data directory contains `songstead.db`, SQLite WAL/SHM files while active,
and a server lock. One server may run against it. Use the binary's backup command
for a consistent live snapshot, rather than copying a live database file alone.
Stop the service before switching directories after restore. Password recovery
uses the local `set-password` command, which revokes all of that account's sessions.

There is no SMTP, external identity provider, or invitation workflow in this
first version. The person running the instance creates accounts for friends.
