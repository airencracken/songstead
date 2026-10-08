# Changelog

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
