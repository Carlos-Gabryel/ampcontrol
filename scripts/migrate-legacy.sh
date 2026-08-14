#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
LEGACY_DIRECTORY="/opt/ampcontrol"
BINARY_PATH=""

fail() {
    printf 'Erro: %s\n' "$1" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Uso: sudo ./scripts/migrate-legacy.sh [DIRETÓRIO] [--binary CAMINHO]

  DIRETÓRIO        instalação legada (padrão: /opt/ampcontrol)
  --binary CAMINHO usa um binário Linux já compilado
EOF
}

while (($# > 0)); do
    case "$1" in
        --binary)
            (($# >= 2)) || fail "--binary exige um caminho"
            BINARY_PATH="$2"
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        -* )
            fail "opção desconhecida: $1"
            ;;
        *)
            [[ "$LEGACY_DIRECTORY" == "/opt/ampcontrol" ]] || fail "informe somente um diretório legado"
            LEGACY_DIRECTORY="$1"
            shift
            ;;
    esac
done

readonly LEGACY_DIRECTORY BINARY_PATH
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
    printf '\nFalha detectada. Restaurando o estado anterior...\n' >&2
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
    printf 'Rollback concluído. Backup preservado em %s\n' "$BACKUP_DIRECTORY" >&2
}

on_error() {
    local status=$?
    if [[ "$ROLLBACK_ARMED" == true ]]; then
        restore_snapshot
    fi
    exit "$status"
}
trap on_error ERR

[[ "$(uname -s)" == Linux ]] || fail "a migração oferece suporte apenas a Linux"
[[ "$EUID" -eq 0 ]] || fail "execute com sudo"
[[ -t 0 ]] || fail "a migração precisa de um terminal interativo"
[[ "$LEGACY_DIRECTORY" == /* && -d "$LEGACY_DIRECTORY" ]] || fail "instalação legada não encontrada em $LEGACY_DIRECTORY"
[[ -f "$LEGACY_DIRECTORY/.env" ]] || fail "arquivo .env legado não encontrado"
[[ -x "$SCRIPT_DIRECTORY/install.sh" ]] || fail "install.sh não está executável"

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
printf 'Backup transacional criado em %s\n' "$BACKUP_DIRECTORY"
INSTALL_ARGUMENTS=(--no-start --migrate-legacy "$LEGACY_DIRECTORY")
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
    printf 'O novo serviço não permaneceu ativo.\n' >&2
    false
fi

ROLLBACK_ARMED=false
printf '\nMigração concluída. A instalação legada foi preservada em %s.\n' "$LEGACY_DIRECTORY"
printf 'Backup para rollback: %s\n' "$BACKUP_DIRECTORY"
printf 'Valide o Discord e os detectores RCON antes de remover qualquer backup.\n'
