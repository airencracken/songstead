# Coordinated suite releases

These releases use the published Comfylib module and independent application
accounts, databases and services. Configure the optional integrations only when
wanted; upgrading does not enable shared sign-in or add app links automatically.

| Component | Version | Release |
| --- | --- | --- |
| Comfylib | 0.2.0 | [Shared library](https://github.com/airencracken/comfylib/releases/tag/v0.2.0) |
| Songstead | 0.13.0 | [Binaries and source](https://github.com/airencracken/songstead/releases/tag/v0.13.0) |
| Witmoot | 0.20.0 | [Binaries, Debian packages and source](https://github.com/airencracken/witmoot/releases/tag/v0.20.0) |
| Imvault | 0.22.0 | [Binaries, Debian packages and source](https://github.com/airencracken/imvault/releases/tag/v0.22.0) |
| Folkweave | 0.3.0 | [Binaries, Debian packages and source](https://github.com/airencracken/folkweave/releases/tag/v0.3.0) |
| Tidings | 0.5.0 | [Binaries, Debian packages and source](https://github.com/airencracken/tidings/releases/tag/v0.5.0) |

All five apps pin Comfylib v0.2.0 with checksum database verification and
`GOWORK=off`, without local replacements. Comfylib remains standard-library-only.
Source builds require Go 1.26.11 or newer. Release workflows scan source and
packaged executables and verify native artifacts before publication. Multi-platform
container images must pass installation, upgrade, backup and restore checks before
publication, including anonymous registry access.

Shared controls preserve each app's palette. App menus use explicitly configured
URLs. Community sign-in requires an existing local account to connect an identity;
there is no automatic provisioning or email matching. Applications retain their
own roles and permissions. See [suite integration](suite.md).

Discussion returns preserve the author's source and destination audience checks.
A source app saves a discussion link only after its signed-in author confirms a
normal CSRF-protected form. Preparing a draft creates no thread, and posting a
thread does not change an album or recommendation audience.

Back up the stopped data directory before upgrading. Songstead applies schema 11
for the search index; large libraries need time for the first index build. Shared
sign-in creates an additive identity-binding table when configured. Restore the
matching pre-upgrade backup when downgrading. Account export format 6 is unchanged.

## Gentoo and website

The Comfyware overlay uses each app's real published source archive and immutable
Go dependency bundles. Manifests record the bytes of published distfiles. New
recipes are published after their upstream application releases are available.
The website release catalog is updated only after native releases and anonymously
pullable container images are published.

Repository publication does not change running services, DNS, proxies or user data.
The website has a separate host deployment procedure (`git pull`, then
`make deploy`); repository updates do not deploy it.

Follow [installation](releases.md), [containers](containers.md), and
[deployment](deployment.md) for the supported installation and upgrade procedures.
