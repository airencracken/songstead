# Optional Comfyware connections

Each app keeps its own accounts, permissions, sessions and data. These
connections are optional. With nothing configured, Songstead uses local
passwords and hides the Apps menu. Native service templates and Compose pass
all four settings below; they default to empty. Edit the service configuration
or the Compose `.env` file and restart.

## Apps menu

Set `SONGSTEAD_APPS` to a JSON array of app IDs and addresses. The current app
is left out of the menu automatically:

```sh
SONGSTEAD_APPS='[{"id":"imvault","url":"https://photos.example.org"},{"id":"songstead","url":"https://music.example.org"},{"id":"witmoot","url":"https://discuss.example.org"},{"id":"folkweave","url":"https://people.example.org"},{"id":"tidings","url":"https://read.example.org"}]'
```

Startup rejects unknown IDs, duplicates, credentials, queries and fragments.
Songstead never contacts these addresses, and a menu link grants no membership
in another app.

## Community sign-in

Configure an OpenID Connect provider:

```sh
SONGSTEAD_SHARED_ISSUER=https://identity.example.org
SONGSTEAD_SHARED_CLIENT_ID=songstead
SONGSTEAD_SHARED_CLIENT_SECRET=configure-in-your-protected-service-environment
```

Set `SONGSTEAD_BASE_URL` to the public address and register exactly
`https://your-app.example.org/auth/shared/callback` with the provider. Give each
app its own client. The issuer must use HTTPS, except loopback addresses for
local testing. Protect the client secret. Songstead makes no provider request at
startup.

Members sign in with their password, open **Account → Community sign-in** and
connect their identity. After that, the login page offers community sign-in.
An unconnected identity cannot create an account or gain permissions from its
email or claims. Suspended accounts cannot sign in. Disconnecting leaves the
password working. Signing out here does not sign out other apps or the provider.
Keep a working owner password for provider outages.

Connections are stored in the database and included in full backups, but not in
personal exports. A pending sign-in expires after five minutes or on restart.

## Witmoot discussions

Songstead prepares drafts for the configured Witmoot server and shows what the
draft includes before you leave. In Witmoot, choose a board or conversation and
check its audience before posting.

To return afterwards, add Songstead's public address to Witmoot's `WITMOOT_APPS`
and set `WITMOOT_BASE_URL`. After posting, choose Return and keep discussion
link, then confirm on Songstead's form. The return is tied to your session and
the recommendation, expires after twenty minutes or on restart, and never
changes an audience. If it expires, save the thread link by hand.

## Reading controls

**Account** holds your settings, groups, admin or invitations, community
sign-in, the theme picker and sign out. Songstead offers comfortable or compact
spacing. This browser remembers the choice for Songstead, shares it across tabs,
and falls back to comfortable when storage is off.
