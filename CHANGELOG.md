# Changelog

## 0.5.0

- Add an owner setting for Songstead, Witmoot or Both discussion locations.
- Keep local participation available without a Witmoot account in Songstead
  and Both; explain separate accounts and invitations before Witmoot posting.
- Enforce local commenting policy transactionally, disable unused handoff routes,
  and keep existing comments, annotation controls and personal bookmarks readable.
- Preserve older configured connections as Both and local-only instances as
  Songstead. Validate modes and connection addresses without partial settings writes.
- Retain schema 7, private feedback, Comfylib 0.1.4 and browser draft handoffs;
  no API key, account creation or automatic cross-app posting is introduced.

## 0.4.1

- Show compact cached thumbnails and artwork fallbacks in List, including Shelf
  and History. Refresh pending artwork in both List and Tiles.
- Separate view/discovery switches, genre/tag pickers and advanced filters into
  clear rows. Apply secondary filters together without losing current choices.
- Rename Account to Your settings and add shortcuts to profile, animation,
  spoiler, discovery, password and data preferences.
- Keep schema 7 and the existing Comfylib 0.1.4 dependency.

## 0.4.0

- Preview titles, artists and thumbnails when pasting supported music links.
- Refresh pending artwork locally, retry failed previews, and fetch YouTube
  thumbnails independently of its metadata endpoint.
- Switch Chips/Tiles immediately; use compact genre/tag pickers and tuck secondary
  dropdowns into More filters. Preserve active filters across layout switches.
- Open external music links in a new tab. Fix YouTube error 153 by sending an
  origin-only referrer to the optional player.
- Upload PNG, JPEG or animated GIF profile pictures. Add an account setting to
  disable animation; browser reduced-motion preferences always choose stills.
- Use Comfylib 0.1.4 for bounded, re-encoded profile images.
- Add generic public sharing cards without exposing private recommendation data.
- Migrate to schema 7 and export format 4; retry existing missing artwork.


## 0.2.1

- Disable terminal echo before displaying either password prompt using Comfylib 0.1.3.
- Verify prompt-time terminal state and terminal restoration with real pseudo-terminal tests.

## 0.2.0

- Add owner administration for persistent identity, joining policy and Witmoot URLs.
- Add expiring, use-limited invitation links with delegated permission, attribution and revocation.
- Add account roles and suspension with transactional last-active-owner protection.
- Add single-use one-hour recovery links and current-password account changes, with session revocation.
- Add configurable welcome text, house rules, contact, source link, version display and normalized mascot/favicon uploads.
- Add explicit local `set-role` for upgrading an existing member to owner.
- Share password confirmation, image normalization and the mutation engine through Comfylib 0.1.2.
- Preserve schema 3/4 backups, existing accounts and sessions, and all music privacy boundaries.
- Verify administration with store, HTTP, schema, property, atomicity, adversarial, mutation and Chromium accessibility tests.

## 0.1.2

- Add transparent jukebox favicons matching the Imvault and Witmoot icon sizes.
- Add local owner provisioning, account role listings, and hidden, confirmed password prompts.
- Read installed OpenRC/systemd data directories for account and backup commands, and preserve service account ownership when invoked as root.
- Add command-specific flags and help. Reject duplicate owner accounts without changing their passwords, roles, or sessions.
- Preserve existing member accounts and schema 3 backups during the owner-role upgrade. Account commands require the updated server to migrate an existing database first.

## 0.1.1

- Add `songstead sandbox` and `sandbox --check` using Comfylib's Bubblewrap service policy.
- Add OpenRC opt-in configuration and a systemd drop-in. Sandbox failures stop startup.
- Keep only the private data directory writable on the host, strip unrelated inherited settings, and disable nested user namespaces.
- Require real sandbox lifecycle and filesystem boundary tests in CI and release validation.

## 0.1.0

First Songstead release: a signed-in Recent tab for music shared with everyone
here, an explicit audience selector, private recommendations from friends and groups,
views by music, person, group and recommendation, shared personal organization
for duplicate music, timestamped comments tied to recordings, annotation
preferences with manual listening positions, and explicit Witmoot draft handoffs.

Go, SQLite and HTMX; local accounts, static Linux binaries, backups, restore,
account exports, OpenRC/systemd configuration and a friendly jukebox mascot.
AGPL-3.0-or-later. No playback telemetry, deadlines, read receipts or reminders.

Uses published Comfylib v0.1.1 with verified module checksums.
