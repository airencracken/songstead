# Validation of the prepared 0.1.0 changes

Songstead `make check build` passes: the complete race-enabled Go suite, theme
JavaScript checks, 15 intentional mutation checks, packaging contracts, vet,
format checks and a static binary stamped `songstead 0.1.0`.

Tests cover current group access and removal, direct-recipient isolation,
duplicate music with distinct social notes, pagination without losing duplicate
provenance, private music organization, migration from schema 1, schema
constraints, annotation/response atomicity, natural multi-reference timestamps,
invalid tokens/durations, explicit album-recording selection, spoiler HTML
suppression, manual positions, export privacy and unavailable providers.
Recent coverage includes member-only access, private friend/group exclusion
across all views, shared/private duplicates, schema 2 migration preserving
privacy, private state on shared music, stable pagination across private posts,
atomic failed creation, randomized audience properties, native audience choices,
legacy forms, mixed/duplicate/invalid audiences, CSRF and failed private drafts.

`GOCACHE=/tmp/songstead-go-cache python3 scripts/check_companion_mutations.py`
passes three coordinated disposable-worktree mutations: credential-free shared
URLs, Witmoot rejecting private album metadata, and imvault requiring current
public-album visibility. This optional development check needs sibling source
worktrees; ordinary app tests and runtime do not depend on sibling applications.

Comfylib's reference/token/root tests and vet pass, including the exported API
and license-header checks. Its full existing suite was run; only socket-based
SMTP tests are blocked by the sandbox's socket prohibition.

Witmoot's new draft, access, escaped preview, private/malformed/unavailable album,
disconnect and CSRF tests pass under the race detector; imvault's new API,
public-image-count, ownership, private handoff and schema/store/configuration
checks pass too. Their existing full suites were run. Listening socket tests
cannot run here, and release/module-copy tests require published Comfylib v0.1.1
checksums. These are unresolved release gates, not passing tests.

Website: all 11 content checks, 6 deployment checks and Caddy validation pass.
HTTP checks cannot create sockets. Playwright's package is not cached for offline
installation; an independent Chromium launch also fails at socket setup.
Visual/browser validation must run outside this sandbox before deployment.

Overlay: Songstead live/staged recipe, account, compile-version and recipe parity
checks pass; the staged install checks the binary, private service configuration,
logrotate and journald output. Existing recipe, Manifest, dependency-bundle
immutability/download failure, publication and sandbox-packaging checks pass.
The three companion preparation checks pass for the proposed Witmoot 0.14.0
and imvault 0.16.0 recipes, including live-service parity, version stamps and
absence of fabricated Manifest entries. These recipes remain staged until
the proposed upstream tags and real distfiles are verified and published.
A full Portage build/install still needs Gentoo and published source/dependency
archives. No fabricated Manifest entries were created.

No GitHub repository, module or application release, overlay dependency bundle,
or live website deployment has been published from this environment.
