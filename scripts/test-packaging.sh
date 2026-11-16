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

# ID da aplicação extraído do token do Discord (instalador).
# shellcheck disable=SC1090
source <(sed -n '/^discord_application_id_from_token()/,/^}/p' "$PROJECT_DIRECTORY/scripts/install.sh")
[[ "$(discord_application_id_from_token 'MTIzNDU2Nzg5MDEyMzQ1Njc4.x.y')" == "123456789012345678" ]] ||
    fail "$(msg 'ID da aplicação não extraído do token' 'application ID not derived from the token')"
UNPADDED_ID="$(printf 1234567890123456789 | base64 | tr -d '=')"
[[ "$(discord_application_id_from_token "$UNPADDED_ID.x.y")" == "1234567890123456789" ]] ||
    fail "$(msg 'ID da aplicação sem padding não extraído' 'unpadded application ID not derived')"
[[ -z "$(discord_application_id_from_token 'abc.def')" ]] ||
    fail "$(msg 'token inválido deveria não gerar ID' 'invalid token should yield no ID')"

# Validação do token do bot (instalador).
# shellcheck disable=SC1090
source <(sed -n '/^valid_discord_bot_token()/,/^}/p' "$PROJECT_DIRECTORY/scripts/install.sh")
valid_discord_bot_token 'MTIzNDU2Nzg5MDEyMzQ1Njc4.fake-first.token_1234' ||
    fail "$(msg 'token de bot válido foi rejeitado' 'a valid bot token was rejected')"
for bad_token in '9f8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c' "$(printf 'a1%.0s' {1..32})" 'abc.def' \
    'MTIzNDU2Nzg5MDEyMzQ1Njc4..x' 'MTIzNDU2Nzg5MDEyMzQ1Njc4.only-two'; do
    ! valid_discord_bot_token "$bad_token" ||
        fail "$(msg 'valor que não é token de bot foi aceito' 'a non-bot-token value was accepted')"
done

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

# O controlador ADS é protegido pelo módulo registrado no instances.json do
# AMP, qualquer que seja o nome. Sem índice legível, valem as checagens antigas.
mkdir -p "$TEMP_DIRECTORY/instances/Controller02"
cat >"$TEMP_DIRECTORY/instances.json" <<'EOF'
[
  {"InstanceName": "ADS01", "Module": "ADS"},
  {"InstanceName": "Controller02", "Module": "ADS"},
  {"InstanceName": "Game01", "Module": "GenericModule"}
]
EOF
expect_failure "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start Controller02
expect_failure "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" update Controller02
grep -q 'ADS' "$TEMP_DIRECTORY/rejected.out" || fail "$(msg 'recusa sem citar o ADS' 'rejection does not mention ADS')"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" start Game01
assert_last_call "--StartInstance Game01"
printf '{ quebrado' >"$TEMP_DIRECTORY/instances.json"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" restart Game01
assert_last_call "--RestartInstance Game01"
rm -f "$TEMP_DIRECTORY/instances.json"
"$PROJECT_DIRECTORY/scripts/ampcontrol-amp" stop Controller02
assert_last_call "--StopInstance Controller02"

"$PROJECT_DIRECTORY/scripts/install.sh" --help | grep -q -- '--binary CAMINHO'
"$PROJECT_DIRECTORY/scripts/install.sh" --help | grep -q -- '--language'
"$PROJECT_DIRECTORY/scripts/install.sh" --language en-US --help | grep -q -- 'installs an already compiled binary'

# prompt_secret é capturado com $(...): nada além do segredo pode sair no
# stdout, senão token e senha são gravados corrompidos.
eval "$(sed -n '/^prompt_secret() {/,/^}/p' "$PROJECT_DIRECTORY/scripts/install.sh")"
captured_secret="$(printf 'segredo com espaço\nsegredo com espaço\n' | prompt_secret 'Token' 2>/dev/null)"
[[ "$captured_secret" == 'segredo com espaço' ]] || fail "$(msg 'prompt_secret alterou o segredo' 'prompt_secret changed the secret'): $(printf '%q' "$captured_secret")"

