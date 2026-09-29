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

Idle registration lives in two files, both read at startup (an instance cannot appear in both):

| File | Written by |
| --- | --- |
| `/var/lib/ampcontrol/data/idle_servers.json` | the installer (instances chosen in the wizard) and `/ampconfig idle-add` |
| `/var/lib/ampcontrol/config/idle.json` | you, by hand, for fine-tuning such as RCON and per-instance timings |

The normal path requires no JSON editing: the installer and `/ampconfig idle-add` register the instance already in `active` mode, with the AMP API detector and a 15-minute timeout. `/ampconfig configure` changes the detection method:

| Discord method | Detector | Fallback |
| --- | --- | --- |
| `amp` | `amp_players` | — |
| `amp_palworld_rcon` | `amp_players` | `palworld_rcon` |
| `amp_project_zomboid_rcon` | `amp_players` | `project_zomboid_rcon` |

RCON methods are only accepted for instances that already have an RCON address and credential configured (see [RCON](#rcon)).

### `idle.json` format

Full example (also in [`config/idle.example.json`](../config/idle.example.json)):

```json
{
  "check_interval_seconds": 30,
  "default_idle_timeout_minutes": 15,
  "servers": [
    {
      "instance": "Palworld01",
      "display_name": "My server",
      "game": "Palworld",
      "enabled": true,
      "mode": "active",
      "detector": "amp_players",
      "fallback_detector": "palworld_rcon",
      "idle_timeout_minutes": 15,
      "startup_grace_minutes": 5,
      "rcon": {
        "address": "127.0.0.1:25575",
        "password_credential": "rcon_Palworld01"
      }
    }
  ]
}
```

| Key | Default | Description |
| --- | --- | --- |
| `check_interval_seconds` | `30` | Interval between checks |
| `default_idle_timeout_minutes` | `15` | Time without players when the instance does not set `idle_timeout_minutes` |
| `instance` | — | AMP instance name (required) |
| `display_name` | instance name | Name shown in Discord |
| `game` | — | Game name shown in Discord |
| `enabled` | `false` | Turns Idle on for the instance |
| `mode` | `observe` | `active` stops the game process; `observe` only logs what it would do |
| `detector` | — | Required with `enabled: true`: `amp_players`, `palworld_rcon`, or `project_zomboid_rcon` |
| `fallback_detector` | — | Detector used when the primary one cannot count players |
| `idle_timeout_minutes` | `default_idle_timeout_minutes` | Time without players before stopping |
| `startup_grace_minutes` | `5` | Grace period after the game starts |
| `rcon.address` | — | RCON `host:port`, required with an RCON detector |
| `rcon.password_credential` | — | Name of the systemd credential holding the RCON password |

> **Mind `mode`:** without `"mode": "active"`, the instance stays in `observe` and Idle **never stops the server**; it only logs. Unknown keys or invalid values make the service reject the file and **fail to start**. Keep a copy before editing and, after restarting, check `systemctl status ampcontrol.service` and `journalctl -u ampcontrol.service -n 20`.

### RCON

RCON gives a more reliable player count for Palworld and Project Zomboid. The password never goes into the JSON: it is an encrypted systemd credential named `rcon_<instance>`. To set it up on a fresh installation (example with `Palworld01`):

```bash
# 1. Encrypt the password (read without echo and kept out of shell history)
read -rsp 'RCON password: ' RCON_PASSWORD; echo
printf '%s' "$RCON_PASSWORD" | sudo systemd-creds encrypt --with-key=host --name=rcon_Palworld01 - /etc/credstore.encrypted/ampcontrol.rcon_Palworld01
unset RCON_PASSWORD
sudo chmod 0600 /etc/credstore.encrypted/ampcontrol.rcon_Palworld01

# 2. Hand the credential to the service
echo 'LoadCredentialEncrypted=rcon_Palworld01:/etc/credstore.encrypted/ampcontrol.rcon_Palworld01' | sudo tee -a /etc/systemd/system/ampcontrol.service.d/credentials.conf
```

3. In `/var/lib/ampcontrol/config/idle.json`, add the instance with the `rcon` block from the example above. If it is already in `data/idle_servers.json` (registered by the installer or from Discord), remove it from there, since the same instance cannot be in both files.
4. Apply: `sudo systemctl daemon-reload && sudo systemctl restart ampcontrol.service`, then check with `/ampconfig diagnostics`.

When re-running the installer, answer **yes** when it asks whether to keep the existing Idle configuration: it rebuilds `credentials.conf` from the `password_credential` references in `idle.json`, so the RCON credentials stay loaded.

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
