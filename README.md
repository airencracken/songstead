# Songstead

Good music, from your people.

[Source repository](https://github.com/airencracken/songstead). See the [release installation guide](docs/releases.md).

Songstead is a small, self-hosted recommendation shelf for friends. Paste a link,
share it with everyone here or choose a friend or group privately, and add a note. Listen when you have a moment, keep your own
listening state and rating, and talk about it together.

It is part of Comfyware: software for a small community, run by the people using
it. The first version follows Witmoot's Go/SQLite/HTML structure and Imvault's
quiet blue panels, with a little jukebox to keep the songs company.

## Songstead 0.2.0

Friends and groups can leave tracks, albums, artists and listening links. Browse
by music, person, group or each recommendation. Duplicate provider identities
keep each sender's words while sharing your private listening organization.
Recommendations are gifts: there is no need to keep up.

Recent shows what people shared with everyone on the instance, newest first.
Choose an audience before posting; private friend and group gifts stay out of
Recent. All views require sign-in. Sharing does not fill anyone else's shelf.

Comments can mark several moments in a chosen recording. Show annotations
immediately, reveal them manually, or use spoiler-free mode with a position you
indicate yourself. Songstead neither tracks playback nor connects streaming
accounts. Explicit Witmoot drafts and saved discussion links keep conversation
optional. Every application remains usable on its own.

Read [the shelf and annotation behavior](docs/quiet-inbox.md),
[deployment and migration details](docs/deployment.md), and
[the publication order](docs/release-coordination.md). Go/SQLite/HTMX,
AGPL-3.0-or-later, with `master` as the repository default branch.

## Build and try it

Requires Go 1.26 or later. Dependencies are pinned, including pure-Go SQLite;
`CGO_ENABLED=0` builds a standalone binary with templates and assets embedded.

Release builds use published Comfylib v0.1.2 and verified module checksums.
Build and test independently of any development workspace:

```sh
GOWORK=off make check build
make demo
```

`make test-browser` adds Chromium administration and accessibility checks.
Install Playwright Chromium from `scripts/browser`, or set `CHROMIUM` to a local
executable. The browser suite uses disposable accounts and deletes its database.

For local changes to the shared library, use an untracked `go.work` with the
sibling Comfylib worktree. Keep workspace files and local replacements out of
commits and release builds. See [release coordination](docs/release-coordination.md).

The demo listens at `http://127.0.0.1:8083`. Sign in as `alice` or `bobby`, both
with `demo-password`. It uses a private temporary directory and removes it when
stopped. Send a link as one account, then sign in as the other to try the shelf.

For a permanent instance, provision an owner locally using a hidden password
prompt. Passwords are entered twice and never passed as command arguments:

```sh
songstead create-owner --username alex --password-prompt
songstead create-user --username freya --password-prompt
songstead list-users
```

On a packaged installation, these commands discover the data directory in the
active OpenRC or systemd service configuration. When invoked as root, they run
as the configured service account to preserve database ownership. An explicit
`--data-dir` overrides `SONGSTEAD_DATA_DIR`, which overrides service settings.
Portable installations default to `./data` and run as an unprivileged user.

For scripts, use `--password-stdin` instead of `--password-prompt`. For example,
in Bash:

```sh
if read -r -s -p 'Password: ' songstead_password; then
    printf '\n'
    if ! printf '%s\n' "$songstead_password" | ./bin/songstead create-user --username alice --password-stdin; then
        printf '%s\n' 'Account creation failed.' >&2
    fi
    unset songstead_password
else
    printf '%s\n' 'Password input was cancelled.' >&2
fi
```

Repeat for your friend, then run `./bin/songstead serve`. Account names use 3–24
letters, digits, underscores, or dashes. Passwords need at least 12 characters
and at most 72 bytes. `set-password` has the same options and revokes every
session for the account. `create-owner` only creates a new owner: it never
promotes, replaces, or resets an existing account, including a username differing
only in case. Existing accounts stay members during upgrade. Owners have the
same music privacy boundaries as members. Sign in as an owner and open
**Admin** to configure the instance, invite people, manage account roles and
suspension, and issue one-hour password recovery links. Members can change
their own passwords from **Your account**.

Joining is invitation-only by default. Owners can delegate invitation
permission or choose closed or open joining. Public joining never makes music
pages public. Invitation links are shown once and support expiry, use limits
and revocation. Existing accounts remain members during an upgrade; explicitly
promote your own existing account with:

```sh
songstead set-role --username alex --role owner
```

Set the public Songstead and Witmoot addresses in **Admin → Instance settings**.
Saved settings apply immediately and persist across restarts. A blank Witmoot
address disables the connection even if the service has an environment default.
See [administration and invitations](docs/administration.md) for the full flow.
Use `songstead help COMMAND` or `COMMAND --help` for CLI flags.

## Configuration and hosting

| Setting | Default | Purpose |
| --- | --- | --- |
| `SONGSTEAD_BASE_URL` | empty | Default public origin for discussion, invitation and recovery links |
| `SONGSTEAD_WITMOOT_URL` | empty | Default Witmoot base URL; editable in Admin; no startup request |
| `SONGSTEAD_DATA_DIR` | `./data` | Private directory containing SQLite and the server lock |
| `SONGSTEAD_ADDR` | `127.0.0.1:8083` | Listen address |
| `SONGSTEAD_SECURE_COOKIES` | `false` | Set `true` for an installation reached over HTTPS |
| `SONGSTEAD_TRUSTED_PROXIES` | empty | Comma-separated proxy IPs/CIDRs; trust forwarding headers only from these |

`--data-dir` and `--addr` override their environment settings. Use `--help` for
commands. The data directory must have mode `0700`; new directories are created
that way. Run the app as its own unprivileged service account.

Put a reverse proxy in front of the localhost listener. A Caddy example is in
[contrib/Caddyfile](contrib/Caddyfile), alongside a
[systemd service](contrib/songstead.service). See [deployment](docs/deployment.md)
for setup and account provisioning. `/healthz` checks SQLite availability.

## Back up and restore

```sh
./bin/songstead backup --output /private/backups/songstead-2026-10-07.db
./bin/songstead restore --input /private/backups/songstead-2026-10-07.db --data-dir /private/restored-songstead
```

Backup uses SQLite's consistent snapshot operation, including committed WAL
contents; it can run while the server is active. It validates the snapshot and
publishes a private file atomically, refusing to overwrite an existing target.
Backups contain account password hashes, sessions, and private recommendations.
Keep them private. Store backup files outside the served web root.

Restore requires an empty, private destination directory. It copies into a
temporary file, checks schema compatibility, integrity, and foreign keys, then
publishes the database without overwriting an existing file. Stop the service
before switching it to the restored directory. Test restoration periodically.

Signed-in users can download their history as JSON. This preserves accessible
recommendations, their own reactions and notes, and comments they authored.
Credentials and other people's personal notes are excluded. It is a portable
record; import and account deletion are not implemented yet.

## Checks

```sh
make test
make check
```

`make check` runs race-enabled Go tests, JavaScript unit tests, mutation tests,
vet, and formatting checks. Tests cover the two-friend HTTP workflow without
JavaScript, route and export contracts, permissions, schema constraints,
transactions and failed writes, concurrent submissions, property checks,
adversarial forms and URLs, session revocation, metadata failures, SSRF, and
backup/restore. Fuzz seeds run in ordinary tests; run the parser continuously with
`go test ./internal/media -fuzz=FuzzURLParsing -fuzztime=10s`.

The app does not need a frontend build step, CDN, external database, or music
provider account. Metadata lookup runs after submission and retries at most
three times. A failed lookup leaves the original link intact.

Read the [architecture notes](docs/architecture.md) for domain and permission
decisions. The [mascot provenance and prompt](design/mascot.md) document the
built-in imagegen artwork. HTMX's license is bundled; see
[THIRD_PARTY.md](THIRD_PARTY.md). Licensed under AGPL-3.0-or-later.

## Support

You can [support Songstead on Ko-fi](https://ko-fi.com/airencracken).

See [Bubblewrap service isolation](docs/sandbox.md) for the optional confined launcher and native service settings.