extract_stable_service_check() {
    sed -n '/^wait_for_stable_service() {/,/^}/p' "$1"
}
[[ -n "$(extract_stable_service_check "$PROJECT_DIRECTORY/scripts/install.sh")" ]] || fail "$(msg 'wait_for_stable_service ausente no instalador' 'wait_for_stable_service missing from the installer')"
for script in migrate-legacy.sh ampcontrol-maintenance; do
    [[ "$(extract_stable_service_check "$PROJECT_DIRECTORY/scripts/$script")" == "$(extract_stable_service_check "$PROJECT_DIRECTORY/scripts/install.sh")" ]] ||
        fail "$(msg 'wait_for_stable_service divergiu em' 'wait_for_stable_service diverged in') $script"
done
eval "$(extract_stable_service_check "$PROJECT_DIRECTORY/scripts/install.sh")"
# systemctl falso: o estado e o NRestarts vêm de listas, uma posição por consulta.
# Os stubs e as listas FAKE_* são usados pela função carregada no eval acima.
# shellcheck disable=SC2034,SC2317,SC2329
{
    sleep() { :; }
    systemctl() {
        local property="${3#--property=}"
        local -n values="FAKE_$property"
        local index_file="$TEMP_DIRECTORY/index.$property"
        local index=0
        [[ ! -f "$index_file" ]] || index="$(<"$index_file")"
        printf '%s' "${values[index]:-${values[-1]}}"
        printf '%s' "$((index + 1))" >"$index_file"
    }
    run_stable_service_check() {
        rm -f -- "$TEMP_DIRECTORY"/index.*
        AMPCONTROL_START_CHECK_SECONDS=5 wait_for_stable_service
    }
    FAKE_ActiveState=(active)
    FAKE_NRestarts=(0)
    run_stable_service_check || fail "$(msg 'serviço estável foi recusado' 'a stable service was rejected')"
    FAKE_ActiveState=(active)
    FAKE_NRestarts=(0 0 0 1)
    ! run_stable_service_check || fail "$(msg 'serviço em loop de reinício foi aceito' 'a service in a restart loop was accepted')"
    FAKE_ActiveState=(active active activating)
    FAKE_NRestarts=(0)
    ! run_stable_service_check || fail "$(msg 'serviço que caiu foi aceito' 'a service that went down was accepted')"
}
unset -f sleep systemctl

# Checagem de versão mínima do systemd, com systemctl falso em subshell.
systemd_check="$(sed -n '/^command -v systemctl /,/^fi$/p' "$PROJECT_DIRECTORY/scripts/install.sh")"
[[ "$systemd_check" == *'SYSTEMD_VERSION < 250'* ]] || fail "$(msg 'checagem de versão do systemd ausente' 'systemd version check missing')"
for version_case in '249:false' '250:true' '255:true' 'x:false'; do
    expected="${version_case#*:}"
    actual=true
    # shellcheck disable=SC2317,SC2329
    (
        systemctl() { printf 'systemd %s (fake)\n' "${version_case%%:*}"; }
        eval "$systemd_check"
    ) >/dev/null 2>&1 || actual=false
    [[ "$actual" == "$expected" ]] || fail "$(msg 'checagem do systemd errada para' 'wrong systemd check for') ${version_case%%:*}"
done

AMPCONTROL_LANGUAGE=pt-BR "$PROJECT_DIRECTORY/scripts/migrate-legacy.sh" --help | grep -q -- '--binary CAMINHO'
"$PROJECT_DIRECTORY/scripts/migrate-legacy.sh" --language en-US --help | grep -q -- 'legacy installation'
"$PROJECT_DIRECTORY/scripts/migration-preflight.sh" --language en-US --help | grep -q -- '^Usage:'
"$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'rollback'
AMPCONTROL_LANGUAGE=en-US "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'latest stable GitHub release'
"$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'uninstall \[--purge\] \[--yes\]'
AMPCONTROL_LANGUAGE=en-US "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'never touched'
# Atualização automática: unidades empacotadas e lógica do auto-update com stubs.
for unit in ampcontrol-update.service ampcontrol-update.timer; do
    [[ -f "$PROJECT_DIRECTORY/packaging/systemd/$unit" ]] || fail "$(msg 'unit ausente' 'missing unit'): $unit"
    grep -q "$unit" "$PROJECT_DIRECTORY/scripts/build-release.sh" || fail "$(msg 'unit fora do pacote de release' 'unit not in the release package'): $unit"
    grep -q "$unit" "$PROJECT_DIRECTORY/scripts/install.sh" || fail "$(msg 'unit não instalada pelo instalador' 'unit not installed by the installer'): $unit"
