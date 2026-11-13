# Setting up Songstead

Create the first owner in the browser or the terminal. Set the service's public
HTTPS address and listener first. Installed commands find the service's data
directory and run as its account when started with sudo. On a portable install,
run them as the account that owns the data directory.

## Browser

```sh
sudo songstead setup
```

This prints the setup address and a private key. Start the service, open
`/setup`, and enter the key with your username and password. A browser on the
same host can use the loopback HTTP address; anywhere else needs HTTPS. The key
never appears in a URL or the server log. The page closes once any owner exists
and never replaces an account.

## Terminal

```sh
sudo songstead create-owner --username yourname
sudo songstead set-password --username yourname
```

Both ask for the password twice with echo off. For automation, use
`--password-stdin` or `--password-file /private/path/password` (a private
regular file). `--data-dir DIRECTORY` selects a portable instance. Passwords
need at least 12 characters and must fit in 72 bytes, so long passwords with
accented letters may be refused. `create-owner` never changes an existing
account. `set-password` signs the account out everywhere.

Run `songstead help setup` for options. Invitations and instance settings are
in **Admin** after you sign in; see [administration](administration.md).
