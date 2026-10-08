# Install a Songstead release

Songstead publishes static Linux binaries for amd64 and arm64, a source archive,
and SHA-256 checksums in [GitHub Releases](https://github.com/airencracken/songstead/releases).
The binary includes SQLite, templates, JavaScript, and the jukebox artwork.
It needs no Go installation, C library, frontend build, or streaming account.

## Linux binary

Choose the archive for your machine and download the matching checksum file.
For the first release on amd64, using the GitHub CLI:

```sh
gh release download v0.1.0 --repo airencracken/songstead \
    --pattern songstead_0.1.0_linux_amd64.tar.gz \
    --pattern songstead_0.1.0_checksums.txt
sha256sum --check --ignore-missing songstead_0.1.0_checksums.txt
tar -xzf songstead_0.1.0_linux_amd64.tar.gz
cd songstead_0.1.0_linux_amd64
./songstead --version
```

Use `arm64` instead of `amd64` for an arm64 machine. Check the checksum command
succeeds before extracting or installing. Install the binary at
`/usr/local/bin/songstead`, then follow [deployment.md](deployment.md) to create
the service account, private data directory, local user accounts, and native
service. The packaged service examples use loopback port 8083 and keep
configuration and data private.

## Gentoo

Use the [Comfyware overlay](https://github.com/airencracken/comfyware#add-the-overlay)
for the binary, service account, OpenRC/systemd configuration, logging, and docs.
After accepting its testing keywords according to the overlay guide:

```sh
emaint sync -r comfyware
emerge --ask =www-apps/songstead-0.1.0::comfyware
```

See [Gentoo deployment](deployment.md) for the existing Gentoo/OpenRC host and optional
Witmoot discussion configuration. Each application works independently.

## Build the published source

Source builds need Go 1.26 or newer. Release builds resolve Comfylib v0.1.1 from
its published module rather than a sibling checkout:

```sh
git clone --branch v0.1.0 https://github.com/airencracken/songstead.git
cd songstead
GOWORK=off make check build
```

Back up the stopped data directory before upgrading an existing instance.
Startup applies migrations; reverting the binary cannot undo a schema upgrade.
Read [deployment.md](deployment.md) for backup and restore behavior.
