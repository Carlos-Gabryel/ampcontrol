#!/usr/bin/env bash

set -Eeuo pipefail

readonly SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly PROJECT_DIRECTORY="$(cd -- "$SCRIPT_DIRECTORY/.." && pwd -P)"
TEMP_DIRECTORY="$(mktemp -d)"
trap 'rm -rf -- "$TEMP_DIRECTORY"' EXIT

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
printf 'Testes dos scripts de instalação: OK\n'