done
grep -q 'ExecStart=/usr/local/sbin/ampcontrol-maintenance auto-update' "$PROJECT_DIRECTORY/packaging/systemd/ampcontrol-update.service" ||
    fail "$(msg 'ExecStart do auto-update incorreto' 'wrong auto-update ExecStart')"
grep -q 'OnCalendar=\*-\*-\* 05:00' "$PROJECT_DIRECTORY/packaging/systemd/ampcontrol-update.timer" ||
    fail "$(msg 'agenda do timer incorreta' 'wrong timer schedule')"
"$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" --help | grep -q -- 'auto-update'

(
    # shellcheck disable=SC2317,SC2034,SC2329
    {
        msg() { printf '%s' "$2"; }
        fail() { printf '%s\n' "$1" >&2; exit 1; }
        STATE_DIRECTORY="$TEMP_DIRECTORY/auto-state"
        mkdir -p "$STATE_DIRECTORY"
        BINARY_PATH="$TEMP_DIRECTORY/fake-ampcontrol"
        UPDATE_CALLS="$TEMP_DIRECTORY/update-calls"
        : > "$UPDATE_CALLS"
        update_ampcontrol() { printf '%s\n' "$1" >> "$UPDATE_CALLS"; return "${FAKE_UPDATE_STATUS:-0}"; }
        install_heartbeat_units() { :; }
        cleanup() { :; }
        ensure_state_directory() { mkdir -p "$STATE_DIRECTORY"; }
        systemctl() { [[ "$*" == 'is-enabled --quiet ampcontrol.service' ]] && return "${FAKE_SERVICE_DISABLED:-0}"; return 1; }
    }
    eval "$(sed -n '/^latest_version() {/,/^}/p;/^auto_update() {/,/^}/p' "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance")"
    set_installed() { printf '#!/usr/bin/env bash\necho "AmpControl %s"\n' "$1" > "$BINARY_PATH"; chmod +x "$BINARY_PATH"; }
    calls() { wc -l < "$UPDATE_CALLS" | tr -d ' '; }

    set_installed dev
    AMPCONTROL_LATEST_VERSION=v9.9.9 auto_update | grep -q 'not a stable release' || fail 'dev build was not skipped'
    set_installed v1.2.3
    FAKE_SERVICE_DISABLED=1 AMPCONTROL_LATEST_VERSION=v9.9.9 auto_update | grep -q 'is not enabled' || fail 'disabled service was not skipped'
    [[ "$(calls)" == 0 ]] || fail 'update ran with the service disabled'
    AMPCONTROL_LATEST_VERSION=v1.2.3 auto_update | grep -q 'already up to date' || fail 'equal version was not a no-op'
    AMPCONTROL_LATEST_VERSION=v1.2.2 auto_update | grep -q 'already up to date' || fail 'older release was not a no-op'
    AMPCONTROL_LATEST_VERSION=v1.10.0 auto_update >/dev/null
    [[ "$(calls)" == 1 && "$(<"$UPDATE_CALLS")" == v1.10.0 ]] || fail 'newer release did not trigger one update (sort -V)'
    [[ ! -e "$STATE_DIRECTORY/auto-update-failed" ]] || fail 'failure marker written on success'
    if FAKE_UPDATE_STATUS=1 AMPCONTROL_LATEST_VERSION=v1.10.1 auto_update >/dev/null 2>&1; then fail 'failed update returned success'; fi
    [[ "$(<"$STATE_DIRECTORY/auto-update-failed")" == v1.10.1 ]] || fail 'failure marker missing'
    if FAKE_UPDATE_STATUS=1 AMPCONTROL_LATEST_VERSION=v1.10.1 auto_update >/dev/null 2>&1; then fail 'second failed update returned success'; fi
    [[ "$(calls)" == 3 ]] || fail 'first failure was not retried on the next run'
    AMPCONTROL_LATEST_VERSION=v1.10.1 auto_update | grep -q 'failed twice' || fail 'twice-failed version was not skipped'
    [[ "$(calls)" == 3 ]] || fail 'twice-failed version triggered another update'
    AMPCONTROL_LATEST_VERSION=v1.10.2 auto_update >/dev/null
    [[ "$(calls)" == 4 && ! -e "$STATE_DIRECTORY/auto-update-failed" ]] || fail 'newer release after a failure was not installed'
) || fail "$(msg 'lógica do auto-update incorreta' 'auto-update logic is wrong')"

