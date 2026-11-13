# Songstead

Good music, from your people.

Songstead is a small, self-hosted inbox for music recommendations between
friends. Paste a link, share it with everyone or send it to a friend or group,
and add a note. Listen when you have time, keep track of what you have heard,
and talk about it if you like. There is no feed to keep up with.

Songstead is part of [Comfyware](https://comfyware.org/): software for a small
community, run by the people who use it.

Current release: 0.16.0. See the [changelog](CHANGELOG.md).

## What members can do

- Share YouTube, Spotify, Bandcamp, SoundCloud, Apple Music, Pandora or any other
  music link. Supported links fill in the title, artist and artwork.
- Choose the audience: everyone here (shown in **Recent**), one friend, or a group.
- Keep a private listening status, notes and song likes. Nobody sees what you
  have or have not heard.
- Comment on a recommendation, mark moments in a recording, and hide
  timestamped comments until you reach that point.
- Search titles, artists, genres, tags and messages you can see.
- Edit or delete your own posts, keep a short profile, and export your history.

Owners invite people, manage accounts and choose whether conversations happen
here, in Witmoot, or both. See [how the inbox works](docs/quiet-inbox.md).

## Install

- [Release archives](docs/releases.md) for Linux amd64 and arm64
- [Container images](docs/containers.md) for Docker Compose
- [Deployment](docs/deployment.md) behind a reverse proxy with systemd or OpenRC

Create the first owner in the browser or the terminal:

```sh
sudo songstead setup
sudo songstead create-owner --username alex
```

`setup` prints a private key for the `/setup` page. See [setup](docs/setup.md),
then [administration and invitations](docs/administration.md).

## Configuration

| Setting | Default | Purpose |
| --- | --- | --- |
| `SONGSTEAD_BASE_URL` | empty | Public address used in invitation, recovery and discussion links |
| `SONGSTEAD_WITMOOT_URL` | empty | Default Witmoot address; owners can change it in Admin |
| `SONGSTEAD_DATA_DIR` | `./data` | Private directory for the database and server lock |
| `SONGSTEAD_ADDR` | `127.0.0.1:8083` | Listen address |
| `SONGSTEAD_SECURE_COOKIES` | `false` | Set `true` when served over HTTPS |
| `SONGSTEAD_TRUSTED_PROXIES` | empty | Proxy IPs or CIDRs whose forwarding headers are trusted |

The data directory must have mode `0700`. `--data-dir` and `--addr` override
the environment. Run `songstead help` for every command.

## Back up and restore

The running server makes a compressed backup every day. See
[automatic backups](docs/automatic-backups.md). For a manual snapshot:

```sh
songstead backup --output /private/backups/songstead.db
songstead restore --input /private/backups/songstead.db --data-dir /private/restored
```

Backups contain password hashes, sessions and private recommendations. Keep
them private. Restore needs an empty, private directory. Stop the service before
switching it to the restored directory.

## Build from source

Songstead needs Go 1.26.11 or later on 1.26, or Go 1.27.4 or later on 1.27.

```sh
GOWORK=off make check build
make demo
```

The demo runs at `http://127.0.0.1:8083` with accounts `alice` and `bobby`,
both using `demo-password`. It deletes its data when stopped.
`make test-browser` adds the Chromium checks. Set `CHROMIUM` to use a local
browser.

## Support

You can [support Songstead on Ko-fi](https://ko-fi.com/airencracken).

## License

AGPL-3.0-or-later. HTMX is bundled under its own license; see
[THIRD_PARTY.md](THIRD_PARTY.md).
