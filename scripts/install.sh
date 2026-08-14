#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

readonly SERVICE_USER="ampcontrol"
readonly SERVICE_GROUP="ampcontrol"
readonly CONFIG_DIRECTORY="/etc/ampcontrol"
readonly STATE_DIRECTORY="/var/lib/ampcontrol"
readonly LIB_DIRECTORY="/usr/lib/ampcontrol"
readonly CREDENTIAL_DIRECTORY="/etc/credstore.encrypted"
readonly WRAPPER_PATH="/usr/local/bin/ampcontrol-amp"
readonly MAINTENANCE_PATH="/usr/local/sbin/ampcontrol-maintenance"
readonly SERVICE_PATH="/etc/systemd/system/ampcontrol.service"
readonly SUDOERS_PATH="/etc/sudoers.d/ampcontrol"
readonly CREDENTIAL_DROPIN_DIRECTORY="/etc/systemd/system/ampcontrol.service.d"
readonly CREDENTIAL_DROPIN_PATH="$CREDENTIAL_DROPIN_DIRECTORY/credentials.conf"

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
PROJECT_DIRECTORY="$(cd -- "$SCRIPT_DIRECTORY/.." && pwd -P)"
BINARY_SOURCE=""
START_SERVICE=true
TEMP_DIRECTORY=""
LEGACY_DIRECTORY=""
LEGACY_ENV_FILE=""
INSTALL_LANGUAGE=""

msg() {
    if [[ "${INSTALL_LANGUAGE:-pt-BR}" == "en-US" ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi
}

cleanup() {
    if [[ -n "$TEMP_DIRECTORY" && -d "$TEMP_DIRECTORY" ]]; then
        rm -rf -- "$TEMP_DIRECTORY"
    fi
}
trap cleanup EXIT

fail() {
    printf '%s: %s\n' "$(msg 'Erro' 'Error')" "$1" >&2
    exit 1
}

show_usage() {
    if [[ "${INSTALL_LANGUAGE:-}" == "en-US" ]]; then cat <<'EOF'
Usage: sudo ./scripts/install.sh [--language pt-BR|en-US] [--binary PATH] [--no-start] [--migrate-legacy DIRECTORY]

  --language         selects the installation language
  --binary PATH      installs an already compiled binary
  --no-start         installs without enabling or starting the service
  --migrate-legacy   imports a legacy installation, usually /opt/ampcontrol
EOF
    else cat <<'EOF'
Uso: sudo ./scripts/install.sh [--language pt-BR|en-US] [--binary CAMINHO] [--no-start] [--migrate-legacy DIRETÓRIO]

  --language         seleciona o idioma da instalação
  --binary CAMINHO   instala um binário já compilado
  --no-start         instala sem habilitar ou iniciar o serviço
  --migrate-legacy   importa uma instalação antiga, normalmente /opt/ampcontrol
EOF
    fi
}

while (($# > 0)); do
    case "$1" in
        --binary)
            (($# >= 2)) || fail "$(msg '--binary exige um caminho' '--binary requires a path')"
            BINARY_SOURCE="$2"
            shift 2
            ;;
        --no-start)
            START_SERVICE=false
            shift
            ;;
        --migrate-legacy)
            (($# >= 2)) || fail "$(msg '--migrate-legacy exige um diretório' '--migrate-legacy requires a directory')"
            LEGACY_DIRECTORY="${2%/}"
            shift 2
            ;;
        --language)
            (($# >= 2)) || fail "--language requires a value / exige um valor"
            INSTALL_LANGUAGE="$2"
            shift 2
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            fail "$(msg 'opção desconhecida' 'unknown option'): $1"
            ;;
    esac
done

[[ "$(uname -s)" == "Linux" ]] || fail "$(msg 'o instalador inicial oferece suporte apenas a Linux' 'the initial installer supports Linux only')"
[[ "$EUID" -eq 0 ]] || fail "$(msg 'execute este instalador com sudo' 'run this installer with sudo')"
[[ -t 0 ]] || fail "$(msg 'o instalador precisa de um terminal interativo' 'the installer requires an interactive terminal')"

if [[ -z "$INSTALL_LANGUAGE" ]]; then
    printf '\nChoose the installation language / Escolha o idioma da instalação:\n'
    printf '  1) Português (Brasil)\n'
    printf '  2) English (United States)\n'
    while true; do
        read -r -p 'Option / Opção [1]: ' LANGUAGE_SELECTION
        case "${LANGUAGE_SELECTION:-1}" in
            1|pt|pt-BR|pt-br)
                INSTALL_LANGUAGE="pt-BR"
                break
                ;;
            2|en|en-US|en-us)
                INSTALL_LANGUAGE="en-US"
                break
                ;;
            *)
                printf 'Invalid option / Opção inválida.\n' >&2
                ;;
        esac
    done
else
    case "${INSTALL_LANGUAGE,,}" in
        pt|pt-br) INSTALL_LANGUAGE="pt-BR" ;;
        en|en-us) INSTALL_LANGUAGE="en-US" ;;
        *) fail "invalid language / idioma inválido: $INSTALL_LANGUAGE" ;;
    esac
