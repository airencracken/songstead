# Run Songstead with Docker

Images are published at `ghcr.io/airencracken/songstead:v0.16.0` for Linux
amd64 and arm64. They hold the same static binary as the native release, plus
CA certificates, timezone data and zstd for automatic backups.

## Start an instance

Download `docker-compose.yml` from the release's source archive or tag, then
run from its directory:

```sh
docker compose pull
docker compose up -d
docker compose exec songstead create-owner --username owner --password-prompt
```

Open `http://localhost:8083`. The example listens only on host loopback. To
reach it from another computer, put a reverse proxy in front and set
`SONGSTEAD_BASE_URL` to your public HTTPS address; HTTPS addresses turn on
secure cookies. List the proxy's container or host address in
`SONGSTEAD_TRUSTED_PROXIES`. Loopback inside a container means that container.

The process runs as UID/GID `10001:10001` with a read-only root filesystem and
no Linux capabilities. The named data volume is private and survives container
replacement. For a bind mount, create the directory first with owner
`10001:10001` and mode `0700`. `docker compose down -v` deletes your data.
The healthcheck probes `/healthz` on the port in `SONGSTEAD_ADDR`.

Pin a version or digest in `.env`:

```dotenv
SONGSTEAD_IMAGE="ghcr.io/airencracken/songstead:v0.16.0"
```

For a digest, use the multi-platform index digest from
`docker buildx imagetools inspect ghcr.io/airencracken/songstead:v0.16.0`.
There is no `latest` or moving minor tag, so a pull never starts a database
migration you did not choose.

## Back up, upgrade and restore

Automatic backups live under `/data/backups`; see
[automatic backups](automatic-backups.md). Copy them off the host. For a
full-volume backup, stop the server first:

```sh
umask 077
docker compose stop songstead
docker compose run --rm --no-deps -T --entrypoint tar songstead -C /data -cf - . > songstead-before-upgrade.tar
```

Check that it succeeded and keep the file private. Read the release notes, set
`SONGSTEAD_IMAGE` to the new version, then run `docker compose pull` and
`docker compose up -d`. Check `docker compose ps` and
`docker compose logs songstead` before using it.

Migrations only go forward. To downgrade, you need the pre-upgrade backup and
the old image. Restore into a **new** named volume with an override file:

```yaml
# docker-compose.restore.yml
volumes:
  songstead-data:
    name: songstead-restored
```

Set the image version that matches the backup in `.env`, then run:

```sh
docker compose -f docker-compose.yml -f docker-compose.restore.yml run --rm --no-deps -T --entrypoint tar songstead -C /data -xf - < songstead-before-upgrade.tar
docker compose -f docker-compose.yml -f docker-compose.restore.yml up -d
```

Keep using the override afterwards. Check health, sign-in and your data before
deleting the old volume. Only restore backups you trust.

## Build locally

```sh
VERSION=0.16.0 docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

This builds `songstead:local` from source. Published images use
`Dockerfile.release`, which packages the verified native release binaries.

## Release maintenance

For maintainers: the release workflow tests both architectures before it
publishes the exact tested images under one version tag. Branch and pull request
builds never publish. On the first release, GitHub creates a private package.
Open **Packages → songstead → Package settings → Change visibility**, make it
public, then rerun the failed publisher job within seven days. A version tag is
never overwritten; ship runtime security fixes as a new patch release.

```sh
make test-containers
docker build --build-arg VERSION=0.16.0 -t songstead:local .
make test-container-lifecycle VERSION=0.16.0
```
