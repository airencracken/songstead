# Coordinated releases

The coordinated release uses the published Comfylib module, real upstream
source archives and immutable Gentoo dependency bundles. Each app keeps its
own accounts, database and runtime; the discussion connections are optional.

| Component | Published version | Release |
| --- | --- | --- |
| Comfylib | 0.1.4 | [Shared library](https://github.com/airencracken/comfylib/releases/tag/v0.1.4) |
| Songstead | 0.4.1 | [Linux binaries and source](https://github.com/airencracken/songstead/releases/tag/v0.4.1) |
| Witmoot | 0.14.2 | [Linux binaries, Debian packages and source](https://github.com/airencracken/witmoot/releases/tag/v0.14.2) |
| Imvault | 0.16.2 | [Linux binaries, Debian packages and source](https://github.com/airencracken/imvault/releases/tag/v0.16.2) |

Comfylib adds shared password confirmation and branding image normalization,
and retains `token.SessionCSRF` and the `reference` browser handoff package.
Existing Witmoot and Imvault CSRF purpose strings and outputs are preserved.
Songstead pins Comfylib v0.1.4 for reusable profile-image validation. Imvault
and Witmoot retain v0.1.3; their existing image handling is unchanged. All pins
use verified Go checksum database records.
Clean builds and release workflows use `GOWORK=off`, without local replacements.

Songstead's public repository uses `master` as its default branch and
AGPL-3.0-or-later licensing. Its v0.4.1 tag is on master and matches VERSION.
Songstead adds owner settings, invitations, account recovery and suspension.
GoReleaser publishes static Linux amd64/arm64 archives, source, license notices
and SHA-256 checksums after the full standalone release checks pass.

Witmoot v0.14.2 accepts deliberate `/share` discussion drafts. Imvault v0.16.2
adds album discussion links and its authenticated
`/api/v1/albums/{ref}/preview` endpoint. Optional previews check current public
visibility; private albums retain plain links. Preparing a draft creates no
thread. Choose a destination, review its audience, then post. Older instances
continue to work independently, with no preview when unsupported.

## Gentoo

The Comfyware overlay contains versioned and live recipes for all three apps.
The versioned recipes use verified published sources and these dependency
releases:

- [Songstead 0.4.1 dependencies](https://github.com/airencracken/comfyware/releases/tag/songstead-0.4.1)
- [Witmoot 0.14.2 dependencies](https://github.com/airencracken/comfyware/releases/tag/witmoot-0.14.2)
- [Imvault 0.16.2 dependencies](https://github.com/airencracken/comfyware/releases/tag/imvault-0.16.2)

The BLAKE2B/SHA512 Manifests record the actual published archives.
All three packages passed builds, upstream tests and the overlay installation harness in an
official Gentoo stage3 container with its external network disconnected.
The native release checks cover service accounts, private permissions,
provisioning and health routes. Songstead also covers backup/restore; Imvault's
preview API and Witmoot's draft route enforce authentication.

Follow [releases.md](releases.md) and [Gentoo deployment](deployment.md) to install. Stop and
back up existing apps before their schema upgrades. Publication does not
change services, DNS, proxies or application data.

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
