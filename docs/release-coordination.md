# Coordinated releases

The coordinated release uses the published Comfylib module, real upstream
source archives and immutable Gentoo dependency bundles. Each app keeps its
own accounts, database and runtime; the discussion connections are optional.

| Component | Published version | Release |
| --- | --- | --- |
| Comfylib | 0.1.1 | [Shared library](https://github.com/airencracken/comfylib/releases/tag/v0.1.1) |
| Songstead | 0.1.0 | [Linux binaries and source](https://github.com/airencracken/songstead/releases/tag/v0.1.0) |
| Witmoot | 0.14.0 | [Linux binaries, Debian packages and source](https://github.com/airencracken/witmoot/releases/tag/v0.14.0) |
| Imvault | 0.16.0 | [Linux binaries, Debian packages and source](https://github.com/airencracken/imvault/releases/tag/v0.16.0) |

Comfylib adds `token.SessionCSRF` and the `reference` browser handoff package.
Existing Witmoot and Imvault CSRF purpose strings and outputs are preserved.
All three applications pin v0.1.1 and its verified Go checksum database records.
Clean builds and release workflows use `GOWORK=off`, without local replacements.

Songstead's public repository uses `master` as its default branch and
AGPL-3.0-or-later licensing. Its v0.1.0 tag is on master and matches VERSION.
GoReleaser publishes static Linux amd64/arm64 archives, source, license notices
and SHA-256 checksums after the full standalone release checks pass.

Witmoot v0.14.0 accepts deliberate `/share` discussion drafts. Imvault v0.16.0
adds album discussion links and its authenticated
`/api/v1/albums/{ref}/preview` endpoint. Optional previews check current public
visibility; private albums retain plain links. Preparing a draft creates no
thread. Choose a destination, review its audience, then post. Older instances
continue to work independently, with no preview when unsupported.

## Gentoo

The Comfyware overlay contains versioned and live recipes for all three apps.
The versioned recipes use verified published sources and these dependency
releases:

- [Songstead 0.1.0 dependencies](https://github.com/airencracken/comfyware/releases/tag/songstead-0.1.0)
- [Witmoot 0.14.0 dependencies](https://github.com/airencracken/comfyware/releases/tag/witmoot-0.14.0)
- [Imvault 0.16.0 dependencies](https://github.com/airencracken/comfyware/releases/tag/imvault-0.16.0)

Portage generated the BLAKE2B/SHA512 Manifests from the actual archives.
All three packages passed full builds, upstream tests and installation in an
official Gentoo stage3 container with its external network disconnected.
Service accounts, private permissions, provisioning and health routes were
checked. Songstead also passed installed backup/restore checks; Imvault's
preview API and Witmoot's draft route enforce authentication.

Follow [releases.md](releases.md) and [Gentoo deployment](deployment.md) to install. Stop and
back up existing apps before their schema upgrades. Publication does not
change services, DNS, proxies or data on the example host.

## Website and funding

The [website repository](https://github.com/airencracken/comfyware_org) includes
Songstead's release page, installation links, privacy FAQ and square jukebox
artwork, plus the companion discussion descriptions. Content, HTTP, deployment,
Caddy and responsive browser checks pass. Publish its document root using the
existing server procedure (`git pull`, then `make deploy`). Repository updates
do not perform that host deployment.

Songstead's README Support section and enabled GitHub Sponsor sidebar link
[Ko-fi](https://ko-fi.com/airencracken), matching Witmoot and Imvault.

See [validation.md](validation.md) for test coverage and CI evidence. Local
`go.work` files remain untracked review conveniences. Earlier workspace-built
preview archives are not release artifacts; use the verified published downloads.
