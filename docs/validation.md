# Songstead 0.5.0 validation

`GOWORK=off make check` passes race-enabled Go tests, JavaScript tests, all
52 deliberate mutations, 15 packaging checks, CLI integration, vet and formatting.
New discussion tests cover owner-only configuration, all three UI/API modes,
legacy JSON and snapshot compatibility, rejected/failed-write rollback, unknown
and duplicate fields, policy changes across restarts, private audiences and
CSRF. A second database connection verifies a waiting comment writer observes
the committed policy before any comment or timestamp rows can be written.
Property checks exercise the mode allowlist. Browser flows include members
without Witmoot accounts, direct drafts and native local comments.

Tests also exercise metadata API contracts and bounded artwork fetching, provider
and DNS allowlists, private thumbnail routes, audience-scoped labels, sender-only
editing, genre/tag validation, failed-write rollback, schema upgrades through 8,
backup compatibility, exports, persisted preferences, favorite ordering and
pagination, exclusions, account spoiler settings and saved-feedback notices.
Tests also cover preview API and authorization contracts, account motion settings,
picture replacement/removal, profile schema constraints and export isolation.
Chromium verifies stale response suppression, automatic artwork refresh, direct
layout switching with and without JavaScript, and the actual origin-only referrer
sent to the YouTube iframe. Property checks cover label normalization; adversarial tests cover URLs, images,
SQL punctuation, invalid forms and unauthorized mutations.

The Chromium suites pass 95 music/preferences checks and 31 administration
checks across desktop/light and mobile/dark views. Both native and HTMX forms
save preferences and listening feedback, confirmations remain in view, unsaved
edits clear old confirmations, and another account cannot read private notes.
List and Tiles use authenticated local artwork with a fallback. List remains
the default, with compact responsive thumbnails. Collapsed advanced filters
apply together, while direct view and discovery switches preserve applied filters.
User settings have a clear navigation link and working section shortcuts. Tested pages have no horizontal overflow or automated WCAG A/AA
violations, and no external thumbnail requests or JavaScript errors occur.

Live smoke checks retrieve normalized artwork from YouTube, Spotify and
SoundCloud sample recordings. Spotify supports both its documented image host
and the current CDN returned by its public oEmbed endpoint.

Real Bubblewrap integration passes with the new schema and static executable.
The URL parser also passes a bounded fuzz run. Comfylib is pinned at v0.1.5;
its new profile-image package keeps validated GIF animations and PNG stills.
Existing artwork uses the shared brand-image normalizer. Companion apps retain
their own pins and require no changes.

# Previous administration release validation

Songstead 0.2.1, Imvault 0.16.2 and Witmoot 0.14.2 pin the published
Comfylib v0.1.3 module and verified Go checksum database records. Release checks
use `GOWORK=off`; no local module replacements participate in the builds.

## Shared library

[Comfylib release CI](https://github.com/airencracken/comfylib/actions/runs/37861496561)
passes formatting, vet, complexity, lint, race tests, API golden checks,
fuzzing, real sandbox/proxy integration, the mutation engine and all 144
mutations. Password tests use real pseudo-terminals to verify echo is disabled
before an action can show a prompt, unrelated flags are preserved, and terminal
state is restored on success, error and panic. Invalid descriptors and restoration
failures return no password. Property tests cover terminal flag combinations and
confirmation values. Shared image normalization covers size limits, malformed
images, metadata removal, transparency and decoding bounds.

## Songstead

[Native release CI](https://github.com/airencracken/songstead/actions/runs/37861732901)
and [master checks](https://github.com/airencracken/songstead/actions/runs/37861732844)
pass standalone race tests, JavaScript checks, all 34 mutations, 15 packaging
contracts, real terminal provisioning, Bubblewrap, formatting and vet.
The published source and amd64 binary pass SHA-256 verification. Source module
inputs and migration 005 match the tag used to prepare the dependency bundle.
The downloaded executable reports `songstead 0.2.1` and passes terminal prompt
checks. A separate stress run passed 25 consecutive immediate-input provisions.

Store and HTTP tests cover invitation expiry, use limits, creator permissions,
concurrent joining and rollback, owner-only settings and account changes,
last-owner protection across separate database handles, suspension, recovery
replacement and replay, password/session atomicity, older-schema migration,
schema validation, route and CSRF contracts, multipart bounds and atomic branding.
Audience property tests preserve private recommendations, groups and listening
notes across administration changes. Owners retain the same private access rules.

Chromium passes 28 administration checks across desktop/light and mobile/dark
layouts, including settings, invitation creation and joining, recovery, replay,
keyboard operation, overflow checks and accessibility. No JavaScript errors or
accessibility violations were found in the checked pages.

## Companions

Imvault and Witmoot pass local release checks, full race suites and real terminal
provisioning. Their terminal regression runner is copied byte-for-byte from the
pinned Comfylib release and checked against that module. Applications retain their
own prompt labels, password policies, storage and authorization.

[Imvault release workflow](https://github.com/airencracken/imvault/actions/runs/37861950521)
and [Witmoot release workflow](https://github.com/airencracken/witmoot/actions/runs/37861855967)
gate publication on repository tests, all mutation tables, real Bubblewrap,
nginx/Apache integration and Ubuntu/Debian package lifecycle checks on amd64
and arm64. Imvault has 104 deliberate mutations; Witmoot has 66.

## Gentoo and website

All three exact patch tags and dependency bundles build and pass internal Go
tests in an official Gentoo stage3 container with networking disabled. The
versioned and live recipes pass the unprivileged installation harness for
service commands, private paths, configuration permissions, logging and sandbox
examples. This harness exercises recipe output; it does not replace full Portage
installation and account ownership checks. Manifests record the BLAKE2B/SHA512
hashes of actual published sources and dependency archives.

[Website CI](https://github.com/airencracken/comfyware_org/actions/runs/37862094888)
passes content, HTTP, deployment, Caddy and 322 Chromium browser checks.
The Songstead screenshot uses the actual 0.2.1 executable with fictional music
and disposable accounts. Funding links match the companion applications.

Publication updates repositories and release artifacts. Upgrading services and
deploying the website document root remain host operations.

Member profile checks cover optional fields, member-only routes, identity isolation,
Unicode boundaries, unsafe links, duplicate fields, atomic rollback, upgrade defaults,
account exports, disabled accounts and linked authors. Browser checks exercise profile
editing with and without JavaScript, responsive layouts and accessibility.

## Search

Search regression tests cover every audience, active and suspended viewers,
Unicode case mapping, literal SQL/wildcard input, query bounds, read-only export
stability, and stable cursors after new music/comments arrive. Comment search is
compared against the detail page's spoiler projection over all three preferences
and listening positions 0–65. Hidden comments cannot crowd out pages. Browser
checks exercise boosted and native forms, thumbnails, mobile layouts and
accessibility. Mutation tests remove authentication, spoiler rules, viewer checks,
Unicode mapping and cursor boundaries, and attempt to search private reaction notes.
