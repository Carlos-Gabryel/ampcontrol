#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
LEGACY_DIRECTORY="/opt/ampcontrol"
BINARY_PATH=""
INSTALL_LANGUAGE="${AMPCONTROL_LANGUAGE:-pt-BR}"

msg() { if [[ "$INSTALL_LANGUAGE" == "en-US" ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi; }

fail() {
    printf '%s: %s\n' "$(msg 'Erro' 'Error')" "$1" >&2
    exit 1
}

usage() {
    if [[ "$INSTALL_LANGUAGE" == "en-US" ]]; then cat <<'EOF'
Usage: sudo ./scripts/migrate-legacy.sh [DIRECTORY] [--binary PATH] [--language pt-BR|en-US]

  DIRECTORY       legacy installation (default: /opt/ampcontrol)
  --binary PATH   uses an already compiled Linux binary
  --language      selects the migration and installation language
EOF
    else cat <<'EOF'
Uso: sudo ./scripts/migrate-legacy.sh [DIRETÓRIO] [--binary CAMINHO] [--language pt-BR|en-US]

  DIRETÓRIO        instalação legada (padrão: /opt/ampcontrol)
  --binary CAMINHO usa um binário Linux já compilado
  --language        seleciona o idioma da migração e instalação
EOF
    fi
}

while (($# > 0)); do
    case "$1" in
        --binary)
            (($# >= 2)) || fail "$(msg '--binary exige um caminho' '--binary requires a path')"
            BINARY_PATH="$2"
            shift 2
            ;;
        --language)
            (($# >= 2)) || fail "--language requires a value / exige um valor"
            case "${2,,}" in pt|pt-br) INSTALL_LANGUAGE="pt-BR" ;; en|en-us) INSTALL_LANGUAGE="en-US" ;; *) fail "invalid language / idioma inválido: $2" ;; esac
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        -* )
            fail "$(msg 'opção desconhecida' 'unknown option'): $1"
            ;;
        *)
            [[ "$LEGACY_DIRECTORY" == "/opt/ampcontrol" ]] || fail "$(msg 'informe somente um diretório legado' 'provide only one legacy directory')"
            LEGACY_DIRECTORY="$1"
            shift
            ;;
    esac
done

readonly LEGACY_DIRECTORY BINARY_PATH
export AMPCONTROL_LANGUAGE="$INSTALL_LANGUAGE"
readonly BACKUP_ROOT="/var/backups/ampcontrol"
BACKUP_DIRECTORY="$BACKUP_ROOT/migration-$(date -u +%Y%m%dT%H%M%SZ)"
readonly BACKUP_DIRECTORY
readonly SNAPSHOT_DIRECTORY="$BACKUP_DIRECTORY/rootfs"
readonly MANAGED_PATHS=(
    /etc/ampcontrol
    /var/lib/ampcontrol
    /usr/lib/ampcontrol
    /usr/local/bin/ampcontrol-amp
    /etc/systemd/system/ampcontrol.service
    /etc/systemd/system/ampcontrol.service.d
    /etc/sudoers.d/ampcontrol
)

ROLLBACK_ARMED=false
SERVICE_WAS_ACTIVE=false
SERVICE_WAS_ENABLED=false

copy_to_snapshot() {
    local source="$1"
    local destination="$SNAPSHOT_DIRECTORY$source"
    [[ -e "$source" ]] || return 0
    mkdir -p -- "$(dirname -- "$destination")"
    cp -a -- "$source" "$destination"
}

restore_snapshot() {
    local path=""
    set +e
    printf '\n%s\n' "$(msg 'Falha detectada. Restaurando o estado anterior...' 'Failure detected. Restoring the previous state...')" >&2
    systemctl stop ampcontrol.service >/dev/null 2>&1 || true
    for path in "${MANAGED_PATHS[@]}"; do
        rm -rf -- "$path"
        if [[ -e "$SNAPSHOT_DIRECTORY$path" ]]; then
            mkdir -p -- "$(dirname -- "$path")"
            cp -a -- "$SNAPSHOT_DIRECTORY$path" "$path"
        fi
    done
    rm -f -- /etc/credstore.encrypted/ampcontrol.*
    if [[ -d "$SNAPSHOT_DIRECTORY/etc/credstore.encrypted" ]]; then
        cp -a -- "$SNAPSHOT_DIRECTORY/etc/credstore.encrypted/." /etc/credstore.encrypted/
    fi
    systemctl daemon-reload
    if [[ "$SERVICE_WAS_ENABLED" == true ]]; then
        systemctl enable ampcontrol.service >/dev/null 2>&1 || true
    else
        systemctl disable ampcontrol.service >/dev/null 2>&1 || true
    fi
    if [[ "$SERVICE_WAS_ACTIVE" == true ]]; then
        systemctl start ampcontrol.service >/dev/null 2>&1 || true
    fi
    printf '%s %s\n' "$(msg 'Rollback concluído. Backup preservado em' 'Rollback completed. Backup preserved at')" "$BACKUP_DIRECTORY" >&2
}

on_error() {
    local status=$?
    if [[ "$ROLLBACK_ARMED" == true ]]; then
        restore_snapshot
    fi
    exit "$status"
}
trap on_error ERR

[[ "$(uname -s)" == Linux ]] || fail "$(msg 'a migração oferece suporte apenas a Linux' 'migration supports Linux only')"
[[ "$EUID" -eq 0 ]] || fail "$(msg 'execute com sudo' 'run with sudo')"
[[ -t 0 ]] || fail "$(msg 'a migração precisa de um terminal interativo' 'migration requires an interactive terminal')"
[[ "$LEGACY_DIRECTORY" == /* && -d "$LEGACY_DIRECTORY" ]] || fail "$(msg 'instalação legada não encontrada em' 'legacy installation not found at') $LEGACY_DIRECTORY"
[[ -f "$LEGACY_DIRECTORY/.env" ]] || fail "$(msg 'arquivo .env legado não encontrado' 'legacy .env file not found')"
[[ -x "$SCRIPT_DIRECTORY/install.sh" ]] || fail "$(msg 'install.sh não está executável' 'install.sh is not executable')"

if systemctl is-active --quiet ampcontrol.service; then
    SERVICE_WAS_ACTIVE=true
fi
if systemctl is-enabled --quiet ampcontrol.service; then
    SERVICE_WAS_ENABLED=true
fi

install -d -o root -g root -m 0700 "$SNAPSHOT_DIRECTORY"
for managed_path in "${MANAGED_PATHS[@]}"; do
    copy_to_snapshot "$managed_path"
done
if compgen -G '/etc/credstore.encrypted/ampcontrol.*' >/dev/null; then
    install -d -o root -g root -m 0700 "$SNAPSHOT_DIRECTORY/etc/credstore.encrypted"
    cp -a -- /etc/credstore.encrypted/ampcontrol.* "$SNAPSHOT_DIRECTORY/etc/credstore.encrypted/"
fi
cp -a -- "$LEGACY_DIRECTORY" "$BACKUP_DIRECTORY/legacy-installation"
SYSTEMD_ENVIRONMENT_FILE="$BACKUP_DIRECTORY/legacy-systemd-environment"
readonly SYSTEMD_ENVIRONMENT_FILE
systemctl show ampcontrol.service --property=Environment --value > "$SYSTEMD_ENVIRONMENT_FILE"
chmod 0600 "$SYSTEMD_ENVIRONMENT_FILE"
printf 'active=%s\nenabled=%s\nlegacy=%s\n' \
    "$SERVICE_WAS_ACTIVE" "$SERVICE_WAS_ENABLED" "$LEGACY_DIRECTORY" > "$BACKUP_DIRECTORY/metadata"

ROLLBACK_ARMED=true
printf '%s %s\n' "$(msg 'Backup transacional criado em' 'Transactional backup created at')" "$BACKUP_DIRECTORY"
INSTALL_ARGUMENTS=(--language "$INSTALL_LANGUAGE" --no-start --migrate-legacy "$LEGACY_DIRECTORY")
if [[ -n "$BINARY_PATH" ]]; then
    INSTALL_ARGUMENTS+=(--binary "$BINARY_PATH")
fi
AMPCONTROL_MIGRATION_TRANSACTION=1 \
AMPCONTROL_LEGACY_SYSTEMD_ENV_FILE="$SYSTEMD_ENVIRONMENT_FILE" \
    "$SCRIPT_DIRECTORY/install.sh" "${INSTALL_ARGUMENTS[@]}"

systemctl daemon-reload
systemctl enable ampcontrol.service >/dev/null
systemctl restart ampcontrol.service
for _ in {1..10}; do
    systemctl is-active --quiet ampcontrol.service && break
    sleep 1
done
if ! systemctl is-active --quiet ampcontrol.service; then
    printf '%s\n' "$(msg 'O novo serviço não permaneceu ativo.' 'The new service did not remain active.')" >&2
    false
fi

ROLLBACK_ARMED=false
printf '\n%s %s.\n' "$(msg 'Migração concluída. A instalação legada foi preservada em' 'Migration completed. The legacy installation was preserved at')" "$LEGACY_DIRECTORY"
printf '%s: %s\n' "$(msg 'Backup para rollback' 'Rollback backup')" "$BACKUP_DIRECTORY"
printf '%s\n' "$(msg 'Valide o Discord e os detectores RCON antes de remover qualquer backup.' 'Validate Discord and RCON detectors before removing any backup.')"