(
    # shellcheck disable=SC2317,SC2034,SC2329
    {
        msg() { printf '%s' "$2"; }
        JOURNAL_CALLS="$TEMP_DIRECTORY/journal-calls"
        service_journal_since() { printf 'x\n' >> "$JOURNAL_CALLS"; printf '%s\n' "$FAKE_JOURNAL"; }
        systemctl() { printf '%s' "${FAKE_ACTIVE:-active}"; }
        sleep() { :; }
    }
    eval "$(sed -n '/^verify_discord_health() {/,/^}/p' "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance")"
    export AMPCONTROL_HEALTH_CHECK_SECONDS=10
    health() { : > "$JOURNAL_CALLS"; verify_discord_health 0 2>/dev/null; }
    FAKE_JOURNAL='{"level":"info","message":"Discord conectado"}'
    health || fail 'healthy bot was rejected'
    FAKE_JOURNAL='{"level":"info","message":"Discord connected"}'
    health || fail 'English connected line was rejected'
    FAKE_JOURNAL='{"level":"info","message":"started"}'
    ! health || fail 'bot that never connected was accepted'
    FAKE_JOURNAL=$'{"message":"Discord conectado"}\n{"message":"Discord conectado"}'
    ! health || fail 'reconnect loop was accepted'
    FAKE_JOURNAL='ERROR error while parsing gateway message name=bot err="unknown channel with type 20"'
    ! health || fail 'gateway error was accepted'
    [[ "$(wc -l < "$JOURNAL_CALLS" | tr -d ' ')" == 1 ]] || fail 'gateway error did not fail fast'
    FAKE_JOURNAL='{"message":"Discord conectado"}'
    FAKE_ACTIVE=failed
    ! health || fail 'inactive service was accepted'
) || fail "$(msg 'verify_discord_health incorreto' 'verify_discord_health is wrong')"

(
    # shellcheck disable=SC2317,SC2034,SC2329
    {
        msg() { printf '%s' "$2"; }
        fail() { printf 'Error: %s\n' "$1" >&2; exit 1; }
        install() { :; }
        chown() { :; }
        sleep() { :; }
        wait_for_stable_service() { return "${FAKE_STABLE_STATUS:-0}"; }
        verify_discord_health() { return "${FAKE_HEALTH_STATUS:-0}"; }
        systemctl() { printf 'systemctl %s\n' "$*" >> "$SET_TOKEN_CALLS"; }
        systemd-creds() { { printf 'enc:'; cat; } > "${*: -1}"; }
        curl() { printf '%s' "${FAKE_HTTP_CODE:-200}"; return "${FAKE_CURL_STATUS:-0}"; }
    }
    eval "$(sed -n '/^discord_application_id_from_token() {/,/^}/p;/^valid_discord_bot_token() {/,/^}/p;/^discord_token_help() {/,/^}/p;/^set_token() {/,/^}/p' "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance")"
    SET_TOKEN_ROOT="$TEMP_DIRECTORY/set-token"
    SET_TOKEN_CALLS="$SET_TOKEN_ROOT/calls"
    CREDENTIAL_DIRECTORY="$SET_TOKEN_ROOT/credstore"
    mkdir -p "$CREDENTIAL_DIRECTORY"
    credential="$CREDENTIAL_DIRECTORY/ampcontrol.discord_token"
    good_token='MTIzNDU2Nzg5MDEyMzQ1Njc4.fake-first.token_1234'
    reset_credential() { printf 'enc:old' > "$credential"; rm -f -- "$credential".bak-* "$credential.new"; : > "$SET_TOKEN_CALLS"; }
    run_set_token() { ( printf '%s\n' "$1" | set_token ) >/dev/null 2>&1; }

    reset_credential
    ! run_set_token 'not-a-token' || fail 'invalid token format was accepted'
    [[ "$(<"$credential")" == enc:old && ! -s "$SET_TOKEN_CALLS" ]] || fail 'invalid format changed something'

    reset_credential
    ! FAKE_HTTP_CODE=401 run_set_token "$good_token" || fail 'token rejected by Discord was accepted'
    [[ "$(<"$credential")" == enc:old && ! -s "$SET_TOKEN_CALLS" ]] || fail 'rejected token changed something'
    compgen -G "$credential.bak-*" >/dev/null && fail 'rejected token created a backup'

    reset_credential
    ! FAKE_CURL_STATUS=6 run_set_token "$good_token" || fail 'network error was accepted'
    [[ "$(<"$credential")" == enc:old ]] || fail 'network error changed the credential'

    reset_credential
    run_set_token "  $good_token  " || fail 'valid token was rejected'
    [[ "$(<"$credential")" == "enc:$good_token" ]] || fail 'credential was not replaced'
    [[ "$(cat "$credential".bak-*)" == enc:old ]] || fail 'backup of the old credential is missing'
    grep -q 'systemctl restart ampcontrol.service' "$SET_TOKEN_CALLS" || fail 'service was not restarted'

    reset_credential
    ! FAKE_HEALTH_STATUS=1 run_set_token "$good_token" || fail 'failed health check returned success'
    [[ "$(<"$credential")" == enc:old ]] || fail 'old credential was not restored after health failure'
    [[ "$(grep -c 'systemctl restart' "$SET_TOKEN_CALLS")" == 2 ]] || fail 'service was not restarted after restoring'
) || fail "$(msg 'set-token incorreto' 'set-token is wrong')"

