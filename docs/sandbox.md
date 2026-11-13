# Bubblewrap service isolation

Songstead can run the whole service inside Comfylib's Bubblewrap launcher. The
sandbox keeps the host network for HTTP and music metadata, mounts runtime files
and CA certificates read-only, and leaves only the data directory writable.
It drops Linux capabilities, strips unrelated environment settings and blocks
nested user namespaces.

Install a non-setuid Bubblewrap with unprivileged user namespaces enabled. Run
as the `songstead` service account, never root, with `/var/lib/songstead` owned
by it and mode 0700. Check the sandbox before enabling it:

```sh
songstead sandbox --check --data-dir /var/lib/songstead
songstead sandbox --data-dir /var/lib/songstead
```

The launcher passes `SONGSTEAD_` settings through. `SONGSTEAD_BWRAP` or
`--bwrap` picks another Bubblewrap binary. `SSL_CERT_FILE` may name a
certificate bundle, mounted read-only. The check creates no database. If
Bubblewrap is missing or namespaces are denied, startup fails; there is no
unconfined fallback.

## OpenRC

On Gentoo, install `sys-apps/bubblewrap`, set `SONGSTEAD_SANDBOX="true"` in
`/etc/conf.d/songstead` and restart. `false`, the default, runs the regular
service. Other values are rejected.

## systemd

Copy `contrib/systemd/songstead-sandbox.conf` to
`/etc/systemd/system/songstead.service.d/sandbox.conf`. Gentoo packages use
`/usr/bin/songstead` in the drop-in in place of `/usr/local/bin/songstead`. Run
the check as the service account, then `systemctl daemon-reload` and restart.

If namespace setup is denied, check the kernel and service policy. Never make
Bubblewrap setuid to get around it. The sandbox does not replace TLS, account
permissions or backups.
