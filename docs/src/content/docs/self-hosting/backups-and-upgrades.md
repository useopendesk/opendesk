---
title: Backups and upgrades
description: Back up and restore a self-hosted opendesk, move between releases, and let it update itself.
---

## What to back up

| What | Why |
| --- | --- |
| The Postgres database | Everything except files. |
| The data volume (`/data`) | Attachments and raw email with the `local` storage driver. |
| `.env` | Secrets. **`CREDENTIALS_KEY` cannot be recovered**, and without it stored mailbox credentials cannot be decrypted. |

## Back up

```sh
/opt/opendesk/opendesk-ctl backup [dir]
```

This writes `backups/<UTC time>/` (or under `dir`) containing:

- `db.dump`: a `pg_dump` in custom format
- `data.tar.gz`: the opendesk data volume
- `env`: a copy of `.env`
- `VERSION`: the running release

The directory is created with mode `0700`. It contains your secrets: copy it off the machine, to encrypted storage you control. Schedule it with cron:

```sh
0 3 * * * /opt/opendesk/opendesk-ctl backup >> /var/log/opendesk-backup.log 2>&1
```

If you use an external database or S3 storage, back those up with your provider's tools as well.

## Restore

```sh
/opt/opendesk/opendesk-ctl restore /path/to/backups/20261011T030000Z
```

It stops the app, replaces the database and the data volume, and asks you to type `restore` first (`OPENDESK_YES=1` skips the prompt). Keep the same `CREDENTIALS_KEY` and `AUTH_SECRET` as the backup.

## Upgrade

```sh
/opt/opendesk/opendesk-ctl upgrade            # the newest release
/opt/opendesk/opendesk-ctl upgrade v1.2.3     # a specific release
```

An upgrade:

1. pulls the new images while the current version keeps serving
2. stops the app and takes a backup, so it holds every write (skip with `OPENDESK_SKIP_BACKUP=1`)
3. downloads that release's installer, checks it against its published SHA-256 and reruns it in place with `--version`, keeping every secret and setting
4. waits up to five minutes for the new version to answer its health check
5. if it does not, puts the previous release's files back, restores the backup when the new version's migrations ran, and starts the previous version again

Migrations run when opendesk starts, so there is no separate migration step. Releases do not skip migrations, so upgrading across several versions at once is fine; read the [release notes](https://github.com/useopendesk/opendesk/releases) first.

To roll back later by hand, restore the backup taken before the upgrade, then install the earlier version with `--version`.

## Automatic updates

The installer turns automatic updates on (`--no-auto-update` skips it). Each night around 04:00, server time, a systemd timer (or a cron entry where there is no systemd) runs `opendesk-ctl auto-update run`. When a newer release is out, it runs the [upgrade](#upgrade) above, with its backups in `backups/auto/` (the newest three are kept).

A run never installs:

- a new **major** version (`v1.x` to `v2.0.0`): those can need you to change something, so the admin panel and an email tell you, and you run `opendesk-ctl upgrade v2.0.0` after reading the release notes
- a **pre-release** (`v1.3.0-rc.1`)
- a release that already **failed** on this server: after going back, it waits for the next release (`opendesk-ctl upgrade <version>` tries it again by hand)
- anything when `OPENDESK_VERSION` is not a release (`latest`, or your own build)

```sh
/opt/opendesk/opendesk-ctl auto-update status   # on or off, the schedule, newest release, last runs
/opt/opendesk/opendesk-ctl auto-update off      # stop, and remove the schedule
/opt/opendesk/opendesk-ctl auto-update on       # start again
/opt/opendesk/opendesk-ctl auto-update run      # run now, as the timer does
```

`on` and `off` set `AUTO_UPDATE` in `.env` and add or remove the schedule (the systemd timer needs root, so they use `sudo`). The timer logs to the journal (`journalctl -u opendesk-update`); a cron entry to `auto-update.log` in the install directory. The app is down for the backup and the new version's first start, usually under a minute. Mail forwarded to opendesk and webhooks from mail providers are retried by their senders.

Each run is reported to the instance: the admin panel's **Health** page shows whether automatic updates are on and how the last run went, and the super admins get an email when one fails.

## Update notices

A release build looks for the newest release on GitHub every six hours (a plain request that carries nothing about your instance). When one is out:

- the admin panel's **Health** page says so, with the release notes and the command to run, and **Check now** looks again
- the [super admins](/self-hosting/admin-panel/) get one email per release, unless automatic updates will install it anyway
- the opendesk log has a line saying so

This works however you run opendesk, including your own Compose file or Kubernetes. Set `UPDATE_CHECK=false` to turn it off, for example on a server without internet access.

### Your own Compose file

Without `opendesk-ctl`, pin `OPENDESK_VERSION` (or the image tag) to a release, and upgrade by changing it:

```sh
docker compose pull && docker compose up -d
```

Back up first. Tools that follow the `latest` tag (Watchtower and the like) also work, but they take no backup and do not go back when a release fails, so prefer `opendesk-ctl`.