# Heartbeat: leitura segura da configuração, estado do serviço, sinal de vida e POST com stubs.
(
    # shellcheck disable=SC2317,SC2034,SC2329
    {
        msg() { printf '%s' "$2"; }
        HB_ROOT="$TEMP_DIRECTORY/heartbeat"
        mkdir -p "$HB_ROOT"
        HEARTBEAT_ENV_PATH="$HB_ROOT/heartbeat.env"
        HEARTBEAT_FILE="$HB_ROOT/discord-heartbeat"
        HEARTBEAT_MAX_AGE=180
        TEMP_DIRECTORY="$HB_ROOT/tmp"
        mkdir -p "$TEMP_DIRECTORY"
        CURL_ARGS="$HB_ROOT/curl-args"
        CURL_BODY="$HB_ROOT/curl-body"
        CURL_HEADER="$HB_ROOT/curl-header"
        systemctl() {
            case "$*" in
                *ActiveState*) printf '%s\n' "${FAKE_STATE:-active}" ;;
                *ExecMainStatus*) printf '%s\n' "${FAKE_EXIT:-0}" ;;
            esac
        }
        date() { printf '%s\n' "${FAKE_NOW:-1000000}"; }
        curl() {
            local previous="" argument
            : > "$CURL_ARGS"
            for argument in "$@"; do
                printf '%s\n' "$argument" >> "$CURL_ARGS"
                [[ "$previous" == --data ]] && printf '%s' "$argument" > "$CURL_BODY"
                [[ "$previous" == -H && "$argument" == @* ]] && cp -- "${argument#@}" "$CURL_HEADER"
                previous="$argument"
            done
            printf '%s' "${FAKE_HTTP_CODE:-204}"
            return "${FAKE_CURL_STATUS:-0}"
        }
    }
    eval "$(sed -n '/^read_heartbeat_config() {/,/^}/p;/^heartbeat() {/,/^}/p' "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance")"
    secret='s3cr3t-hb-token'
    printf 'HEARTBEAT_URL=https://example.test/hook\nHEARTBEAT_TOKEN="%s"\n$(touch %s/pwned)\n' "$secret" "$HB_ROOT" > "$HEARTBEAT_ENV_PATH"
    send() { rm -f -- "$CURL_ARGS" "$CURL_BODY"; heartbeat 2>"$HB_ROOT/stderr"; }

    rm -f -- "$HEARTBEAT_ENV_PATH"
    send || fail 'missing config should exit 0'
    [[ ! -e "$CURL_ARGS" ]] || fail 'POST sent without configuration'
    printf 'HEARTBEAT_URL=https://example.test/hook\nHEARTBEAT_TOKEN="%s"\n$(touch %s/pwned)\n' "$secret" "$HB_ROOT" > "$HEARTBEAT_ENV_PATH"
    [[ ! -e "$HB_ROOT/pwned" ]] || fail 'config file was executed'

    FAKE_STATE=inactive send >/dev/null || fail 'inactive service heartbeat failed'
    grep -q '"status": "error"' "$CURL_BODY" && grep -q 'inactive' "$CURL_BODY" || fail 'inactive service was not reported as error'
    FAKE_STATE=failed FAKE_EXIT=78 send >/dev/null || fail 'token failure heartbeat failed'
    grep -q 'set-token' "$CURL_BODY" || fail 'exit status 78 did not mention set-token'

    rm -f -- "$HEARTBEAT_FILE"
    send >/dev/null || fail 'never connected heartbeat failed'
    grep -q '"status": "error"' "$CURL_BODY" || fail 'missing heartbeat file was not an error'
    printf '%s\n' $((1000000 - 600)) > "$HEARTBEAT_FILE"
    send >/dev/null || fail 'stale heartbeat send failed'
    grep -q '"status": "error"' "$CURL_BODY" && grep -q '10 min' "$CURL_BODY" || fail 'stale heartbeat file was not an error'
    printf '%s\n' $((1000000 - 30)) > "$HEARTBEAT_FILE"
    send >/dev/null || fail 'fresh heartbeat send failed'
    grep -q '"status": "ok"' "$CURL_BODY" && grep -q '"message": ""' "$CURL_BODY" || fail 'fresh heartbeat was not ok'
    grep -qx 'https://example.test/hook' "$CURL_ARGS" || fail 'URL was not passed to curl'
    grep -qx 'Content-Type: application/json' "$CURL_ARGS" || fail 'Content-Type missing'
    [[ "$(<"$CURL_HEADER")" == "Authorization: Bearer $secret" ]] || fail 'Authorization header file is wrong'
    ! grep -q -- "$secret" "$CURL_ARGS" "$CURL_BODY" || fail 'token leaked into curl argv or body'

    FAKE_HTTP_CODE=500 send >/dev/null 2>&1 && fail 'non-2xx was accepted'
    FAKE_CURL_STATUS=7 send >/dev/null 2>&1 && fail 'curl failure was accepted'
    [[ -s "$HB_ROOT/stderr" ]] || fail 'failure printed nothing to stderr'
    exit 0
) || fail "$(msg 'heartbeat incorreto' 'heartbeat is wrong')"

