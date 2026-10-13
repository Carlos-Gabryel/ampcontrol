#!/usr/bin/env bash
# Teste de fumaça do binário linux_arm64: confere --version e --check-config.
# Em CI roda sob emulação QEMU; localmente aceita um binário nativo.
#
# Uso: scripts/test-arm64.sh CAMINHO_DO_BINÁRIO [EXECUTOR]
#   EXECUTOR: prefixo opcional, por exemplo qemu-aarch64-static

set -euo pipefail

if (($# < 1 || $# > 2)); then
    printf 'Uso: %s CAMINHO_DO_BINÁRIO [EXECUTOR]\n' "$0" >&2
    exit 2
fi

SCRIPT_DIRECTORY="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPOSITORY="$(dirname "$SCRIPT_DIRECTORY")"
BINARY="$(realpath "$1")"
RUNNER=()
if (($# == 2)); then RUNNER=("$2"); fi
readonly EXPECTED_VERSION="${EXPECTED_VERSION:-ci-arm64}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# --version deve mostrar a versão injetada no build.
output="$("${RUNNER[@]}" "$BINARY" --version)"
printf '%s\n' "$output"
[[ "$output" == *"$EXPECTED_VERSION"* ]] ||
    { printf 'versão inesperada: %s\n' "$output" >&2; exit 1; }

# Configuração mínima: o exemplo com IDs falsos de 18 dígitos.
mkdir -p "$WORK/config"
cp "$REPOSITORY/config/idle.json" "$WORK/config/idle.json"
sed -E \
    -e 's/^guild_id = .*/guild_id = "111111111111111111"/' \
    -e 's/^notification_channel_id = .*/notification_channel_id = "222222222222222222"/' \
    -e 's/^audit_channel_id = .*/audit_channel_id = "333333333333333333"/' \
    -e 's/^owner_user_id = .*/owner_user_id = "444444444444444444"/' \
    "$REPOSITORY/config/ampcontrol.example.toml" >"$WORK/config.toml"

# --check-config fora do systemd: segredos pelas variáveis legadas.
(cd "$WORK" && env -i PATH="$PATH" \
    AMPCONTROL_CONFIG="$WORK/config.toml" \
    DISCORD_TOKEN='MTIzNDU2Nzg5MDEyMzQ1Njc4.fake.token_1234' \
    AMP_PASSWORD='senha-falsa' \
    "${RUNNER[@]}" "$BINARY" --check-config)
printf 'OK: --version e --check-config passaram\n'
