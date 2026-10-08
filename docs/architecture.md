# Songstead architecture

Songstead follows Witmoot's Go / SQLite / embedded server-rendered HTML and HTMX
structure, with Imvault's quieter blue visual direction. A statically built
binary serves its own assets. Accounts are provisioned locally; sessions are
hashed in SQLite, and session-bound CSRF derivation comes from Comfylib.

The application has three layers: URL identity and provider metadata in
`internal/media`, transactions and access rules in `internal/store`, and
browser handlers/templates in `internal/web`. The embedded migration sequence
uses SQLite user_version and commits upgrades atomically, rejecting newer schemas.

Recommendations preserve source URLs and sender notes independently of music
identity. Group access follows current membership; direct access follows the
sender/recipient pair. The same visibility predicate protects reads and mutations.
Music organization belongs to a viewer, while per-recommendation ratings and
personal notes remain separate. Neither is publicized.

Recordings are explicit identities linked to tracks or named album tracks.
Annotation offsets belong to comments and recordings, with byte spans into the
original plain text. The timestamp parser recognizes complete bounded tokens.
Spoiler preferences filter comments before rendering, with manually entered
positions and no telemetry. Detailed rules are in [quiet-inbox.md](quiet-inbox.md).

Metadata is best effort. Only recognized YouTube IDs invoke a bounded fixed-host
oEmbed lookup. DNS answers are checked and pinned, redirects and proxies are
disabled, and returned HTML is ignored. Durable retry jobs never prevent saving
a recommendation. Other URLs remain ordinary links, without server fetches.

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

No ranked feed, completion metrics, playback tracking, deadlines or reminders.
Extremely eventual consistency: because your friends have lives.
