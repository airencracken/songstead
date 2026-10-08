# Music left for your people

Recommendations are gifts. There are no deadlines, streaks, completion counts,
read receipts, automatic listening changes or reminders. The shelf uses arrival
order and defaults to a music view. Person, group and individual recommendation
views are filters over the same recommendations, not separate copies.

A group creator chooses members and can change that membership. Current members
can recommend music to the group and see its recommendations and comments;
removal revokes that access. Joining grants access to the group's earlier music.
Direct recommendations remain visible only to their sender and recipient.

Recent is the instance's shared shelf, newest first. The audience selector offers
everyone here, one friend privately, or a group privately. Everyone here means
accounts signed in on this installation; there is no anonymous feed. A shared
recommendation's music, note and conversation are visible to those accounts.
Private friend and group recommendations never appear in Recent, even for their
participants, and a shared duplicate never reveals a private recommendation's
note or conversation. Existing recommendations keep their audience on upgrade.
Older forms that omit the new audience field still send privately.

Shared posts do not fan out into personal shelves. Your history includes your direct and
group gifts, shared posts you wrote, and shared music you chose to organize or
comment on. Merely opening a shared recommendation records no participation.
Recent defaults to each recommendation; its optional music view groups matching
shared music while keeping every sender's words. There are no badges, unread
counts, automatic refresh, reminders or ranking.

The personal view is Your shelf at `/shelf`. Existing `/inbox` links and filters
remain supported, with the same audience rules, but new navigation uses Shelf.

Music identity is conservative: recognized YouTube video IDs and Spotify
track/album/artist IDs are shared across their provider URL variants. Other URLs
share identity only when their normalized URLs match; fragments are ignored.
Songstead does not guess that recordings on different providers are identical.
Each recommendation retains its original URL, sender, note and group context.
The default music view paginates music identities and retains all matching
recommendation contexts for each displayed identity.

Saved, explored, listened and dismissed are optional, private organization.
Unheard and revisit remain accepted for existing clients. Organization applies
to a person's music identity; ratings and personal notes remain attached to the
individual recommendation. Nobody else's state appears in a recipient's view.

# Timestamp annotations

Comments recognize complete `m:ss` and `h:mm:ss` tokens, with seconds and the
middle minutes in the range 00–59. Multiple references are stored individually
as recording IDs, second offsets and byte spans. Invalid tokens stay prose;
positions beyond a known duration do not become markers. Unknown durations allow
references up to 24 hours. Clock times may be ambiguous: timestamps belong to
the recording selected for the comment, rather than being inferred from prose.

A recognized track has an automatic recording. Identify an album track by its
URL and title, optionally entering its duration, then select that recording
when commenting. Album or artist comments with valid timestamp tokens and no
recording are rejected rather than linked to an arbitrary track. One comment's
references belong to its selected recording; use separate comments when
referring to several album tracks. Changes cannot shorten a duration below an
already stored reference. A known duration yields a proportional interactive
SVG timeline; offset links also work without JavaScript or a known duration.

Annotation preference is per account: immediate, spoiler-free or hidden.
Spoiler-free comments appear only after your manually indicated position reaches
all their references. Hidden comments require an explicit reveal for that view.
Concealed comment bodies are excluded from the HTML, not hidden with CSS.
Unmarked prose remains visible in every mode. References are not inferred as
spoilers for an album when the recording was never identified.

Playback links and the optional YouTube iframe have no position reporting.
Setting a position never marks music listened or reports your position to
another account. There is no streaming account connection or playback telemetry.

# Explicit Witmoot discussions

Set `SONGSTEAD_BASE_URL=https://music.example.org` and optionally
`SONGSTEAD_WITMOOT_URL=https://board.example.org`. Base URLs may include an app
path prefix but must not contain credentials, queries or fragments. Restart
after changing service configuration.

Prepare a discussion from a recommendation. A local review page leads to
Witmoot's `/share` draft chooser, where you select a board or existing topic.
The handoff carries the music title, artist, provider link and source link;
friend notes, group provenance and private reactions are not copied. Witmoot
requires sign-in and its normal CSRF-protected posting action. Sign in to
Witmoot first, or reopen the source handoff after signing in.

You can also paste a recommendation URL into any existing Witmoot message.
Witmoot displays a small source reference, and plain links work even in older
Witmoot versions. The Songstead source still requires its own sign-in and
recommendation permission. Songstead never exposes an unauthenticated metadata
endpoint and never automatically creates a discussion.

Keep an existing discussion URL from the recommendation page. Links are private
to the person saving them, shared across that person's matching music. They
are not fetched or verified; an unavailable discussion leaves the music usable.
To remove a link, choose Remove beside it. An unavailable Witmoot instance has
no effect on Songstead startup, comments, listening links or organization.
