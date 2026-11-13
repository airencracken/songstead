# Administration and invitations

This guide is for owners. Create the first owner with
[setup](setup.md). To promote an existing account, run:

```sh
songstead set-role --username alex --role owner
```

Sign in and open **Account → Admin**. **Metrics**, **Instance settings**,
**Accounts** and **Invitations** link to each other. Being an owner gives no
extra access to private recommendations, groups or listening notes.

## Choose where people discuss music

In **Instance settings → Discussion location**, choose:

- **Songstead**: members comment here. The Witmoot draft action is off.
- **Witmoot**: new comments go to Witmoot. Existing comments stay readable,
  with their spoiler controls and timestamps.
- **Both**: members choose. Use this when some people only have Songstead
  accounts.

Witmoot and Both need both app addresses. Each person needs their own Witmoot
account; a Songstead account does not grant one. No API key is needed, and
Songstead never posts for anyone. People review the Witmoot destination and its
audience before posting. Listening status, ratings and notes stay private in
every mode.

Changes apply immediately. New instances start with Songstead. Instances
upgraded with a working Witmoot connection keep Both until an owner changes it.
Changing an address does not change a saved discussion mode.

## Connect Witmoot

Set **Public Songstead address** to your HTTPS origin, such as
`https://music.example.org`, and **Witmoot address** to your board, such as
`https://boards.example.org`. The Witmoot address may include a path. Choose
Songstead before clearing the Witmoot address. **Restore default settings**
returns to the service's environment defaults.

**Prepare a Witmoot discussion** on a recommendation opens a review page, then
Witmoot's draft chooser. The draft carries the music title, artist, provider
link and a link back to Songstead. It never carries friends' notes, group names
or private listening notes. A private recommendation still needs Songstead
access to open.

## Invite people

Joining starts as **Invitation only**. Choose **Closed** to pause joining or
**Anyone can create an account** for open sign-up. Music pages need an account
in every mode. Closing keeps existing accounts and invitations.

Create an invitation under **Invitations**. The default is one use within seven
days. Limits go up to 10,000 uses and 3,650 days; zero means no limit. Copy the
link right away: Songstead stores only its hash and cannot show it again. Owners
see every invitation; members with invitation permission see their own. Revoke
an invitation to stop further use.

The person opening the link reads the house rules, picks a username and sets a
password. Opening the link does not use it up. A failed attempt leaves it
usable. Each account records who invited it.

## Manage accounts and recovery

**Accounts** shows roles, suspension, invitation permission and who invited
each person. Owners can always invite. Removing a member's invitation permission
revokes their open invitations for good.

Suspending an account signs it out and revokes its invitations and recovery
links. Resuming lets the person sign in again but does not revive old links. At
least one active owner must remain.

**Create password recovery link** makes a one-hour, single-use link. Send it
privately. A new link replaces the old one. Using it changes the password and
signs out every session. Members can change their own password in Your settings.

## Instance metrics

**Admin → Metrics** shows operational health, the kind of numbers a server log
gives: member, recommendation, music link and comment totals, database size,
requests, error rate and response times per route, data served, failed
sign-ins, refused attempts and rejected invitation or recovery links, plus
backup status and configured connections. Songstead does not record who signs in
or which recommendations people open, and keeps no per-member or per-item
rankings. Counts are kept for 90 days. Schema 12 deletes the sign-in and view
records kept by 0.15 and 0.16.

## Identity and images

Instance settings holds the site name, welcome message, house rules, owner
contact, source code link and whether the footer shows the running version.
**Site colors** sets the palette everyone sees first: Sage (the default) or
Slate. Members can pick System, Light or Dark and their own palette from the
Account menu; that choice stays in their browser.
Upload a mascot or favicon as PNG, JPEG or GIF, up to 2.1 MB and 2048 by 2048
pixels. Songstead saves it as a still PNG. If either image is invalid, neither
changes.

## Member preferences and profiles

Members set spoiler and discovery preferences, a profile picture, a short
profile and their password in **Your settings**. Profiles show an optional name,
plain-text bio and up to five links to signed-in members. They never show
private listening, email or credentials. See [how the inbox works](quiet-inbox.md).
