#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
PROJECT_DIRECTORY="$(cd -- "$SCRIPT_DIRECTORY/.." && pwd -P)"
readonly PROJECT_DIRECTORY
TEMP_DIRECTORY="$(mktemp -d)"
trap 'rm -rf -- "$TEMP_DIRECTORY"' EXIT
PYTHON_COMMAND="$(command -v python3 || command -v python || true)"
[[ -n "$PYTHON_COMMAND" ]] || {
    printf 'Falha: Python 3 não encontrado\n' >&2
    exit 1
}

fail() {
    printf 'Falha: %s\n' "$1" >&2
    exit 1
}

assert_last_call() {
    local expected="$1"
    local actual=""
    actual="$(tail -n 1 "$TEMP_DIRECTORY/calls.log")"
    [[ "$actual" == "$expected" ]] || fail "esperado '$expected', recebido '$actual'"
}

expect_failure() {
    if "$@" >"$TEMP_DIRECTORY/rejected.out" 2>&1; then
        fail "o comando deveria ter sido recusado: $*"
    fi
}

mkdir -p "$TEMP_DIRECTORY/instances/Game01" "$TEMP_DIRECTORY/instances/ADS01"
cat >"$TEMP_DIRECTORY/ampinstmgr" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$AMPCONTROL_TEST_CALLS"
EOF
chmod 0755 "$TEMP_DIRECTORY/ampinstmgr"

export AMPINSTMGR="$TEMP_DIRECTORY/ampinstmgr"
export INSTANCES_DIRECTORY="$TEMP_DIRECTORY/instances"
export AMPCONTROL_TEST_CALLS="$TEMP_DIRECTORY/calls.log"

"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" list
assert_last_call "--ShowInstancesList"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" status
assert_last_call "status"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start Game01
assert_last_call "--StartInstance Game01"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" stop Game01
assert_last_call "--StopInstance Game01"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" restart Game01
assert_last_call "--RestartInstance Game01"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" update Game01
assert_last_call "--UpgradeInstance Game01 true"

expect_failure "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start ADS01
expect_failure "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start ../Game01
expect_failure "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start Missing01
expect_failure "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start Game01 extra
expect_failure "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" unknown

"$PROJECT_DIRECTORY/scripts/install.sh" --help | grep -q -- '--binary CAMINHO'
"$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'rollback'

cat > "$TEMP_DIRECTORY/legacy.env" <<'EOF'
DISCORD_TOKEN="token sem execução"
AMP_PASSWORD='senha # preservada'
RCON_PASSWORD=segredo-rcon
EOF
[[ "$("$PYTHON_COMMAND" "$PROJECT_DIRECTORY/scripts/legacy_config.py" env "$TEMP_DIRECTORY/legacy.env" AMP_PASSWORD)" == 'senha # preservada' ]] || fail "parser seguro de .env alterou o valor"

cat > "$TEMP_DIRECTORY/legacy-idle.json" <<'EOF'
{
  "check_interval_seconds": 30,
  "servers": [{
    "instance": "Game 01",
    "enabled": true,
    "detector": "amp_players",
    "fallback_detector": "palworld_rcon",
    "rcon": {"address": "127.0.0.1:25575", "password_env": "RCON_PASSWORD"}
  }]
}
EOF
"$PYTHON_COMMAND" "$PROJECT_DIRECTORY/scripts/legacy_config.py" migrate-idle \
    "$TEMP_DIRECTORY/legacy-idle.json" "$TEMP_DIRECTORY/migrated-idle.json" "$TEMP_DIRECTORY/manifest"
grep -q '"password_credential": "rcon_Game_01"' "$TEMP_DIRECTORY/migrated-idle.json" || fail "credencial RCON não foi convertida"
! grep -q 'password_env' "$TEMP_DIRECTORY/migrated-idle.json" || fail "referência legada RCON permaneceu no JSON"
grep -q $'^rcon_Game_01\tRCON_PASSWORD$' "$TEMP_DIRECTORY/manifest" || fail "manifesto RCON inválido"
printf 'Testes dos scripts de instalação: OK\n'