# Units do heartbeat embutidas: só reescreve se mudou e nunca liga o timer.
(
    # shellcheck disable=SC2317,SC2034,SC2329
    {
        HEARTBEAT_SERVICE_PATH="$TEMP_DIRECTORY/units/ampcontrol-heartbeat.service"
        HEARTBEAT_TIMER_PATH="$TEMP_DIRECTORY/units/ampcontrol-heartbeat.timer"
        mkdir -p "$TEMP_DIRECTORY/units"
        SYSTEMCTL_CALLS="$TEMP_DIRECTORY/units/calls"
        : > "$SYSTEMCTL_CALLS"
        systemctl() { printf '%s\n' "$*" >> "$SYSTEMCTL_CALLS"; }
    }
    eval "$(sed -n '/^install_heartbeat_units() {/,/^}/p' "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance")"
    install_heartbeat_units
    grep -q 'ExecStart=/usr/local/sbin/ampcontrol-maintenance heartbeat$' "$HEARTBEAT_SERVICE_PATH" || fail 'service ExecStart wrong'
    grep -q 'Type=oneshot' "$HEARTBEAT_SERVICE_PATH" || fail 'service is not oneshot'
    grep -q 'OnCalendar=\*:0/15' "$HEARTBEAT_TIMER_PATH" || fail 'timer schedule wrong'
    [[ "$(<"$SYSTEMCTL_CALLS")" == daemon-reload ]] || fail 'units were not reloaded exactly once'
    install_heartbeat_units
    [[ "$(wc -l < "$SYSTEMCTL_CALLS" | tr -d ' ')" == 1 ]] || fail 'unchanged units triggered a reload'
    ! grep -q enable "$SYSTEMCTL_CALLS" || fail 'timer was enabled automatically'
) || fail "$(msg 'units do heartbeat incorretas' 'heartbeat units are wrong')"

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

