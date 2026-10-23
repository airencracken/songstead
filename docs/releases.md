# Install a Songstead release

Songstead publishes static Linux binaries for amd64 and arm64, a source archive,
and SHA-256 checksums in [GitHub Releases](https://github.com/airencracken/songstead/releases).
The binary includes SQLite, templates, JavaScript, and the jukebox artwork.
It needs no Go installation, C library, frontend build, or streaming account.

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
gh release download v0.11.2 --repo airencracken/songstead \
    --pattern songstead_0.11.2_linux_amd64.tar.gz \
    --pattern songstead_0.11.2_checksums.txt
sha256sum --check --ignore-missing songstead_0.11.2_checksums.txt
tar -xzf songstead_0.11.2_linux_amd64.tar.gz
cd songstead_0.11.2_linux_amd64
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
emerge --ask =www-apps/songstead-0.11.2::comfyware
```

See [deployment.md](deployment.md) for native services and optional Witmoot
discussion configuration. Each application works independently.

## Build the published source

Source builds need Go 1.26 or newer. Release builds resolve Comfylib v0.1.6 from
its published module rather than a sibling checkout:

```sh
git clone --branch v0.11.2 https://github.com/airencracken/songstead.git
cd songstead
GOWORK=off make check build
```

Back up the stopped data directory before upgrading an existing instance.
Startup applies migrations; reverting the binary cannot undo a schema upgrade.
Read [deployment.md](deployment.md) for backup and restore behavior.

## Published container images

See [Docker installation and operations](containers.md) for multi-platform images,
Compose setup, version pins, backups, upgrades, and restore instructions.
