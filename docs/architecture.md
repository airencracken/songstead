# Songstead architecture

Songstead follows Witmoot's Go / SQLite / embedded server-rendered HTML and HTMX
structure, with Imvault's quieter blue visual direction. A statically built
binary serves its own assets. Owners can provision accounts locally or issue
expiring, use-limited invitations. Sessions, invitations and recovery links are
hashed in SQLite, and session-bound CSRF derivation comes from Comfylib.

The application has three layers: URL identity and provider metadata in
`internal/media`, transactions and access rules in `internal/store`, and
browser handlers/templates in `internal/web`. The embedded migration sequence
uses SQLite user_version and commits upgrades atomically, rejecting newer schemas.

Recommendations preserve source URLs and sender notes independently of music
identity. Group access follows current membership; direct access follows the
sender/recipient pair. Explicit instance sharing is visible to valid signed-in
accounts. The same visibility predicate protects reads and mutations. Recent
adds a shared-only predicate before sorting and pagination; it never reads
private provenance for display. A schema constraint prevents a group context
from being combined with instance sharing. Shared posts use one destination row
without manufacturing individual gifts.
Music organization belongs to a viewer, while per-recommendation ratings and
personal notes remain separate. Neither is publicized.

Recordings are explicit identities linked to tracks or named album tracks.
Annotation offsets belong to comments and recordings, with byte spans into the
original plain text. The timestamp parser recognizes complete bounded tokens.
Spoiler preferences filter comments before rendering, with manually entered
positions and no telemetry. Detailed rules are in [quiet-inbox.md](quiet-inbox.md).

Metadata is best effort. Recognized YouTube, Spotify and SoundCloud links invoke
bounded, canonical oEmbed lookups. DNS answers are checked and pinned, redirects
and proxies are disabled, and returned HTML is ignored. Allowlisted provider
artwork is normalized with Comfylib and cached as a local PNG in SQLite.
Authenticated thumbnail routes use the same recommendation visibility predicate.
Durable retry jobs never prevent saving a recommendation; an image failure
retains the metadata text. Other URLs remain ordinary links without fetches.

Comfylib's small `reference` package validates web addresses and constructs draft
handoff links. Songstead and imvault explicitly prepare Witmoot drafts; Witmoot
requires its own sign-in, audience choice and post action. No databases, identities,
transactions or required runtime services are shared. Source permissions remain
with the source app. Imvault album previews use the existing per-user encrypted
connection and a currently public, authenticated ownership-checked summary API.

Exports take a consistent transaction containing visible recommendation context,
only the caller's private state, authored comments and structured offsets,
annotation preferences, manual positions, recordings and discussion links.
Backups use SQLite VACUUM INTO; restore validates schema, integrity and foreign
keys before installing a snapshot into an empty private directory.

Genres and tags describe individual recommendations, preserving private-send
boundaries across duplicate provider identities. Only the sender edits them.
Per-user discovery preferences filter Recent before pagination and sort matching
favorites ahead of other music. Exclusions win; each tier retains newest-first
order. SQL parameters bind normalized names as JSON arrays. Shelf, history and
access permissions stay independent of discovery preferences.

No engagement scoring, completion metrics, playback tracking, deadlines or reminders.
Extremely eventual consistency: because your friends have lives.

Administration settings are validated and stored as an instance override. Each
request reads effective settings, so saves apply immediately without mutating
shared application configuration. Owner checks, invitation consumption, account
changes and last-active-owner protection run inside SQLite write transactions.
Password resets bind to current credentials and change passwords, revoke
sessions and consume the link atomically. Suspension invalidates sessions and
bearer links. Administration does not extend private music access.

Comfylib supplies password confirmation, a terminal echo guard and bounded image
normalization. Apps supply their terminal reader, prompt labels and password
policy, and retain storage and authorization.
Branding uploads normalize both assets before an atomic database update.
