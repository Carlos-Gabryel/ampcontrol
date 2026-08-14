# Linux installation

**English** · [Português (Brasil)](INSTALLATION.md)

This guide installs AmpControl without modifying game instances. Use a maintenance window for the first deployment and keep a backup of the current configuration.

## 1. Requirements

- Linux with systemd and `systemd-creds`;
- AMP installed on the same host;
- a user with `sudo` access;
- Git and Python 3;
- `curl`, `tar`, and `sha256sum` for updates;
- Go 1.26.6+ unless using `--binary`;
- a Discord server where you may install applications.

The installer requires an interactive terminal and must run from the repository root.

## 2. Create the Discord application

1. Open the [Discord Developer Portal](https://discord.com/developers/applications), create an application, and add a bot.
2. Store its token in a password manager. Never place it in an issue, commit, screenshot, or TOML file.
3. Under **Installation**, enable Guild Install.
4. Generate an installation link with the `bot` and `applications.commands` scopes.
5. In the dashboard channel, grant only: View Channel, Send Messages, Embed Links, Attach Files, Read Message History, and Manage Messages.
6. Do not grant the bot Administrator. AmpControl does not need that global permission.

AmpControl uses interactions and does not read regular messages, so Message Content Intent is unnecessary.

Enable Developer Mode in Discord and collect:

- server ID;
- dashboard and command channel ID;
- private audit channel ID;
- owner user ID;
- optional administrator role IDs.

Discord only displays `/ampconfig` to members with Administrator permission. AmpControl additionally validates the configured owner and role IDs before executing an action.

## 3. Create a dedicated AMP account

Create an AMP user exclusively for the bot. Grant only the permissions needed to list/query instances and perform the operations you intend to expose: start, stop, restart, shut down, and update. Validate the login in AMP before installation and do not reuse the primary administrator account.

## 4. Run the installer

```bash
git clone https://github.com/Carlos-Gabryel/ampcontrol.git
cd ampcontrol
sudo ./scripts/install.sh
```

The first question selects Portuguese or English for the entire installation. It can also be provided non-interactively:

```bash
sudo ./scripts/install.sh --language en-US
sudo ./scripts/install.sh --language en-US --binary /path/to/ampcontrol
sudo ./scripts/install.sh --language en-US --no-start
```

The assistant:

1. detects `ampinstmgr`, the AMP Linux user, and the instance directory;
2. validates inventory access as the AMP user;
3. offers 15-minute Idle for all, none, or selected instances;
4. collects Discord IDs and access policy;
5. collects the AMP API account, URLs, and public addresses;
6. shows a summary before changing the system;
7. installs the binary, configuration, restricted wrapper, sudoers rule, and systemd unit;
8. encrypts the Discord token and AMP password with the host key;
9. validates and starts the service unless `--no-start` was selected.

Reinstallation offers to preserve the Idle registry and backs up managed files before replacement.

## 5. Verify

```bash
sudo systemctl status ampcontrol --no-pager
sudo journalctl -u ampcontrol -n 100 --no-pager
```

In Discord:

1. verify that the guide and cards appear in the configured channel;
2. run `/amp status` as a regular user;
3. run `/ampconfig diagnostics` as the owner;
4. verify that commands are rejected in other channels when restriction is enabled;
5. verify that the private channel receives audit events.

## 6. Network and AMP URL

`amp.ads_url` should point to ADS as reachable by the service, normally `http://127.0.0.1:8080`. `amp.public_url` is optional and must be reachable from the administrator's browser. Do not expose AMP directly to the internet without TLS, authentication, and an appropriate network policy.

## 7. Update or recover

```bash
sudo ampcontrol-maintenance update
sudo ampcontrol-maintenance update v1.2.3
sudo ampcontrol-maintenance status
sudo ampcontrol-maintenance rollback
```

The updater downloads the official release, verifies its SHA-256 checksum, validates the current configuration in a temporary systemd unit, and only then replaces files. TOML, credentials, Idle registration, and local state are preserved. Installation or startup failure triggers automatic rollback.

Backups are stored under `/var/backups/ampcontrol`. Keep the latest backup until Discord, dashboards, Idle, and RCON have been validated.

## 8. Migrate a legacy installation

Legacy `/opt/ampcontrol` installations based on `.env` should use the transactional migrator:

```bash
git clone https://github.com/Carlos-Gabryel/ampcontrol.git
cd ampcontrol
sudo ./scripts/migration-preflight.sh /opt/ampcontrol --language en-US
sudo ./scripts/migrate-legacy.sh /opt/ampcontrol --language en-US
```

The preflight is read-only and never prints secret values. Fix every `FAIL` result before migration.

The migrator snapshots managed files and service state, parses `.env` without executing it, converts Discord/AMP/RCON secrets to `systemd-creds`, preserves Idle and persistent data, requests missing values, and restores the previous installation automatically if the new service fails.

The legacy directory and transactional snapshot remain intact after success. Remove them only after functional validation and an external backup.
