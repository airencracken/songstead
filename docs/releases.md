# Install a Songstead release

For first-run accounts, use [browser or command-line setup](setup.md). Run
`songstead setup` for the browser key, or `songstead create-owner --username NAME`
for a hidden terminal prompt.

Songstead publishes static Linux binaries for amd64 and arm64, a source archive,
and SHA-256 checksums in [GitHub Releases](https://github.com/airencracken/songstead/releases).
The binary includes SQLite, templates, JavaScript, and the jukebox artwork.
It needs no Go installation, C library, frontend build, or streaming account.

## Upgrade to 0.15.0

Schema 11, introduced in 0.13.0, builds the music search index at startup.
Version 0.15.0 uses the shared admin dashboard and records successful sign-ins
and authorized recommendation opens through Comfylib. Activity tables are
additive and separately versioned. Collection begins after upgrade and retains
90 UTC days; existing application schema and export versions are unchanged.
Back up the stopped data directory before upgrading, allow time for the initial
index build, and retain the pre-upgrade backup for a downgrade. Optional shared
sign-in creates an additive identity-binding table when configured. Existing
accounts, audiences, music likes and account export format 6 are preserved.
See the [release notes](../CHANGELOG.md#0150) and [suite guide](suite.md).

## Upgrade from 0.10.x

Version 0.11.0 applies schema 10 at server startup. Back up the stopped data
directory first. Post corrections are stored separately from shared music metadata;
existing audiences, comments, likes and listening history are preserved. Missing
previews are queued again, including previously unsupported YouTube playlists.
Account export format 6 is unchanged. A downgrade requires the pre-upgrade database
backup as well as the earlier binary.

When upgrading from 0.9.x or earlier, schema 9 is applied first. The latest
private recommendation rating for each song sets
its initial song like; existing listening status, ratings and notes are preserved.
Account export format 6 includes your private song likes. A downgrade requires
the pre-upgrade database backup as well as the earlier binary.

## Linux binary

Choose the archive for your machine and download the matching checksum file.
For this release on amd64, using the GitHub CLI:

```sh
gh release download v0.15.0 --repo airencracken/songstead \
    --pattern songstead_0.15.0_linux_amd64.tar.gz \
    --pattern songstead_0.15.0_checksums.txt
sha256sum --check --ignore-missing songstead_0.15.0_checksums.txt
tar -xzf songstead_0.15.0_linux_amd64.tar.gz
cd songstead_0.15.0_linux_amd64
./songstead --version
```

Use `arm64` instead of `amd64` for an arm64 machine. Check the checksum command
succeeds before extracting or installing. Install the binary at
`/usr/local/bin/songstead`, then follow [deployment.md](deployment.md) to create
the service account, private data directory, local user accounts, and native
service. The packaged service examples use loopback port 8083 and keep
configuration and data private.

## Gentoo

The overlay provides a source-built package and offline dependency bundle.

Use the [Comfyware overlay](https://github.com/airencracken/comfyware#add-the-overlay)
for the binary, service account, OpenRC/systemd configuration, logging, and docs.
After accepting its testing keywords according to the overlay guide:

```sh
emaint sync -r comfyware
emerge --ask =www-apps/songstead-0.15.0::comfyware
```

See [deployment.md](deployment.md) for native services and optional Witmoot
discussion configuration. Each application works independently.

## Build the published source

Source builds need Go 1.26.11 or newer. Release builds resolve Comfylib v0.5.0 from
its published module rather than a sibling checkout:

```sh
git clone --branch v0.15.0 https://github.com/airencracken/songstead.git
cd songstead
GOWORK=off make check build
```

Back up the stopped data directory before upgrading an existing instance.
Startup applies migrations; reverting the binary cannot undo a schema upgrade.
Read [deployment.md](deployment.md) for backup and restore behavior.

## Published container images

See [Docker installation and operations](containers.md) for multi-platform images,
Compose setup, version pins, backups, upgrades, and restore instructions.

## Rebuild an existing release tag

Maintainers can dispatch the Release workflow on `master` with `release_tag` set
to an existing version tag. The workflow validates the tag, exact commit, VERSION
where present, and reachability from master. It uses Go 1.26.11 and runs the full
native, package and container gates against that tag. Container revision labels
identify the checked-out source commit. Published releases and image tags remain
immutable; reruns may finish an unpublished draft.
