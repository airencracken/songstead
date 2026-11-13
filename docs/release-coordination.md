# Coordinated suite releases

This page is for maintainers releasing Songstead alongside the other Comfyware
apps. Each app has its own accounts, database and service. Upgrading never turns
on shared sign-in or adds app links; operators configure those in
[Comfyware connections](suite.md).

All five apps pin published Comfylib and Comfylib OIDC versions with checksum
verification and `GOWORK=off`. Release builds never use a `go.work` file or
local `replace` directives. For local work on Comfylib, use an untracked
`go.work` with a sibling Comfylib worktree and keep it out of commits.

## Order of publication

1. Publish Comfylib.
2. Publish each app's native release, then its container images. Images must
   be pullable anonymously before the release is announced.
3. Update the Comfyware Gentoo overlay from the published source archives.
4. Update the website release catalog. Deploying the website is a separate step
   on its host.

Publishing changes no running service, DNS, proxy or user data.

## Upgrade notes for operators

Back up the stopped data directory before upgrading. The current Songstead
schema is 12. Schema 11 builds a search index on first start, which takes a
while on large libraries. Schema 12 deletes per-account and per-item activity
records. To downgrade, restore the matching pre-upgrade backup. Account export
format 6 is unchanged.

See [installation](releases.md), [containers](containers.md) and
[deployment](deployment.md).
