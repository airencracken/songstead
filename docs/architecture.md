# Songstead architecture

Songstead is one static Go binary with SQLite, server-rendered HTML and HTMX.
It follows Witmoot's structure and Imvault's quiet blue look. Sessions,
invitations and recovery links are stored as hashes. Comfylib supplies session
CSRF, password prompts and image normalization.

## Layers

- `internal/media`: URL identity and provider metadata.
- `internal/store`: transactions, migrations and access rules.
- `internal/web`: handlers and templates.

Migrations use SQLite `user_version`, run in one transaction, and refuse a
database from a newer binary.

## Access

Each recommendation keeps its source URL and sender note apart from the shared
music identity. Group access follows current membership. Direct access follows
the sender and recipient. Posts shared with everyone are visible to any active
signed-in account. One visibility check guards reads, writes and thumbnails.
Recent only reads shared posts, so private context never reaches it. A schema
constraint stops a post from being both group-only and shared.

Listening status and song likes belong to the viewer and the music identity.
Personal notes and older ratings stay with their recommendation. None of this
is shown to anyone else.

Genres and tags belong to one recommendation and follow its audience. Only the
sender edits them, along with title, artist and kind corrections. Edits and
deletes check the author inside the write transaction. A post with timestamped
comments keeps its music identity so the timestamps stay attached.

Discovery preferences filter Recent before pagination. Exclusions win over
favorites. They never change access, the shelf or history.

## Comments and timestamps

Recordings are tracks or named album tracks. Comment timestamps point at a
recording and store their position in the original text. Spoiler preferences
filter comments before rendering. Positions are entered by hand; Songstead
tracks no playback. See [how the inbox works](quiet-inbox.md).

## Metadata

YouTube, Spotify and SoundCloud links use bounded oEmbed lookups. DNS answers
are checked and pinned, redirects and proxies are refused, and provider HTML is
ignored. Artwork from allowed hosts is normalized and cached as PNG in SQLite.
Lookups retry in the background and never block saving. Other links are never
fetched.

## Other apps

Comfylib's `reference` package builds Witmoot draft links. Witmoot asks for its
own sign-in, audience and post action. The apps share no database, accounts or
runtime services.

## Exports and backups

An export reads one transaction: recommendations you can see, your own private
state, comments you wrote and your discussion links. Backups use SQLite
`VACUUM INTO`. Restore checks schema, integrity and foreign keys before
installing into an empty private directory.

## Administration

Instance settings are stored as an override and read on each request, so saves
apply at once. Owner checks, invitation use, account changes and the last-owner
rule run inside write transactions. A password reset changes the password,
revokes sessions and uses up the link together. Branding uploads normalize both
images before one database update.

There is no engagement scoring, completion tracking, deadlines or reminders.
Instance metrics are aggregate operational totals only.