fi
readonly INSTALL_LANGUAGE
export AMPCONTROL_LANGUAGE="$INSTALL_LANGUAGE"

for command_name in systemctl systemd-creds install getent sudo visudo find sort python3; do
    command -v "$command_name" >/dev/null 2>&1 || fail "$(msg 'dependência ausente' 'missing dependency'): $command_name"
done

if [[ -n "$LEGACY_DIRECTORY" ]]; then
    [[ "${AMPCONTROL_MIGRATION_TRANSACTION:-}" == "1" ]] || fail "$(msg 'use scripts/migrate-legacy.sh para uma migração transacional' 'use scripts/migrate-legacy.sh for a transactional migration')"
    [[ "$LEGACY_DIRECTORY" == /* && -d "$LEGACY_DIRECTORY" ]] || fail "$(msg 'instalação legada não encontrada em' 'legacy installation not found at') $LEGACY_DIRECTORY"
    LEGACY_ENV_FILE="$LEGACY_DIRECTORY/.env"
    [[ -f "$LEGACY_ENV_FILE" ]] || fail "$(msg 'arquivo legado ausente' 'missing legacy file'): $LEGACY_ENV_FILE"
fi

legacy_env_value() {
    local name="$1"
    [[ -n "$LEGACY_ENV_FILE" ]] || return 3
    if python3 "$PROJECT_DIRECTORY/scripts/legacy_config.py" env "$LEGACY_ENV_FILE" "$name"; then
        return 0
    fi
    if [[ -n "${AMPCONTROL_LEGACY_SYSTEMD_ENV_FILE:-}" && -f "$AMPCONTROL_LEGACY_SYSTEMD_ENV_FILE" ]]; then
        python3 "$PROJECT_DIRECTORY/scripts/legacy_config.py" systemd-env \
            "$AMPCONTROL_LEGACY_SYSTEMD_ENV_FILE" "$name"
        return $?
    fi
    return 3
}

legacy_or_default() {
    local name="$1"
    local fallback="$2"
    local value=""
    value="$(legacy_env_value "$name" 2>/dev/null || true)"
    printf '%s' "${value:-$fallback}"
}

prompt_required() {
    local prompt="$1"
    local value=""
    while [[ -z "$value" ]]; do
        read -r -p "$prompt: " value
        value="${value#"${value%%[![:space:]]*}"}"
        value="${value%"${value##*[![:space:]]}"}"
    done
    printf '%s' "$value"
}

prompt_default() {
    local prompt="$1"
    local default_value="$2"
    local value=""
    read -r -p "$prompt [$default_value]: " value
    printf '%s' "${value:-$default_value}"
}

prompt_optional() {
    local prompt="$1"
    local value=""
    read -r -p "$prompt ($(msg 'opcional' 'optional')): " value
    printf '%s' "$value"
}

prompt_secret() {
    local prompt="$1"
    local first=""
    local second=""
    while true; do
        read -r -s -p "$prompt: " first
        printf '\n'
        [[ -n "$first" ]] || { printf '%s\n' "$(msg 'O valor não pode ficar vazio.' 'The value cannot be empty.')" >&2; continue; }
        read -r -s -p "$(msg 'Confirme o valor' 'Confirm the value'): " second
        printf '\n'
        [[ "$first" == "$second" ]] || { printf '%s\n' "$(msg 'Os valores não coincidem.' 'The values do not match.')" >&2; continue; }
        printf '%s' "$first"
        return
    done
}

prompt_yes_no() {
    local prompt="$1"
    local default_value="$2"
    local suffix
    suffix="$(msg '[s/N]' '[y/N]')"
    [[ "$default_value" == "true" ]] && suffix="$(msg '[S/n]' '[Y/n]')"
    local answer=""
    read -r -p "$prompt $suffix: " answer
    if [[ -z "$answer" ]]; then
        printf '%s' "$default_value"
    elif [[ "$answer" =~ ^[sSyY]$ ]]; then
        printf 'true'
    else
        printf 'false'
    fi
}

validate_discord_id() {
    [[ "$1" =~ ^[0-9]{15,20}$ ]] || fail "$2 $(msg 'não parece ser um ID válido do Discord' 'does not look like a valid Discord ID')"
}

toml_escape() {
    local value="$1"
    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//$'\n'/}"
    value="${value//$'\r'/}"
    printf '%s' "$value"
}

detect_amp_manager() {
    local detected=""
    detected="$(command -v ampinstmgr 2>/dev/null || true)"
    if [[ -z "$detected" ]]; then
        for candidate in /usr/bin/ampinstmgr /usr/local/bin/ampinstmgr /opt/cubecoders/ampinstmgr; do
            if [[ -x "$candidate" ]]; then
                detected="$candidate"
                break
            fi
        done
    fi
    printf '%s' "$detected"
}

detect_amp_users() {
    local user_name=""
    local home_directory=""
    while IFS=: read -r user_name _ _ _ _ home_directory _; do
        if [[ -d "$home_directory/.ampdata/instances" ]]; then
            printf '%s\n' "$user_name"
        fi
    done < <(getent passwd)
}

printf '\n%s\n' "$(msg 'AmpControl — instalação Linux' 'AmpControl — Linux installation')"
printf '%s\n\n' "$(msg 'A produção existente não é alterada por este script até a confirmação final.' 'The existing production installation is not changed until final confirmation.')"

AMP_MANAGER_PATH="$(detect_amp_manager)"
[[ -n "$AMP_MANAGER_PATH" ]] || AMP_MANAGER_PATH="$(prompt_required "$(msg 'Caminho absoluto do ampinstmgr' 'Absolute path to ampinstmgr')")"
[[ "$AMP_MANAGER_PATH" == /* && -x "$AMP_MANAGER_PATH" ]] || fail "$(msg 'ampinstmgr não encontrado ou não executável em' 'ampinstmgr was not found or is not executable at') $AMP_MANAGER_PATH"

mapfile -t DETECTED_AMP_USERS < <(detect_amp_users)
if ((${#DETECTED_AMP_USERS[@]} == 1)); then
    AMP_SYSTEM_USER="${DETECTED_AMP_USERS[0]}"
    printf '%s: %s\n' "$(msg 'Usuário AMP detectado' 'Detected AMP user')" "$AMP_SYSTEM_USER"
else
    AMP_SYSTEM_USER="$(prompt_default "$(msg 'Usuário Linux proprietário do AMP' 'Linux user that owns AMP')" 'amp')"
fi
[[ "$AMP_SYSTEM_USER" =~ ^[a-z_][a-z0-9_-]*[$]?$ ]] || fail "$(msg 'usuário AMP inválido' 'invalid AMP user')"
AMP_ACCOUNT="$(getent passwd "$AMP_SYSTEM_USER" || true)"
[[ -n "$AMP_ACCOUNT" ]] || fail "$(msg 'o usuário' 'user') $AMP_SYSTEM_USER $(msg 'não existe' 'does not exist')"
AMP_HOME="$(cut -d: -f6 <<<"$AMP_ACCOUNT")"
[[ -n "$AMP_HOME" ]] || fail "$(msg 'o usuário' 'user') $AMP_SYSTEM_USER $(msg 'não existe' 'does not exist')"
AMP_INSTANCES_DIRECTORY="$AMP_HOME/.ampdata/instances"
[[ -d "$AMP_INSTANCES_DIRECTORY" ]] || fail "$(msg 'diretório de instâncias não encontrado em' 'instance directory not found at') $AMP_INSTANCES_DIRECTORY"

printf '%s\n' "$(msg 'Validando acesso ao inventário AMP...' 'Validating access to the AMP inventory...')"
if ! AMP_INVENTORY_OUTPUT="$(sudo -n -u "$AMP_SYSTEM_USER" "$AMP_MANAGER_PATH" --ShowInstancesList 2>&1)"; then
    fail "$(msg 'não foi possível consultar o AMP como' 'could not query AMP as') $AMP_SYSTEM_USER: $AMP_INVENTORY_OUTPUT"
fi
AMP_INSTANCE_COUNT="$(grep -c 'Instance Name' <<<"$AMP_INVENTORY_OUTPUT" || true)"
printf '%s %s\n' "$AMP_INSTANCE_COUNT" "$(msg 'instância(s) AMP encontrada(s).' 'AMP instance(s) found.')"
unset AMP_INVENTORY_OUTPUT

PRESERVE_EXISTING_IDLE=false
if [[ -n "$LEGACY_DIRECTORY" && -f "$LEGACY_DIRECTORY/config/idle.json" ]]; then
    PRESERVE_EXISTING_IDLE=true
    printf '%s\n' "$(msg 'Configuração de Idle legada detectada e será preservada.' 'Legacy Idle configuration detected and will be preserved.')"
elif [[ -f "$STATE_DIRECTORY/config/idle.json" ]]; then
    PRESERVE_EXISTING_IDLE="$(prompt_yes_no "$(msg 'Manter a configuração de Idle da instalação existente?' 'Keep the existing installation Idle configuration?')" 'true')"
fi

mapfile -t DETECTED_INSTANCES < <(
    find "$AMP_INSTANCES_DIRECTORY" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' |
        while IFS= read -r instance_name; do
            if [[ "$instance_name" != "ADS01" && "$instance_name" =~ ^[A-Za-z0-9._-]+$ ]]; then
                printf '%s\n' "$instance_name"
            fi
        done | sort
)
SELECTED_IDLE_INSTANCES=()
if [[ "$PRESERVE_EXISTING_IDLE" == false ]] && ((${#DETECTED_INSTANCES[@]} > 0)); then
    printf '\n%s\n' "$(msg 'Instâncias disponíveis para Idle automático:' 'Instances available for automatic Idle:')"
    for index in "${!DETECTED_INSTANCES[@]}"; do
        printf '  %d) %s\n' "$((index + 1))" "${DETECTED_INSTANCES[$index]}"
    done
    read -r -p "$(msg 'Ativar Idle de 15 minutos em quais instâncias? Use todos, nenhum ou números separados por vírgula [nenhum]: ' 'Enable 15-minute Idle for which instances? Use all, none, or comma-separated numbers [none]: ')" IDLE_SELECTION
    IDLE_SELECTION="${IDLE_SELECTION:-$(msg 'nenhum' 'none')}"
    case "${IDLE_SELECTION,,}" in
        todos|all)
            SELECTED_IDLE_INSTANCES=("${DETECTED_INSTANCES[@]}")
            ;;
        nenhum|none)
            ;;
        *)
            IFS=',' read -r -a IDLE_INDEXES <<<"$IDLE_SELECTION"
            declare -A SELECTED_IDLE_INDEXES=()
            for selected_index in "${IDLE_INDEXES[@]}"; do
                selected_index="${selected_index//[[:space:]]/}"
                [[ "$selected_index" =~ ^[0-9]+$ ]] || fail "$(msg 'seleção de Idle inválida' 'invalid Idle selection'): $selected_index"
                selected_index="$((10#$selected_index))"
                ((selected_index >= 1 && selected_index <= ${#DETECTED_INSTANCES[@]})) || fail "$(msg 'índice de Idle fora da lista' 'Idle index is outside the list'): $selected_index"
                if [[ -z "${SELECTED_IDLE_INDEXES[$selected_index]:-}" ]]; then
                    SELECTED_IDLE_INSTANCES+=("${DETECTED_INSTANCES[$((selected_index - 1))]}")
                    SELECTED_IDLE_INDEXES[$selected_index]=1
                fi
            done
            ;;
    esac
fi

LEGACY_GUILD_ID="$(legacy_env_value DISCORD_GUILD_ID 2>/dev/null || true)"
if [[ -n "$LEGACY_GUILD_ID" ]]; then
    DISCORD_GUILD_ID="$(prompt_default "$(msg 'ID do servidor Discord' 'Discord server ID')" "$LEGACY_GUILD_ID")"
else
    DISCORD_GUILD_ID="$(prompt_required "$(msg 'ID do servidor Discord' 'Discord server ID')")"
fi
DISCORD_PANEL_CHANNEL_ID="$(prompt_default "$(msg 'ID do canal do painel e comandos' 'Dashboard and command channel ID')" "$(legacy_or_default DISCORD_NOTIFICATION_CHANNEL_ID '')")"
DISCORD_AUDIT_CHANNEL_ID="$(prompt_default "$(msg 'ID do canal privado de auditoria' 'Private audit channel ID')" "$(legacy_or_default DISCORD_AUDIT_CHANNEL_ID '')")"
DISCORD_OWNER_USER_ID="$(prompt_default "$(msg 'ID do proprietário do bot' 'Bot owner user ID')" "$(legacy_or_default DISCORD_OWNER_USER_ID '')")"
DISCORD_ADMIN_ROLE_IDS_RAW="$(prompt_optional "$(msg 'IDs dos cargos administrativos do Discord, separados por vírgula' 'Discord administrator role IDs, separated by commas')")"
RESTRICT_COMMAND_CHANNEL="$(prompt_yes_no "$(msg 'Restringir /amp e /ampconfig ao canal do painel?' 'Restrict /amp and /ampconfig to the dashboard channel?')" 'true')"
ALLOW_DISCORD_ADMINISTRATORS="$(prompt_yes_no "$(msg 'Autorizar automaticamente qualquer membro com permissão Administrator?' 'Automatically authorize any member with the Administrator permission?')" 'false')"
printf '%s\n' "$(msg 'Nota: o proprietário e os cargos escolhidos devem ter permissão Administrator no Discord para visualizar /ampconfig.' 'Note: the owner and selected roles must have the Discord Administrator permission to see /ampconfig.')"
validate_discord_id "$DISCORD_GUILD_ID" "$(msg 'O ID do servidor' 'The server ID')"
validate_discord_id "$DISCORD_PANEL_CHANNEL_ID" "$(msg 'O ID do canal do painel' 'The dashboard channel ID')"
validate_discord_id "$DISCORD_AUDIT_CHANNEL_ID" "$(msg 'O ID do canal de auditoria' 'The audit channel ID')"
validate_discord_id "$DISCORD_OWNER_USER_ID" "$(msg 'O ID do proprietário' 'The owner ID')"
[[ "$DISCORD_PANEL_CHANNEL_ID" != "$DISCORD_AUDIT_CHANNEL_ID" ]] || fail "$(msg 'os canais do painel e de auditoria precisam ser diferentes' 'the dashboard and audit channels must be different')"

DISCORD_ADMIN_ROLE_IDS_TOML=""
if [[ -n "$DISCORD_ADMIN_ROLE_IDS_RAW" ]]; then
    IFS=',' read -r -a DISCORD_ADMIN_ROLE_IDS <<<"$DISCORD_ADMIN_ROLE_IDS_RAW"
    for role_id in "${DISCORD_ADMIN_ROLE_IDS[@]}"; do
        role_id="${role_id//[[:space:]]/}"
        validate_discord_id "$role_id" "$(msg 'O ID de cargo administrativo' 'The administrator role ID')"
        [[ -z "$DISCORD_ADMIN_ROLE_IDS_TOML" ]] || DISCORD_ADMIN_ROLE_IDS_TOML+=", "
        DISCORD_ADMIN_ROLE_IDS_TOML+="\"$role_id\""
    done
fi

AMP_API_USERNAME="$(prompt_default "$(msg 'Usuário da API do AMP usado pelo bot' 'AMP API user used by the bot')" "$(legacy_or_default AMP_USERNAME 'ampcontrol')")"
AMP_ADS_URL="$(prompt_default "$(msg 'URL local do ADS do AMP' 'Local AMP ADS URL')" "$(legacy_or_default AMP_ADS_URL 'http://127.0.0.1:8080')")"
AMP_PUBLIC_URL="$(prompt_default "$(msg 'URL pública do painel AMP' 'Public AMP panel URL')" "$(legacy_or_default AMP_PUBLIC_URL '')")"
GAME_SERVER_ADDRESS="$(prompt_default "$(msg 'Endereço público padrão dos jogos' 'Default public game-server address')" "$(legacy_or_default AMP_GAME_SERVER_ADDRESS '')")"
DISCORD_TOKEN="$(legacy_env_value DISCORD_TOKEN 2>/dev/null || true)"
AMP_PASSWORD="$(legacy_env_value AMP_PASSWORD 2>/dev/null || true)"
if [[ -n "$LEGACY_DIRECTORY" && -n "$DISCORD_TOKEN" && -n "$AMP_PASSWORD" ]]; then
    printf '%s\n' "$(msg 'Token Discord e senha AMP serão migrados diretamente para credenciais criptografadas.' 'The Discord token and AMP password will be migrated directly into encrypted credentials.')"
else
    [[ -n "$DISCORD_TOKEN" ]] || DISCORD_TOKEN="$(prompt_secret "$(msg 'Token do bot Discord' 'Discord bot token')")"
    [[ -n "$AMP_PASSWORD" ]] || AMP_PASSWORD="$(prompt_secret "$(msg 'Senha do usuário da API do AMP' 'AMP API user password')")"
fi

printf '\n%s\n' "$(msg 'Resumo:' 'Summary:')"
printf '  AMP: %s %s, %s %s\n' "$(msg 'usuário Linux' 'Linux user')" "$AMP_SYSTEM_USER" "$(msg 'gerenciador' 'manager')" "$AMP_MANAGER_PATH"
printf '  Discord: %s %s, %s %s, %s %s\n' "$(msg 'servidor' 'server')" "$DISCORD_GUILD_ID" "$(msg 'painel' 'dashboard')" "$DISCORD_PANEL_CHANNEL_ID" "$(msg 'auditoria' 'audit')" "$DISCORD_AUDIT_CHANNEL_ID"
printf '  %s: %s\n' "$(msg 'Instalação' 'Installation')" "$LIB_DIRECTORY"
read -r -p "$(msg 'Continuar com a instalação? [s/N]: ' 'Continue with the installation? [y/N]: ')" CONFIRMATION
[[ "$CONFIRMATION" =~ ^[sSyY]$ ]] || { printf '%s\n' "$(msg 'Instalação cancelada.' 'Installation canceled.')"; exit 10; }

if [[ -z "$BINARY_SOURCE" ]]; then
    command -v go >/dev/null 2>&1 || fail "$(msg 'Go não foi encontrado; instale Go 1.26.6+ ou use --binary' 'Go was not found; install Go 1.26.6+ or use --binary')"
    TEMP_DIRECTORY="$(mktemp -d)"
    printf '%s\n' "$(msg 'Compilando AmpControl...' 'Building AmpControl...')"
    (cd "$PROJECT_DIRECTORY" && GOTOOLCHAIN=auto go build -trimpath -o "$TEMP_DIRECTORY/ampcontrol" ./cmd/ampcontrol)
    BINARY_SOURCE="$TEMP_DIRECTORY/ampcontrol"
fi
[[ -f "$BINARY_SOURCE" && -x "$BINARY_SOURCE" ]] || fail "$(msg 'binário inválido' 'invalid binary'): $BINARY_SOURCE"

BACKUP_REQUIRED=false
for existing_path in "$CONFIG_DIRECTORY" "$STATE_DIRECTORY" "$LIB_DIRECTORY/ampcontrol" "$WRAPPER_PATH" "$MAINTENANCE_PATH" "$SERVICE_PATH" "$SUDOERS_PATH" "$CREDENTIAL_DROPIN_PATH"; do
    if [[ -e "$existing_path" ]]; then
        BACKUP_REQUIRED=true
        break
    fi
done
if [[ "$BACKUP_REQUIRED" == true ]]; then
    BACKUP_DIRECTORY="/var/backups/ampcontrol/$(date -u +%Y%m%dT%H%M%SZ)"
    install -d -o root -g root -m 0700 "$BACKUP_DIRECTORY"
    [[ ! -e "$CONFIG_DIRECTORY" ]] || cp -a -- "$CONFIG_DIRECTORY" "$BACKUP_DIRECTORY/etc-ampcontrol"
    [[ ! -e "$STATE_DIRECTORY" ]] || cp -a -- "$STATE_DIRECTORY" "$BACKUP_DIRECTORY/state"
    [[ ! -e "$LIB_DIRECTORY/ampcontrol" ]] || cp -a -- "$LIB_DIRECTORY/ampcontrol" "$BACKUP_DIRECTORY/binary"
    [[ ! -e "$WRAPPER_PATH" ]] || cp -a -- "$WRAPPER_PATH" "$BACKUP_DIRECTORY/wrapper"
    [[ ! -e "$MAINTENANCE_PATH" ]] || cp -a -- "$MAINTENANCE_PATH" "$BACKUP_DIRECTORY/maintenance"
    [[ ! -e "$SERVICE_PATH" ]] || cp -a -- "$SERVICE_PATH" "$BACKUP_DIRECTORY/service"
    [[ ! -e "$SUDOERS_PATH" ]] || cp -a -- "$SUDOERS_PATH" "$BACKUP_DIRECTORY/sudoers"
    [[ ! -e "$CREDENTIAL_DROPIN_PATH" ]] || cp -a -- "$CREDENTIAL_DROPIN_PATH" "$BACKUP_DIRECTORY/credentials.conf"
    printf '%s %s\n' "$(msg 'Backup da instalação anterior criado em' 'Previous installation backup created at')" "$BACKUP_DIRECTORY"
fi

if [[ -n "$LEGACY_DIRECTORY" ]]; then
    printf '%s\n' "$(msg 'Pausando o serviço para copiar um estado consistente...' 'Stopping the service to copy a consistent state...')"
    systemctl stop ampcontrol.service
fi

if ! getent group "$SERVICE_GROUP" >/dev/null; then
    groupadd --system "$SERVICE_GROUP"
fi
if ! getent passwd "$SERVICE_USER" >/dev/null; then
    useradd --system --gid "$SERVICE_GROUP" --home-dir "$STATE_DIRECTORY" --shell /usr/sbin/nologin "$SERVICE_USER"
fi

install -d -o root -g "$SERVICE_GROUP" -m 0755 "$CONFIG_DIRECTORY"
install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0750 "$STATE_DIRECTORY/config" "$STATE_DIRECTORY/data"
install -d -o root -g root -m 0755 "$LIB_DIRECTORY"
install -d -o root -g root -m 0700 "$CREDENTIAL_DIRECTORY"
install -o root -g root -m 0755 "$BINARY_SOURCE" "$LIB_DIRECTORY/ampcontrol"
install -o root -g root -m 0755 "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" "$WRAPPER_PATH"
install -o root -g root -m 0755 "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" "$MAINTENANCE_PATH"
if [[ -n "$LEGACY_DIRECTORY" && -f "$LEGACY_DIRECTORY/config/idle.json" ]]; then
    install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0640 "$LEGACY_DIRECTORY/config/idle.json" "$STATE_DIRECTORY/config/idle.json"
    if [[ -d "$LEGACY_DIRECTORY/data" ]]; then
        cp -a -- "$LEGACY_DIRECTORY/data/." "$STATE_DIRECTORY/data/"
        chown -R "$SERVICE_USER":"$SERVICE_GROUP" "$STATE_DIRECTORY/data"
        find "$STATE_DIRECTORY/data" -type d -exec chmod 0700 {} +
        find "$STATE_DIRECTORY/data" -type f -exec chmod 0600 {} +
    fi
elif [[ "$PRESERVE_EXISTING_IDLE" == false ]]; then
    install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0640 "$PROJECT_DIRECTORY/config/idle.json" "$STATE_DIRECTORY/config/idle.json"
fi

IDLE_SERVERS_PATH="$STATE_DIRECTORY/data/idle_servers.json"
IDLE_SERVERS_TEMP="$IDLE_SERVERS_PATH.new"
if [[ "$PRESERVE_EXISTING_IDLE" == false ]]; then
{
    printf '{\n  "servers": ['
    for index in "${!SELECTED_IDLE_INSTANCES[@]}"; do
        instance_name="${SELECTED_IDLE_INSTANCES[$index]}"
        ((index == 0)) || printf ','
        printf '\n    {\n'
        printf '      "instance": "%s",\n' "$instance_name"
        printf '      "display_name": "%s",\n' "$instance_name"
        printf '      "enabled": true,\n'
        printf '      "mode": "active",\n'
        printf '      "detector": "amp_players",\n'
        printf '      "idle_timeout_minutes": 15,\n'
        printf '      "startup_grace_minutes": 5\n'
        printf '    }'
    done
    if ((${#SELECTED_IDLE_INSTANCES[@]} > 0)); then
        printf '\n  ]\n}\n'
    else
        printf ']\n}\n'
    fi
} > "$IDLE_SERVERS_TEMP"
chown "$SERVICE_USER":"$SERVICE_GROUP" "$IDLE_SERVERS_TEMP"
chmod 0600 "$IDLE_SERVERS_TEMP"
mv -f -- "$IDLE_SERVERS_TEMP" "$IDLE_SERVERS_PATH"
fi

printf 'AMPINSTMGR=%q\nINSTANCES_DIRECTORY=%q\n' "$AMP_MANAGER_PATH" "$AMP_INSTANCES_DIRECTORY" > "$CONFIG_DIRECTORY/amp-runtime.conf"
chown root:root "$CONFIG_DIRECTORY/amp-runtime.conf"
chmod 0644 "$CONFIG_DIRECTORY/amp-runtime.conf"

cat > "$CONFIG_DIRECTORY/config.toml" <<EOF
language = "$INSTALL_LANGUAGE"

[discord]
guild_id = "$(toml_escape "$DISCORD_GUILD_ID")"
notification_channel_id = "$(toml_escape "$DISCORD_PANEL_CHANNEL_ID")"
audit_channel_id = "$(toml_escape "$DISCORD_AUDIT_CHANNEL_ID")"
owner_user_id = "$(toml_escape "$DISCORD_OWNER_USER_ID")"
admin_role_ids = [$DISCORD_ADMIN_ROLE_IDS_TOML]
restrict_commands_to_channel = $RESTRICT_COMMAND_CHANNEL
allow_discord_administrators = $ALLOW_DISCORD_ADMINISTRATORS
notification_ttl_minutes = 10
status_refresh_seconds = 60
command_user_cooldown_seconds = 5
command_server_cooldown_seconds = 15

[amp]
username = "$(toml_escape "$AMP_API_USERNAME")"
ads_url = "$(toml_escape "$AMP_ADS_URL")"
public_url = "$(toml_escape "$AMP_PUBLIC_URL")"
game_server_address = "$(toml_escape "$GAME_SERVER_ADDRESS")"
system_user = "$(toml_escape "$AMP_SYSTEM_USER")"
manager_path = "$(toml_escape "$AMP_MANAGER_PATH")"
wrapper_path = "$WRAPPER_PATH"
sudo_path = "$(command -v sudo)"

[logging]
level = "info"
EOF
chown root:"$SERVICE_GROUP" "$CONFIG_DIRECTORY/config.toml"
chmod 0640 "$CONFIG_DIRECTORY/config.toml"

encrypt_credential() {
    local name="$1"
    local value="$2"
    local destination="$CREDENTIAL_DIRECTORY/ampcontrol.$name"
    local temporary="$destination.new"
    printf '%s' "$value" | systemd-creds encrypt --with-key=host --name="$name" - "$temporary" >/dev/null
    chown root:root "$temporary"
    chmod 0600 "$temporary"
    mv -f -- "$temporary" "$destination"
}
encrypt_credential discord_token "$DISCORD_TOKEN"
encrypt_credential amp_password "$AMP_PASSWORD"
unset DISCORD_TOKEN AMP_PASSWORD

RCON_MANIFEST=""
if [[ -f "$STATE_DIRECTORY/config/idle.json" ]]; then
    RCON_MANIFEST="$(mktemp)"
    MIGRATED_IDLE="$(mktemp)"
    python3 "$PROJECT_DIRECTORY/scripts/legacy_config.py" migrate-idle \
        "$STATE_DIRECTORY/config/idle.json" "$MIGRATED_IDLE" "$RCON_MANIFEST"
    install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0640 "$MIGRATED_IDLE" "$STATE_DIRECTORY/config/idle.json"
    while IFS=$'\t' read -r credential_name environment_name; do
        [[ -n "$credential_name" ]] || continue
        credential_path="$CREDENTIAL_DIRECTORY/ampcontrol.$credential_name"
        if [[ -n "$environment_name" ]]; then
            rcon_password="$(legacy_env_value "$environment_name" 2>/dev/null || true)"
            [[ -n "$rcon_password" ]] || fail "$(msg 'a variável RCON' 'RCON variable') $environment_name $(msg 'não existe no .env legado' 'does not exist in the legacy .env')"
            encrypt_credential "$credential_name" "$rcon_password"
            unset rcon_password
        fi
        [[ -f "$credential_path" ]] || fail "$(msg 'credencial RCON ausente' 'missing RCON credential'): $credential_name"
    done < "$RCON_MANIFEST"
fi

install -d -o root -g root -m 0755 "$CREDENTIAL_DROPIN_DIRECTORY"
{
    printf '[Service]\n'
    if [[ -n "$RCON_MANIFEST" ]]; then
        while IFS=$'\t' read -r credential_name _; do
            [[ -n "$credential_name" ]] || continue
            printf 'LoadCredentialEncrypted=%s:%s/ampcontrol.%s\n' \
                "$credential_name" "$CREDENTIAL_DIRECTORY" "$credential_name"
        done < "$RCON_MANIFEST"
    fi
} > "$CREDENTIAL_DROPIN_PATH.new"
install -o root -g root -m 0644 "$CREDENTIAL_DROPIN_PATH.new" "$CREDENTIAL_DROPIN_PATH"
rm -f -- "$CREDENTIAL_DROPIN_PATH.new"
[[ -z "$RCON_MANIFEST" ]] || rm -f -- "$RCON_MANIFEST" "$MIGRATED_IDLE"

TEMP_SUDOERS="$(mktemp)"
printf '%s ALL=(%s) NOPASSWD: %s *\n' "$SERVICE_USER" "$AMP_SYSTEM_USER" "$WRAPPER_PATH" > "$TEMP_SUDOERS"
chmod 0440 "$TEMP_SUDOERS"
visudo -cf "$TEMP_SUDOERS" >/dev/null || fail "$(msg 'a política sudo gerada é inválida' 'the generated sudo policy is invalid')"
install -o root -g root -m 0440 "$TEMP_SUDOERS" "$SUDOERS_PATH"
rm -f -- "$TEMP_SUDOERS"

install -o root -g root -m 0644 "$PROJECT_DIRECTORY/packaging/systemd/ampcontrol.service" "$SERVICE_PATH"
systemctl daemon-reload
if [[ "$START_SERVICE" == true ]]; then
    systemctl enable ampcontrol.service >/dev/null
    systemctl restart ampcontrol.service
systemctl is-active --quiet ampcontrol.service || fail "$(msg 'o serviço não iniciou; consulte journalctl -u ampcontrol.service' 'the service did not start; check journalctl -u ampcontrol.service')"
printf '\n%s\n' "$(msg 'AmpControl instalado e em execução.' 'AmpControl installed and running.')"
else
printf '\n%s\n' "$(msg 'AmpControl instalado. Inicie com: systemctl enable --now ampcontrol.service' 'AmpControl installed. Start it with: systemctl enable --now ampcontrol.service')"
fi
