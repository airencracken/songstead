# Install a Songstead release

Songstead publishes static Linux binaries for amd64 and arm64, a source archive
and SHA-256 checksums on [GitHub Releases](https://github.com/airencracken/songstead/releases).
The binary includes SQLite, templates and artwork. It needs no Go, C library or
music service account. After installing, create the first owner with
[setup](setup.md).

## Linux binary

On amd64, using the GitHub CLI:

```sh
gh release download v0.17.1 --repo airencracken/songstead \
    --pattern songstead_0.17.1_linux_amd64.tar.gz \
    --pattern songstead_0.17.1_checksums.txt
sha256sum --check --ignore-missing songstead_0.17.1_checksums.txt
tar -xzf songstead_0.17.1_linux_amd64.tar.gz
cd songstead_0.17.1_linux_amd64
./songstead --version
```

Use `arm64` on an arm64 machine. Only extract the archive after the checksum
passes. Install the binary at `/usr/local/bin/songstead`, then follow
[deployment](deployment.md) for the service account, data directory and service.

## Gentoo

The [Comfyware overlay](https://github.com/airencracken/comfyware#add-the-overlay)
builds Songstead from source and installs the service account, OpenRC and
systemd configuration, logging and documentation. After accepting its testing
keywords as the overlay guide describes:

```sh
emaint sync -r comfyware
emerge --ask =www-apps/songstead-0.17.1::comfyware
```

## Build the published source

Source builds need Go 1.26.11 or newer and use the published Comfylib v0.7.1:

```sh
git clone --branch v0.17.1 https://github.com/airencracken/songstead.git
cd songstead
GOWORK=off make check build
```

## Upgrading

Back up the stopped data directory first and keep that copy for a downgrade.
Startup applies migrations, and an older binary cannot open a migrated
database. The next release applies schema 12, which deletes the sign-in and view
records kept by 0.15 and 0.16. Upgrading from 0.12 or earlier also builds the
search index (schema 11) on first start. See the [changelog](../CHANGELOG.md)
for each release.

## Rebuild an existing release tag

Maintainers can run the Release workflow on `master` with `release_tag` set to
an existing tag. It checks that the tag points at a commit on `master` that
matches VERSION, then reruns every native, package and container check.
Published releases and image tags never change; a rerun can only finish an
unpublished draft.
