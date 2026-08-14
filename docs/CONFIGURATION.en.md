# Configuration

**English** · [Português (Brasil)](CONFIGURATION.md)

Non-secret configuration is stored in `/etc/ampcontrol/config.toml`. See the complete [`config/ampcontrol.example.en.toml`](../config/ampcontrol.example.en.toml).

| Key | Purpose |
| --- | --- |
| `language` | Global `pt-BR` or `en-US` language for commands, dashboards, responses, audit, logs, and maintenance. |

## Discord

| Key | Purpose |
| --- | --- |
| `guild_id` | Discord server where commands are registered |
| `notification_channel_id` | Guide, dashboard, and command channel |
| `audit_channel_id` | Private audit destination |
| `owner_user_id` | Owner allowed to use `/ampconfig` |
| `admin_role_ids` | Additional roles allowed to use `/ampconfig` |
| `restrict_commands_to_channel` | Rejects commands outside the dashboard channel |
| `allow_discord_administrators` | Allows any Discord Administrator in addition to explicit IDs |
| `notification_ttl_minutes` | Lifetime of temporary messages |
| `status_refresh_seconds` | Card refresh interval |
| `command_user_cooldown_seconds` | Minimum interval per user |
| `command_server_cooldown_seconds` | Minimum operation interval per instance |

Keep `allow_discord_administrators = false` and explicitly list trusted owners and roles whenever possible.

## AMP

| Key | Purpose |
| --- | --- |
| `username` | Dedicated AMP API account |
| `ads_url` | Local ADS endpoint |
| `public_url` | URL opened by the administrator from Details |
| `game_server_address` | Default public address shown on cards |
| `system_user` | Linux user that owns AMP |
| `manager_path` | `ampinstmgr` path |
| `wrapper_path` | Restricted privileged wrapper |
| `sudo_path` | `sudo` binary used by the service |

## Secrets

New installations load `discord_token` and `amp_password` through systemd. Encrypted files reside at `/etc/credstore.encrypted/ampcontrol.*` and are decrypted only for the service. Re-running the installer is the safest credential-rotation method.

RCON credentials use names such as `rcon_<instance>`. Store only `password_credential` in Idle JSON—never the password itself.

## Idle and detectors

`/var/lib/ampcontrol/config/idle.json` contains per-instance registration. Available detectors are:

- `amp`: AMP API telemetry;
- `amp_palworld_rcon`: authoritative Palworld RCON fallback;
- `amp_project_zomboid_rcon`: authoritative Project Zomboid RCON fallback.

Failures and ambiguous counts are handled conservatively: destructive commands from regular users are blocked whenever the system cannot safely confirm that a server is empty.

## Discord administration

English installations provide:

| Subcommand | Action |
| --- | --- |
| `diagnostics` | Checks integrations |
| `configure` | Customizes name, game, address, maximum, and detector |
| `details` | Shows effective configuration |
| `reset` | Removes customizations |
| `hide` / `show` | Controls dashboard and command visibility |
| `list` | Lists hidden instances |
| `idle-add` | Registers an instance for 15-minute automatic Idle |

Discord preferences and Idle state are stored under the installation data directory and must remain writable only by the service user.

## Environment compatibility

Legacy environment variables remain accepted during migration and override equivalent TOML values. Avoid them for new installations because secrets are more likely to appear in files, dumps, or inspection tools.
