# The first shape

One Go binary, one private data directory, and SQLite. `cmd/songstead` wires
configuration, account commands, shutdown, and the metadata worker.
`internal/store` owns explicit SQL and embedded migrations. `internal/web` owns
HTTP routes, CSRF checks, escaped templates, and bundled assets. `internal/media`
validates links and retrieves optional metadata.

The first coherent workflow is two friends sharing a URL, finding it in an
inbox later, recording a reaction, and leaving a comment. Groups and invitations
are deliberately deferred to Phase 2. Users are local accounts on one instance;
two is the starting use case, not a hard-coded account limit.

## Relational model

| Table | Meaning |
| --- | --- |
| `users` | Case-insensitive unique username and bcrypt password hash |
| `sessions` | Digest of a random session token, account, expiry |
| `media` | Original URL, provider, best-effort metadata, type, verified video ID |
| `recommendations` | Media reference, sender, personal message, timestamp |
| `recommendation_destinations` | One recipient for each Phase 1 recommendation |
| `reactions` | Listening state, rating, private note per user and recommendation |
| `comments` | Author and text in chronological order |
| `metadata_jobs` | Durable pending lookup, attempt count, next attempt |

Media and recommendation records are separate even though each submission
currently creates its own media record. No URL uniqueness constraint merges
repeated recommendations. This keeps notes and audiences distinct without
introducing a provider identity or deduplication scheme.

Destinations have a separate table so a future group destination can be added
with a migration and an explicit exactly-one-destination constraint. No empty
group scaffolding is included now. Reactions use a compound primary key; their
listening enum and rating are separate checked columns. Missing reaction rows
mean unheard and no rating. Opening a page never changes either.

SQLite enforces foreign keys and domain checks. A recommendation, destination,
media record, and optional metadata job commit in one transaction. WAL and a
single connection keep writes simple; transactions take an immediate write lock.
A server lock allows one metadata worker per instance. Schema migration is
transactional and repeatable; older binaries refuse a newer schema.

## Permissions and authentication

Only the sender and recipient may view or comment on a direct recommendation.
The same visibility predicate governs detail queries and write statements.
Lists and exports apply the corresponding audience checks. Unrelated viewers
receive the same 404 as for a missing recommendation.

Every participant's listening state, rating, and personal note are their own.
The current interface shows only the viewer's reaction; comments provide the
shared conversation. The operator can inspect the underlying database, as with
the other self-hosted applications, but has no web bypass for private messages.

Passwords use bcrypt. Sessions store digests, expire in seven days, and rotate
on login. Password changes revoke sessions atomically. New sessions also verify
that the password hash checked during login is still current, closing a race
with account recovery. Failed logins use a dummy bcrypt hash and bounded
per-client-network rate limiting.

Every mutating form requires CSRF protection, including login and logout. The
signed-in token is derived from the session using Comfylib's shared helper and
Songstead's own purpose. Signed-out forms use a random double-submit token.
Secure cookies use the `__Host-` namespace, `HttpOnly`, and `SameSite=Lax`.
Forwarding headers are trusted only through Comfylib's configured proxy resolver.
Go's cross-origin protection adds a second check. Bodies and headers are bounded;
duplicate form fields and malformed form encoding are rejected.

## Metadata and playback

Submission performs URL validation and a local transaction. It makes no external
request. The original URL remains the fallback title until metadata succeeds.
A durable worker retrieves YouTube oEmbed data from a fixed HTTPS endpoint,
with a five-second deadline and a 64 KiB response limit. It retries three times
with delays and preserves pending work across restart.

The submitted hostname is never a fetch destination. Only a recognized YouTube
URL with an eleven-character video ID is used to construct a canonical request.
DNS answers must all be public addresses. The connection uses the validated IP
without a second DNS lookup, while TLS verifies the provider's hostname. HTTP
redirects and environment proxy settings are disabled. Returned provider HTML
and thumbnail URLs are ignored; artwork and embeds are constructed from the
validated ID. Text is rendered through Go's escaping templates.

Other providers are recognized labels and ordinary links in this phase. Unknown
HTTP(S) URLs, including custom ports, are accepted and never fetched. A provider
being unavailable cannot prevent storing a recommendation. Embeds and artwork
are optional third-party browser requests; ordinary provider links remain usable.

## Shared Comfylib work

Session-bound CSRF derivation was duplicated in Witmoot and Imvault. It now lives
in `comfylib/token.SessionCSRF`. Their original application purposes are retained,
and independent test vectors verify that existing sessions continue to work.
Songstead also reuses token generation, digest comparison, and trusted-proxy
handling. Account storage, invitations, and audience policy remain app-specific.

## A possible Witmoot connection

Ordinary links are the first connection between the apps. Later, an explicit
“Share to Witmoot” action could post a recommendation or mixtape preview into a
chosen topic, and “Recommend in Songstead” could turn a music link in a topic into
a new recommendation. Each app remains useful independently.

A private recommendation must not become a public preview automatically. Sharing
its title, artwork, or note would be a deliberate publication into that topic's
audience. Witmoot would govern the shared copy; opening the Songstead link would
still require Songstead access. Listening state stays individual. Account linking,
credential storage, and audience changes need explicit design before an API bridge.
These are later possibilities, not Phase 1 features.
