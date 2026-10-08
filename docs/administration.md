# Administration and invitations

Provision a new owner with `songstead create-owner --username alex --password-prompt`.
For an existing account, run `songstead set-role --username alex --role owner`.
Restart the updated server before account commands when upgrading an older
schema. These commands discover the installed service's data directory and
run as its configured service account when invoked as root.

Sign in and choose **Admin**. **Instance settings**, **Accounts**, and
**Invitations** are linked together. Owners configure and manage the instance;
owner status does not bypass recommendation or group privacy. Personal listening
notes remain private to their author.

## Connect Witmoot

In Instance settings, set **Public Songstead address** to your HTTPS origin,
such as `https://music.example.org`, and **Witmoot address** to your board,
such as `https://boards.example.org`. Witmoot may have a path prefix.
Save settings; no restart is needed. A blank Witmoot address disables handoffs.
Restore default settings to use the service's environment defaults again.

A recommendation offers **Prepare a Witmoot discussion**, followed by a review
page and Witmoot's draft chooser. Sign in to Witmoot separately, choose a board
or topic, and review before posting. This transfers the music title, artist,
provider URL and Songstead source URL. It never transfers friend notes, group
provenance or private listening notes. A private source still requires its
original Songstead permissions. Neither app requires the connection to operate.

## Invite people

Joining starts in **Invitation only** mode. Owners can choose **Closed** to
pause joining or **Anyone can create an account** to permit open registration.
Music pages require an account in every mode. Closed joining keeps existing
accounts available and preserves invitations until their own expiry or revocation.

Create an invitation from Invitations. The default is one use within seven days.
A label helps identify the intended recipient. Limits allow up to 10,000 uses and
3,650 days; zero means unlimited. Copy the private link immediately and send it
to the intended person. Its raw secret is shown only on creation; the database
stores its hash. Owners see everyone's invitations; delegated members see only
their own. Lists paginate. Revoke an invitation to stop future use.

The recipient opens the link, reads the house rules, chooses a username and
confirms a password. Viewing the link does not consume it. Successful joining
creates a member and consumes one use atomically. Failed validation or a taken
username leaves the invitation usable. The account records who invited it.

## Manage accounts and recovery

Accounts shows roles, suspension, invitation permission and inviter attribution.
Owners always have invitation permission. Delegated members can issue and revoke
their own invitations. Removing permission permanently revokes outstanding
invitations; granting permission again does not revive them.

Suspension immediately ends sessions and revokes invitations and recovery links.
Resuming an account permits a fresh sign-in, without reviving old links. At least
one active owner must remain; concurrent demotions or suspensions cannot remove
the last one. Local role changes have the same protection.

Create a password recovery link for an active account in Accounts. Copy and
send it privately. It expires in one hour, can be used once, and is shown only
at creation. A new link replaces the previous one; cancel it when necessary.
Opening a link does not consume it. Successful reset changes the password,
revokes all sessions and consumes the link in one transaction. A local password
change or suspension also invalidates outstanding recovery links. Members can
change their own passwords from their account page using their current password.

## Identity and shared code

Instance settings includes the site name, welcome message, house rules,
public owner contact text, source code link and optional running version.
Upload a mascot or favicon, or restore the defaults. PNG, JPEG and GIF uploads
are limited to 2 MiB and 2048 by 2048 pixels. Re-encoding to PNG removes metadata
and animation while retaining transparency. Invalid images leave both assets
unchanged.

Songstead follows Witmoot and Imvault's owner settings, invitation creation and
recovery patterns. Comfylib v0.1.2 supplies shared password confirmation and image
normalization, alongside existing tokens, session CSRF, trusted proxy resolution,
service configuration and privilege dropping. Application roles, SQLite schemas,
joining policy and music access rules remain in Songstead.
