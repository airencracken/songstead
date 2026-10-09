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

Choose **Your settings → Spoiler preferences** to set the account-wide mode:
immediate, spoiler-free or hidden. The same selector on a recommendation updates
this global preference, and both forms confirm a successful save.
Spoiler-free comments appear only after your manually indicated position reaches
all their references. Hidden comments require an explicit reveal for that view.
Concealed comment bodies are excluded from the HTML, not hidden with CSS.
Unmarked prose remains visible in every mode. References are not inferred as
spoilers for an album when the recording was never identified.

Playback links and the optional YouTube iframe have no position reporting.
Setting a position never marks music listened or reports your position to
another account. There is no streaming account connection or playback telemetry.

# Explicit Witmoot discussions

Owners set the public Songstead origin (for example `https://music.example.org`)
and optional Witmoot address in **Admin → Instance settings**. Changes apply
immediately. Witmoot may use a path prefix; the public Songstead address is an
origin without a path. URLs cannot contain credentials, queries or fragments.
`SONGSTEAD_BASE_URL` and `SONGSTEAD_WITMOOT_URL` remain service defaults; restart
after changing those defaults. Saved settings take precedence, including a blank
Witmoot address that disables handoffs.

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

# Labels, artwork and discovery

A sender can add a freeform genre (80 characters) and up to 20 comma-separated
tags (40 characters each), during sharing or from **Genre & tags** on their
recommendation. Names are single-line, trimmed and deduplicated without case
sensitivity. Descriptions follow each recommendation's audience and only its
sender can edit them. Labels on a private send stay within that send, including
when the same provider recording appears in another recommendation.

Recent defaults to **List** with compact covers. Choose **Tiles** for larger artwork cards;
filters, audiences, grouping and pagination work in either layout. Shelf and
History also offer both layouts. Genre and tag filters match exact names without
case sensitivity. Pagination preserves the current layout and filters.

In **Your settings → Discovery preferences**, list genres and tags to exclude or
prefer. These preferences are private, persisted per account and included in
your export. They apply to Recent: an exclusion hides a matching recommendation
from that view, even when another label is a favorite. Remaining favorites come
first, then other music; each tier uses newest-first order. Unlabeled music stays
visible. A preference neither changes access nor removes music from your shelf
or history. Choose **All music** to temporarily bypass exclusions
and favorites; clear preference fields and save to remove them permanently.

YouTube, Spotify track/album/artist links and SoundCloud track/set links use
public oEmbed metadata without account credentials. Provider HTML is ignored.
The worker only contacts fixed provider/CDN hosts, validates DNS answers and
refuses redirects and private destinations. Small PNG/JPEG/GIF images are
normalized by Comfylib and cached as PNG inside the database. Thumbnail routes
require the recommendation's current access permissions. No provider image
request comes from the listener's browser. The optional YouTube player remains
a separate, explicit provider embed.

Missing artwork has a jukebox fallback. Text survives artwork failures, which
retry at most three times. Unsupported links are saved without fetching them.
Upgrading to schema 6 queues existing supported links for artwork; manual titles
and artists stay intact. Back up before upgrading; an older binary cannot use
the new schema.

Listening saves confirm beside the form and keep invalid drafts for correction.
Export format 3 adds accessible labels and the exporting user's own discovery
preferences. It never includes another user's preferences or private feedback.

## Browsing controls and your settings

Choose List or Tiles to change the layout immediately; both include locally cached
artwork. For you uses your private exclusions and favorites, while All music
browses everything shared here in newest-first order. Genre and tag buttons
filter immediately. Open More filters to choose music kind, person, group or
listening status, then Apply filters. These controls preserve applied choices.

Open **Your settings** in the top navigation, or visit `/account`. Shortcuts on
that page lead to your profile picture and animation preference, spoilers,
genre/tag discovery preferences, password and history export. Instance owners
configure their service separately through Admin.

## Link previews and profile pictures

The authenticated compose form previews YouTube, Spotify and SoundCloud links
after a short pause. Provider responses supply text and locally normalized PNG
bytes; provider HTML is never rendered. A failed or unsupported preview leaves
sharing available. Manual title and artist edits take precedence. After saving,
artwork refreshes without reloading the listening or comment forms. A retry
button requeues an accessible supported link. Preview requests and retries are
limited per account; fetch destinations retain their fixed provider allowlist.

Schema 7 stores account pictures and a viewer animation preference. Upgrading
also requeues supported links with missing artwork, leaving cached artwork alone.
PNG, JPEG and GIF uploads are limited to 2 MiB and 512 by 512 pixels; animated GIFs
are limited to 64 frames. Comfylib re-encodes both GIF animation and a PNG still.
Pictures are visible only to signed-in members. Disabling animation makes the
authenticated picture route serve PNG; reduced-motion browsers request PNG too.
Account export format 4 includes only the exporting user's picture and preference.

Public Open Graph metadata describes the instance, not a particular song, note or
person. Signal and Slack preview the login page after following an unauthenticated
recommendation redirect. Set the public base URL in Admin for stable absolute
artwork URLs; otherwise the card uses the request origin. A song-specific public
card would require a separate explicit public-sharing feature.

The optional YouTube player uses `strict-origin-when-cross-origin` on the iframe
so YouTube receives the instance origin, but never the recommendation path.
Ordinary external music links still suppress their referrer and open a new tab.
