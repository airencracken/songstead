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

Comfylib's prepared candidate `411cfde` passes `make check` with local sockets
enabled: formatting, vet, complexity, lint, race tests (including SMTP),
mutation-engine checks, and all 138 mutations. Its URL validator was refactored
to meet the existing complexity limit without changing behavior. New tests
cover valid/invalid port boundaries and generated ports; four new mutations
cover port limits, credential exclusion, and duplicate handoff fields. The
handoff fuzz check also passes after 109,030 executions. Exported API and
license-header checks pass. Optional real Bubblewrap and nginx/Apache checks
were not run in this continuation; no changes were made to those packages.

Witmoot's new draft, access, escaped preview, private/malformed/unavailable album,
disconnect and CSRF tests pass under the race detector; imvault's new API,
public-image-count, ownership, private handoff and schema/store/configuration
checks pass too. Their existing full suites were run. Listening socket tests
cannot run here, and release/module-copy tests require published Comfylib v0.1.1
checksums. These are unresolved release gates, not passing tests.

Website rechecked on 2026-10-08: all 12 content checks, 9 production HTTP
checks, 6 deployment checks, Caddy validation and 306 Chromium checks pass.
Pinned browser dependencies were installed and local socket checks ran outside
the restricted sandbox. Browser coverage includes both palettes at four widths,
accessibility, full square Songstead artwork, and keyboard use of its privacy
FAQ without JavaScript. Desktop and mobile screenshots were reviewed; the
product page now uses the intended mascot styling instead of cropping it as a
wide screenshot. The website remains unpublished.

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

On 2026-10-08, the overlay's complete local suite and shellcheck pass. CI now
includes Songstead and companion preparation checks. All three staged recipes
pass installation checks against their actual local source checkouts. Additional
mutations reject public data directories, exposed configuration, missing logging,
wrong service commands, and compressed examples. Dependency publication tests
cover all three apps and reject inherited development workspaces; resolution
now explicitly uses `GOWORK=off`.

Songstead's 9 packaging checks also pass, including OpenRC's `serve` command,
private path setup, default settings, rejection of relative/adversarial paths
before changes, stopping after failed directory setup, and CI's standalone
dependency/checksum checks. The complete `make check build` was rerun and
passes with the updated local library candidate, producing `songstead 0.1.0`.
This still uses the development workspace, rather than a published dependency.

Website and overlay default branches were pulled before this continuation.
Songstead's remote initially returned `Repository not found`; its public
GitHub repository was subsequently created under `airencracken` on 2026-10-08,
and the source was pushed to `master` after explicit publication approval.
Comfylib's remote still has no `v0.1.1` tag. Clean standalone dependency resolution remains a release
gate. GitHub CI confirms `unknown revision v0.1.1` during dependency resolution.
The earlier Witmoot/Imvault socket-suite limitations above describe the previous
run; those full application suites have not been rerun in this continuation.

The public Songstead GitHub repository has been created and its source pushed
to the `master` default branch. No module or
application release, overlay dependency bundle, or live website deployment
has been published from this environment.
