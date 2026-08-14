#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
LANGUAGE="${AMPCONTROL_LANGUAGE:-pt-BR}"
LEGACY_DIRECTORY="/opt/ampcontrol"
while (($# > 0)); do
    case "$1" in
        --language) [[ $# -ge 2 ]] || { printf 'Error / Erro: --language requires a value / exige um valor\n' >&2; exit 2; }; case "${2,,}" in pt|pt-br) LANGUAGE="pt-BR" ;; en|en-us) LANGUAGE="en-US" ;; *) printf 'Error / Erro: invalid language / idioma inválido: %s\n' "$2" >&2; exit 2 ;; esac; shift 2 ;;
        -h|--help) if [[ "$LANGUAGE" == "en-US" ]]; then printf 'Usage: sudo ./scripts/migration-preflight.sh [DIRECTORY] [--language pt-BR|en-US]\n'; else printf 'Uso: sudo ./scripts/migration-preflight.sh [DIRETÓRIO] [--language pt-BR|en-US]\n'; fi; exit 0 ;;
        *) LEGACY_DIRECTORY="$1"; shift ;;
    esac
done
[[ "$LANGUAGE" == "en-US" ]] || LANGUAGE="pt-BR"
msg() { if [[ "$LANGUAGE" == "en-US" ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi; }
readonly LANGUAGE LEGACY_DIRECTORY
export AMPCONTROL_LANGUAGE="$LANGUAGE"
readonly ENV_FILE="$LEGACY_DIRECTORY/.env"
readonly IDLE_FILE="$LEGACY_DIRECTORY/config/idle.json"

FAILURES=0
WARNINGS=0
TEMP_DIRECTORY=""

cleanup() {
    [[ -z "$TEMP_DIRECTORY" || ! -d "$TEMP_DIRECTORY" ]] || rm -rf -- "$TEMP_DIRECTORY"
}
trap cleanup EXIT

ok() { printf 'OK   %s\n' "$1"; }
warn() { printf '%s %s\n' "$(msg 'AVISO' 'WARN ')" "$1"; WARNINGS=$((WARNINGS + 1)); }
fail() { printf '%s %s\n' "$(msg 'FALHA' 'FAIL ')" "$1"; FAILURES=$((FAILURES + 1)); }

check_command() {
    if command -v "$1" >/dev/null 2>&1; then
        ok "$(msg 'dependência disponível' 'dependency available'): $1"
    else
        fail "$(msg 'dependência ausente' 'missing dependency'): $1"
    fi
}

env_present() {
    if python3 "$SCRIPT_DIRECTORY/legacy_config.py" env "$ENV_FILE" "$1" >/dev/null 2>&1; then
        return 0
    fi
    [[ -n "$TEMP_DIRECTORY" && -f "$TEMP_DIRECTORY/systemd-environment" ]] &&
        python3 "$SCRIPT_DIRECTORY/legacy_config.py" systemd-env \
            "$TEMP_DIRECTORY/systemd-environment" "$1" >/dev/null 2>&1
}

printf '%s\n' "$(msg 'AmpControl — pré-validação da migração (somente leitura)' 'AmpControl — migration preflight (read-only)')"
printf '%s: %s\n\n' "$(msg 'Origem' 'Source')" "$LEGACY_DIRECTORY"

if [[ "$(uname -s)" == Linux ]]; then ok "$(msg 'sistema Linux detectado' 'Linux system detected')"; else fail "$(msg 'o sistema não é Linux' 'the system is not Linux')"; fi
if [[ "$EUID" -eq 0 ]]; then ok "$(msg 'execução com privilégios para ler os segredos legados' 'running with privileges to read legacy secrets')"; else fail "$(msg 'execute com sudo' 'run with sudo')"; fi
for dependency in python3 systemctl systemd-creds sudo install visudo find sort; do
    check_command "$dependency"
done

if [[ -d "$LEGACY_DIRECTORY" ]]; then ok "$(msg 'diretório legado encontrado' 'legacy directory found')"; else fail "$(msg 'diretório legado ausente' 'legacy directory missing')"; fi
if [[ -f "$ENV_FILE" ]]; then ok "$(msg '.env legado encontrado' 'legacy .env found')"; else fail "$(msg '.env legado ausente' 'legacy .env missing')"; fi
if [[ -x "$LEGACY_DIRECTORY/ampcontrol" ]]; then ok "$(msg 'binário legado encontrado' 'legacy binary found')"; else fail "$(msg 'binário legado ausente ou não executável' 'legacy binary missing or not executable')"; fi
if [[ -f "$IDLE_FILE" ]]; then ok "$(msg 'configuração de Idle encontrada' 'Idle configuration found')"; else fail "$(msg 'configuração de Idle ausente' 'Idle configuration missing')"; fi
if [[ -d "$LEGACY_DIRECTORY/data" ]]; then ok "$(msg 'diretório de estado encontrado' 'state directory found')"; else warn "$(msg 'diretório data não existe' 'data directory does not exist')"; fi

TEMP_DIRECTORY="$(mktemp -d)"
if systemctl cat ampcontrol.service >/dev/null 2>&1; then
    systemctl show ampcontrol.service --property=Environment --value > "$TEMP_DIRECTORY/systemd-environment"
fi

if [[ -f "$ENV_FILE" ]]; then
    for required in DISCORD_TOKEN DISCORD_NOTIFICATION_CHANNEL_ID DISCORD_AUDIT_CHANNEL_ID DISCORD_OWNER_USER_ID AMP_USERNAME AMP_PASSWORD; do
        if env_present "$required"; then ok "$(msg 'variável legada presente' 'legacy variable present'): $required"; else fail "$(msg 'variável legada ausente' 'legacy variable missing'): $required"; fi
    done
    for optional in DISCORD_GUILD_ID AMP_ADS_URL AMP_PUBLIC_URL AMP_GAME_SERVER_ADDRESS LOG_LEVEL; do
        if env_present "$optional"; then ok "$(msg 'variável opcional presente' 'optional variable present'): $optional"; else warn "$(msg 'será solicitada ou usará padrão' 'will be prompted or use a default'): $optional"; fi
    done
fi

if [[ -f "$IDLE_FILE" && -f "$ENV_FILE" ]]; then
    if python3 "$SCRIPT_DIRECTORY/legacy_config.py" migrate-idle \
        "$IDLE_FILE" "$TEMP_DIRECTORY/idle.json" "$TEMP_DIRECTORY/rcon.tsv"; then
        ok "$(msg 'JSON de Idle aceito pelo conversor' 'Idle JSON accepted by the converter')"
        while IFS=$'\t' read -r credential environment; do
            [[ -n "$credential" ]] || continue
            if [[ -z "$environment" ]]; then
                warn "$(msg 'credencial' 'credential') $credential $(msg 'já usa o formato novo; confirme o arquivo criptografado' 'already uses the new format; confirm the encrypted file')"
            elif env_present "$environment"; then
                ok "$(msg 'RCON migrável' 'migratable RCON'): $environment -> $credential"
            else
                fail "$(msg 'senha RCON ausente no .env' 'RCON password missing from .env'): $environment"
            fi
        done < "$TEMP_DIRECTORY/rcon.tsv"
    else
        fail "$(msg 'a configuração de Idle não pôde ser convertida' 'the Idle configuration could not be converted')"
    fi
fi

if systemctl cat ampcontrol.service >/dev/null 2>&1; then
    ok "$(msg 'unidade ampcontrol.service encontrada' 'ampcontrol.service unit found')"
    printf 'INFO %s=%s %s=%s\n' "$(msg 'estado' 'state')" \
        "$(systemctl is-active ampcontrol.service 2>/dev/null || true)" \
        "$(msg 'habilitado' 'enabled')" \
        "$(systemctl is-enabled ampcontrol.service 2>/dev/null || true)"
    systemctl show ampcontrol.service \
        -p User -p Group -p WorkingDirectory -p ExecStart --no-pager |
        sed 's/^/INFO /'
else
    fail "$(msg 'unidade ampcontrol.service não encontrada' 'ampcontrol.service unit not found')"
fi

for target in /etc/ampcontrol /var/lib/ampcontrol /usr/lib/ampcontrol; do
    if [[ -e "$target" ]]; then
        warn "$(msg 'destino novo já existe e será incluído no backup' 'new destination already exists and will be included in the backup'): $target"
    else
        ok "$(msg 'destino novo livre' 'new destination available'): $target"
    fi
done

if [[ -d "$LEGACY_DIRECTORY" ]]; then
    printf 'INFO %s=%s\n' "$(msg 'tamanho_origem' 'source_size')" "$(du -sh -- "$LEGACY_DIRECTORY" 2>/dev/null | awk '{print $1}')"
fi
printf '\n%s: %d %s, %d %s.\n' "$(msg 'Resultado' 'Result')" "$FAILURES" "$(msg 'falha(s)' 'failure(s)')" "$WARNINGS" "$(msg 'aviso(s)' 'warning(s)')"
[[ "$FAILURES" -eq 0 ]]
