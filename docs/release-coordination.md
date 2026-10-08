# Coordinated release preparation

Songstead 0.1.0 is prepared on `master`, under AGPL-3.0-or-later, using Go,
SQLite and HTMX. Comfylib, Witmoot, imvault, the Gentoo overlay and website have
separate changes; there is no shared database or mandatory runtime service.

1. Publish Comfylib v0.1.1, including the additive session CSRF helper and
   `reference` browser handoff package. Existing Witmoot/imvault CSRF purpose
   strings and outputs remain unchanged.
2. In each app, resolve that real tag with `GOWORK=off go mod download`, record
   its verified checksums, and run clean builds and the complete test suites.
   Never invent checksums for the pending library tag.
3. Fetch and verify that the proposed companion tags are unused, then publish
   Witmoot v0.14.0 and imvault v0.16.0 (the inspected local tags end at v0.13.0
   and v0.15.0 respectively). Prepared release notes describe their operator
   changes. Songstead uses Witmoot's `/share` draft
   chooser. Album previews use imvault's authenticated
   `/api/v1/albums/{ref}/preview` endpoint; older instances simply yield no
   preview and keep the destination links usable.
4. Create `airencracken/songstead` as a public repository, push `master`, set
   its default branch to `master`, and confirm CI. Tag `v0.1.0` on that branch.
   The GoReleaser workflow produces static Linux amd64/arm64 archives, source,
   license notices and SHA-256 checksums. Publication requires the tag to be
   on master, match VERSION, and pass the full checks with GOWORK disabled.
5. Generate and verify the overlay's Songstead dependency bundle. Publish it
   under overlay tag `songstead-0.1.0`, move the staged versioned ebuild into
   `www-apps/songstead`, generate real BLAKE2B/SHA512 Manifest hashes, and run
   package/install checks. The live ebuild and service accounts are prepared.
   Companion recipes are staged at `release-preparation/witmoot-0.14.0.ebuild`
   and `release-preparation/imvault-0.16.0.ebuild`. Generate their dependency
   bundles from the actual new source archives, publish under overlay tags
   `witmoot-0.14.0` and `imvault-0.16.0`, and add real Manifest entries before
   enabling the recipes. Update the README's published version table at that
   point; currently it continues to name the existing releases.
6. Add verified public repository/release links to the website, run its content,
   Caddy and browser checks, then deploy through its existing workflow.

The local `go.work` files select the sibling Comfylib checkout during review.
They remain untracked, with no replacement in application go.mod files.
Local review archives live in `.artifacts/songstead-0.1.0-preview`; they are
explicitly unpublished and were built using that workspace. They are not a
substitute for resolving the released module and passing clean release checks.

GitHub DNS, listening sockets and browser socket setup are unavailable in the
current sandbox. Remote creation/push, dependency publication, full socket-based
checks, Gentoo installation, and deployment must run where those capabilities
are available. Website copy accurately labels 0.1.0 as in preparation.

See [validation.md](validation.md) for checks and unresolved release gates.
