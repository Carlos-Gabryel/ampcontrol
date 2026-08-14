#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
readonly LEGACY_DIRECTORY="${1:-/opt/ampcontrol}"
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
warn() { printf 'AVISO %s\n' "$1"; WARNINGS=$((WARNINGS + 1)); }
fail() { printf 'FALHA %s\n' "$1"; FAILURES=$((FAILURES + 1)); }

check_command() {
    if command -v "$1" >/dev/null 2>&1; then
        ok "dependência disponível: $1"
    else
        fail "dependência ausente: $1"
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

printf 'AmpControl — pré-validação da migração (somente leitura)\n'
printf 'Origem: %s\n\n' "$LEGACY_DIRECTORY"

if [[ "$(uname -s)" == Linux ]]; then ok 'sistema Linux detectado'; else fail 'o sistema não é Linux'; fi
if [[ "$EUID" -eq 0 ]]; then ok 'execução com privilégios para ler os segredos legados'; else fail 'execute com sudo'; fi
for dependency in python3 systemctl systemd-creds sudo install visudo find sort; do
    check_command "$dependency"
done

if [[ -d "$LEGACY_DIRECTORY" ]]; then ok 'diretório legado encontrado'; else fail 'diretório legado ausente'; fi
if [[ -f "$ENV_FILE" ]]; then ok '.env legado encontrado'; else fail '.env legado ausente'; fi
if [[ -x "$LEGACY_DIRECTORY/ampcontrol" ]]; then ok 'binário legado encontrado'; else fail 'binário legado ausente ou não executável'; fi
if [[ -f "$IDLE_FILE" ]]; then ok 'configuração de Idle encontrada'; else fail 'configuração de Idle ausente'; fi
if [[ -d "$LEGACY_DIRECTORY/data" ]]; then ok 'diretório de estado encontrado'; else warn 'diretório data não existe'; fi

TEMP_DIRECTORY="$(mktemp -d)"
if systemctl cat ampcontrol.service >/dev/null 2>&1; then
    systemctl show ampcontrol.service --property=Environment --value > "$TEMP_DIRECTORY/systemd-environment"
fi

if [[ -f "$ENV_FILE" ]]; then
    for required in DISCORD_TOKEN DISCORD_NOTIFICATION_CHANNEL_ID DISCORD_AUDIT_CHANNEL_ID DISCORD_OWNER_USER_ID AMP_USERNAME AMP_PASSWORD; do
        if env_present "$required"; then ok "variável legada presente: $required"; else fail "variável legada ausente: $required"; fi
    done
    for optional in DISCORD_GUILD_ID AMP_ADS_URL AMP_PUBLIC_URL AMP_GAME_SERVER_ADDRESS LOG_LEVEL; do
        if env_present "$optional"; then ok "variável opcional presente: $optional"; else warn "será solicitada ou usará padrão: $optional"; fi
    done
fi

if [[ -f "$IDLE_FILE" && -f "$ENV_FILE" ]]; then
    if python3 "$SCRIPT_DIRECTORY/legacy_config.py" migrate-idle \
        "$IDLE_FILE" "$TEMP_DIRECTORY/idle.json" "$TEMP_DIRECTORY/rcon.tsv"; then
        ok 'JSON de Idle aceito pelo conversor'
        while IFS=$'\t' read -r credential environment; do
            [[ -n "$credential" ]] || continue
            if [[ -z "$environment" ]]; then
                warn "credencial $credential já usa o formato novo; confirme o arquivo criptografado"
            elif env_present "$environment"; then
                ok "RCON migrável: $environment -> $credential"
            else
                fail "senha RCON ausente no .env: $environment"
            fi
        done < "$TEMP_DIRECTORY/rcon.tsv"
    else
        fail 'a configuração de Idle não pôde ser convertida'
    fi
fi

if systemctl cat ampcontrol.service >/dev/null 2>&1; then
    ok 'unidade ampcontrol.service encontrada'
    printf 'INFO estado=%s habilitado=%s\n' \
        "$(systemctl is-active ampcontrol.service 2>/dev/null || true)" \
        "$(systemctl is-enabled ampcontrol.service 2>/dev/null || true)"
    systemctl show ampcontrol.service \
        -p User -p Group -p WorkingDirectory -p ExecStart --no-pager |
        sed 's/^/INFO /'
else
    fail 'unidade ampcontrol.service não encontrada'
fi

for target in /etc/ampcontrol /var/lib/ampcontrol /usr/lib/ampcontrol; do
    if [[ -e "$target" ]]; then
        warn "destino novo já existe e será incluído no backup: $target"
    else
        ok "destino novo livre: $target"
    fi
done

if [[ -d "$LEGACY_DIRECTORY" ]]; then
    printf 'INFO tamanho_origem=%s\n' "$(du -sh -- "$LEGACY_DIRECTORY" 2>/dev/null | awk '{print $1}')"
fi
printf '\nResultado: %d falha(s), %d aviso(s).\n' "$FAILURES" "$WARNINGS"
[[ "$FAILURES" -eq 0 ]]
