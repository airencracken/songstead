# Coordinated release

Changes live in separate worktrees for Songstead, Comfylib, Witmoot, Imvault, and
comfyware_org. The companion applications now use the shared CSRF function with
their existing purpose strings, preserving their session token formats.

Before publishing application releases:

1. Review and release Comfylib's additive `token.SessionCSRF` API as v0.1.1.
2. Resolve v0.1.1 through the normal Go module proxy and record its real checksums
   in each application's `go.sum`. Do not invent checksums for the unpublished tag.
3. Run clean builds and the complete test suites with `GOWORK=off`. Run Imvault's
   HTTP and SMTP checks and the website's Caddy/browser checks where sockets work.
4. Publish the application source/releases and update Songstead's public website
   page with real source and installation links when they exist.

For development before the library release, use the untracked `go.work` files
beside each app. They select the sibling Comfylib checkout and replace the
specific pending module version. No `replace` directive belongs in committed
application `go.mod` files.

The website introduces Songstead as in development and makes no release or
installation availability claim. Website changes have been prepared, not deployed.
