# Project: Comfyware Music Recommendations

You are building a small, self-hosted, collaborative music recommendation application as part of the Comfyware family of software.

## Philosophy

Build a comfortable, durable place for friends to share music and discover things recommended by people they trust.

The problem is simple: friends send YouTube, Spotify, Bandcamp, and other music links through chat. Everyone is busy, recommendations get buried, and nobody remembers what they've listened to or what they thought.

This application is an **asynchronous recommendation inbox**, not a streaming service, collaborative playback room, or algorithmic social network.

Prioritize simplicity, correctness, low operational overhead, and long-term maintainability.

Follow the conventions of existing Comfyware projects where applicable.

## Technology

- Go backend, preferably a single statically linked executable.
- SQLite for persistence.
- Server-rendered HTML with HTMX for progressive enhancement.
- Minimal CSS.
- No required frontend JavaScript framework.
- No Redis, message broker, external search engine, or unnecessary infrastructure.
- Support straightforward deployment behind a reverse proxy.
- Reuse existing Comfylib functionality where appropriate, particularly authentication, invitations, sessions, and shared utilities.
- Inspect existing project conventions before introducing dependencies or architectural patterns.

Do not introduce complexity merely to accommodate hypothetical future scale.

## Core Domain

### Media

A media item represents something recommended.

Initially support:

- Individual songs or performances.
- Full albums.
- Music videos.
- Arbitrary URLs as a fallback.

Media items retain:

- Original URL.
- Provider (YouTube, Spotify, Bandcamp, SoundCloud, Apple Music, Pandora, or unknown).
- Title.
- Artist or creator, when available.
- Album, when available.
- Artwork or thumbnail, when available.
- Duration, when available.
- Media type.
- Optional metadata.

Metadata extraction must be best-effort. Never reject a valid URL simply because its provider is unsupported or metadata retrieval fails.

Preserve the original URL.

Do not attempt to build a universal music player or bypass provider playback restrictions. Embed supported media where practical; otherwise link to the provider.

### Recommendations

A recommendation is a first-class object, distinct from the media item.

It consists of:

- Media item.
- Sender.
- Destination.
- Optional personal note.
- Creation timestamp.

A destination can be either an individual user or a group.

The same media may be recommended multiple times by different people, to different audiences, with different notes.

Do not automatically merge distinct recommendations.

### Reactions

Reactions belong to individual users and individual recommendations.

Support:

- Unheard.
- Listened.
- Revisit later.
- Dismissed.
- Like.
- Dislike.
- No rating.
- Optional personal notes.

Listening status and opinion are separate dimensions. Someone can listen to something without deciding whether they like it.

Users must be able to revise their reactions.

### Tags

Support freeform tags for organizing and rediscovering recommendations.

Tags may describe genres, moods, instrumentation, or anything else users find useful.

Start with user-owned tags. Avoid complex global taxonomies.

### Comments

Allow conversation attached to recommendations.

Keep comments simple and chronological.

Do not implement nested discussion trees, karma, or elaborate voting systems.

## Users and Groups

Users can send recommendations directly to another user.

Users can also create groups.

Groups support three access modes:

1. Private: invitation only, not publicly discoverable.
2. Restricted: publicly discoverable, membership requires approval or invitation.
3. Open: publicly discoverable and joinable.

Group recommendations are visible according to group access rules.

Each group member maintains their own listening status and reactions. Group membership does not create a single shared listening queue.

Public group content should eventually support anonymous read-only access and RSS/Atom feeds.

Design permissions explicitly and enforce them server-side.

## User Experience

### Personal Inbox

Show recommendations received directly and through groups.

Prioritize unheard recommendations.

Allow filtering by:

- Listening status.
- Sender.
- Group.
- Provider.
- Media type.
- Tags.

Users should be able to open a recommendation, listen using an embed or external provider, react, tag, and comment.

### Submitting Recommendations

The primary interaction should be extremely fast:

1. Paste URL.
2. Select recipient or group.
3. Optionally add a note.
4. Submit.

The system should retrieve metadata automatically when possible.

Never make submission substantially more cumbersome than pasting a link into chat.

### Group Feed

Display recommendations chronologically.

Show sender, media information, notes, comments, and aggregate reactions where appropriate.

Do not automatically mark recommendations as listened merely because another member listened.

### Personal Library

Allow users to browse their recommendation history, including items they've listened to, liked, disliked, or saved for later.

Make this searchable and filterable.

## Non-Goals

Do not implement these in the initial version:

- Streaming or transcoding.
- Hosting copyrighted music.
- Synchronized listening rooms.
- AI-generated recommendations.
- Algorithmic engagement feeds.
- Public follower counts or karma.
- Music fingerprinting.
- Automatic cross-provider track matching.
- Mobile applications.
- Push notifications.
- Federation.
- Monetization.
- Elaborate moderation infrastructure.

The application should be useful without any of these.

## Architecture

Keep the data model small, but preserve important domain distinctions:

- Users.
- Groups.
- Group memberships and invitations.
- Media.
- Recommendations.
- Recommendation destinations.
- User reactions and listening state.
- Tags.
- Comments.

Use SQLite constraints, foreign keys, transactions, and migrations appropriately.

Be careful about access-control boundaries, particularly private recommendations and groups.

Do not use a generic activity-feed architecture when ordinary relational tables suffice.

Avoid premature abstraction.

## Implementation Phases

### Phase 1: Two Friends

Build the smallest genuinely useful application:

- Authentication.
- Two users.
- Direct recommendations.
- URL submission.
- Basic YouTube metadata.
- Inbox.
- Listening status.
- Like/dislike.
- Comments.
- Basic history.

This should be deployable and usable.

### Phase 2: Groups

Add:

- Group creation.
- Invitations and membership.
- Group recommendations.
- Group feeds.
- Individual reactions.
- Access controls.

### Phase 3: Music Discovery

Add:

- Additional provider metadata.
- Album support.
- Tags.
- Search and filtering.
- Personal library improvements.
- Public groups.
- RSS/Atom.

### Phase 4: Polish

Explore:

- Ordered mixtapes with liner notes.
- Recommendation forwarding.
- Importing links from chat exports.
- Cross-provider media relationships.
- Discovery based on trusted friends' reactions.

These are possibilities, not commitments.

## Engineering Requirements

- Write tests for domain behavior, permissions, and database operations.
- Favor straightforward HTTP handlers and explicit SQL.
- Ensure that the application remains usable without client-side JavaScript.
- Handle malformed URLs, unavailable providers, deleted media, and failed metadata extraction gracefully.
- Never make external metadata retrieval a prerequisite for storing a recommendation.
- Protect against SSRF when fetching external URLs, including redirects and DNS resolution.
- Escape untrusted metadata and user-generated content.
- Make schema migrations repeatable and safe.
- Keep deployment and configuration simple.
- Document how to build, run, configure, back up, and restore the application.

## Working Instructions

First inspect the existing Comfyware repositories and identify reusable patterns and libraries.

Then:

1. Propose a minimal architecture and database schema.
2. Identify the smallest coherent Phase 1 implementation.
3. Explain any important tradeoffs or ambiguities.
4. Implement Phase 1 incrementally.
5. Run tests and resolve failures.
6. Document the result.

Do not spend excessive time planning speculative future features.

Do not implement later phases merely because they are described here.

Prefer working software over elaborate scaffolding.

**The success criterion is simple: two friends stop losing music recommendations in chat because this application is easier and more useful.**
