# Bubblewrap service isolation

Songstead 0.1.1 supports whole-service isolation through Comfylib's Bubblewrap
launcher. It retains the host network for HTTP and optional music metadata,
mounts runtime files and CA certificates read-only, and exposes only the existing
private data directory as writable host storage. Temporary files stay inside the
sandbox. Unrelated environment settings and loader variables are omitted, Linux
capabilities are dropped, and nested user namespaces are disabled. Songstead
launches no media conversion helpers.

Install a non-setuid Bubblewrap with unprivileged user namespaces available.
Run as the `songstead` service account, never root. Provision accounts first and
create `/var/lib/songstead` with that owner and mode 0700. Check the actual
namespaces and bound binary before enabling the service:

```sh
songstead sandbox --check --data-dir /var/lib/songstead
songstead sandbox --data-dir /var/lib/songstead
```

The launcher forwards `SONGSTEAD_` settings and preserves the HTTP server's
loopback default. `SONGSTEAD_BWRAP` or `--bwrap` selects an alternate launcher.
`SSL_CERT_FILE` may name an existing certificate bundle; it is mounted read-only.
The check creates no database. Missing Bubblewrap, invalid mounts, or namespace
restrictions fail startup; there is no unrestricted fallback.

## OpenRC

On Gentoo, install `sys-apps/bubblewrap` and set
`SONGSTEAD_SANDBOX="true"` in `/etc/conf.d/songstead`, then restart the service.
Use `false` to choose the regular service explicitly. Other values are rejected.
The opt-in setting defaults to `false` for existing installations.

## systemd

Copy `contrib/systemd/songstead-sandbox.conf` to
`/etc/systemd/system/songstead.service.d/sandbox.conf`, creating the directory
first. Native archives use `/usr/local/bin/songstead`; for a Gentoo package,
use `/usr/bin/songstead` in the drop-in. Run the check as the service account,
then `systemctl daemon-reload` and restart Songstead. The drop-in permits the
namespaces Bubblewrap requires and gives the supervisor time for shutdown.
Keep the existing unit's service user and filesystem restrictions.

If namespace setup is denied, check the kernel and service policy. Do not make
Bubblewrap setuid or enable a fallback to bypass a failed check. HTTP remains
available through the existing reverse proxy; Bubblewrap does not replace TLS,
account permissions, backups, or the application's outbound metadata checks.

## Validation

`make test-sandbox` requires Bubblewrap and functioning namespaces. The tests
start the real binary, check `/healthz`, stop it with SIGTERM, verify persistent
private data, and assert that host files and unrelated settings remain hidden.
`make check` also covers race, input, packaging, and security mutation tests.
CI and release jobs run both targets on a disposable Linux runner.
