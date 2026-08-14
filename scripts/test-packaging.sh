#!/usr/bin/env bash

set -Eeuo pipefail

readonly LANGUAGE="${AMPCONTROL_LANGUAGE:-pt-BR}"
msg() { if [[ "$LANGUAGE" == "en-US" ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi; }

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
PROJECT_DIRECTORY="$(cd -- "$SCRIPT_DIRECTORY/.." && pwd -P)"
readonly PROJECT_DIRECTORY
TEMP_DIRECTORY="$(mktemp -d)"
trap 'rm -rf -- "$TEMP_DIRECTORY"' EXIT
PYTHON_COMMAND="$(command -v python3 || command -v python || true)"
[[ -n "$PYTHON_COMMAND" ]] || {
    printf '%s\n' "$(msg 'Falha: Python 3 não encontrado' 'Failure: Python 3 was not found')" >&2
    exit 1
}

fail() {
    printf '%s: %s\n' "$(msg 'Falha' 'Failure')" "$1" >&2
    exit 1
}

assert_last_call() {
    local expected="$1"
    local actual=""
    actual="$(tail -n 1 "$TEMP_DIRECTORY/calls.log")"
    [[ "$actual" == "$expected" ]] || fail "$(msg 'esperado' 'expected') '$expected', $(msg 'recebido' 'received') '$actual'"
}

expect_failure() {
    if "$@" >"$TEMP_DIRECTORY/rejected.out" 2>&1; then
        fail "$(msg 'o comando deveria ter sido recusado' 'the command should have been rejected'): $*"
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
"$PROJECT_DIRECTORY/scripts/install.sh" --help | grep -q -- '--language'
"$PROJECT_DIRECTORY/scripts/install.sh" --language en-US --help | grep -q -- 'installs an already compiled binary'
AMPCONTROL_LANGUAGE=pt-BR "$PROJECT_DIRECTORY/scripts/migrate-legacy.sh" --help | grep -q -- '--binary CAMINHO'
"$PROJECT_DIRECTORY/scripts/migrate-legacy.sh" --language en-US --help | grep -q -- 'legacy installation'
"$PROJECT_DIRECTORY/scripts/migration-preflight.sh" --language en-US --help | grep -q -- '^Usage:'
"$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'rollback'
AMPCONTROL_LANGUAGE=en-US "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'latest stable GitHub release'
AMPCONTROL_LANGUAGE=en-US "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start Missing01 >"$TEMP_DIRECTORY/english-error.out" 2>&1 || true
grep -q "instance 'Missing01' does not exist" "$TEMP_DIRECTORY/english-error.out" || fail "$(msg 'mensagem em inglês do wrapper não foi aplicada' 'the wrapper English message was not applied')"

cat > "$TEMP_DIRECTORY/legacy.env" <<'EOF'
DISCORD_TOKEN="token sem execução"
AMP_PASSWORD='senha # preservada'
RCON_PASSWORD=segredo-rcon
EOF
[[ "$("$PYTHON_COMMAND" "$PROJECT_DIRECTORY/scripts/legacy_config.py" env "$TEMP_DIRECTORY/legacy.env" AMP_PASSWORD)" == 'senha # preservada' ]] || fail "$(msg 'parser seguro de .env alterou o valor' 'the safe .env parser changed the value')"
printf 'INVALID\n' >"$TEMP_DIRECTORY/invalid.env"
AMPCONTROL_LANGUAGE=en-US "$PYTHON_COMMAND" "$PROJECT_DIRECTORY/scripts/legacy_config.py" env "$TEMP_DIRECTORY/invalid.env" TEST >"$TEMP_DIRECTORY/python-english.out" 2>&1 || true
grep -q 'Error: line 1 of .env' "$TEMP_DIRECTORY/python-english.out" || fail "$(msg 'erro em inglês do conversor não foi aplicado' 'the converter English error was not applied')"

cat > "$TEMP_DIRECTORY/systemd-environment" <<'EOF'
HOME=/home/ampcontrol "AMP_PUBLIC_URL=http://192.168.1.22:8080" AMP_GAME_SERVER_ADDRESS=192.168.1.22
EOF
[[ "$("$PYTHON_COMMAND" "$PROJECT_DIRECTORY/scripts/legacy_config.py" systemd-env "$TEMP_DIRECTORY/systemd-environment" AMP_PUBLIC_URL)" == 'http://192.168.1.22:8080' ]] || fail "$(msg 'ambiente legado do systemd não foi interpretado' 'the legacy systemd environment was not parsed')"

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
grep -q '"password_credential": "rcon_Game_01"' "$TEMP_DIRECTORY/migrated-idle.json" || fail "$(msg 'credencial RCON não foi convertida' 'RCON credential was not converted')"
! grep -q 'password_env' "$TEMP_DIRECTORY/migrated-idle.json" || fail "$(msg 'referência legada RCON permaneceu no JSON' 'legacy RCON reference remained in JSON')"
grep -q $'^rcon_Game_01\tRCON_PASSWORD$' "$TEMP_DIRECTORY/manifest" || fail "$(msg 'manifesto RCON inválido' 'invalid RCON manifest')"
printf '%s\n' "$(msg 'Testes dos scripts de instalação: OK' 'Installation script tests: OK')"

