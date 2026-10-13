#!/usr/bin/env bash
# Ensaio de ponta a ponta do instalador, como um usuário novo faria. Roda como
# root dentro de um container descartável com systemd (packaging/test/
# installer.Dockerfile); nunca execute em uma máquina real.
#
# Uso: scripts/test-installer.sh CAMINHO_DO_BINÁRIO

set -Eeuo pipefail

readonly LANGUAGE="${AMPCONTROL_LANGUAGE:-pt-BR}"
msg() { if [[ "$LANGUAGE" == "en-US" ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi; }

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
readonly BINARY="${1:?$(msg 'informe o caminho do binário' 'provide the binary path')}"

fail() {
    printf '%s: %s\n' "$(msg 'Falha' 'Failure')" "$1" >&2
    exit 1
}

[[ "$EUID" -eq 0 ]] || fail "$(msg 'execute como root dentro do container de teste' 'run as root inside the test container')"
[[ -f /.dockerenv || "${AMPCONTROL_INSTALLER_TEST:-}" == 1 ]] ||
    fail "$(msg 'este ensaio altera o sistema; use somente no container de teste' 'this rehearsal changes the system; use it only in the test container')"

[[ ! -e /etc/ampcontrol ]] ||
    fail "$(msg 'o ensaio precisa de um container novo, sem instalação anterior' 'the rehearsal needs a fresh container without a previous installation')"

# Formato de token real: base64 do ID da aplicação, como o disgo exige.
readonly TOKEN_FIRST='MTIzNDU2Nzg5MDEyMzQ1Njc4.fake-first.token_1234'
readonly TOKEN_SECOND='MTIzNDU2Nzg5MDEyMzQ1Njc4.fake-second.token_5678'
readonly AMP_PASSWORD='senha com espaço #1'
readonly GUILD_ID='111111111111111111'
readonly PANEL_ID='222222222222222222'
readonly AUDIT_ID='333333333333333333'
readonly OWNER_ID='444444444444444444'

# AMP falso: usuário, instâncias e um ampinstmgr que só lista.
id amp >/dev/null 2>&1 || useradd --create-home --shell /bin/bash amp
install -d -o amp -g amp /home/amp/.ampdata/instances/ADS01 \
    /home/amp/.ampdata/instances/Palworld01 /home/amp/.ampdata/instances/Valheim01
cat >/usr/bin/ampinstmgr <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "--ShowInstancesList" ]]; then
    for directory in "$HOME"/.ampdata/instances/*/; do
        name="$(basename "$directory")"
        printf 'Instance Name │ %s\nModule        │ GenericModule\nRunning       │ Yes\n\n' "$name"
    done
    exit 0
fi
printf 'ampinstmgr falso: %s\n' "$*" >&2
exit 1
EOF
chmod 0755 /usr/bin/ampinstmgr

# Sem o segundo parâmetro, usa o install.sh do repositório com --binary. Com ele
# (caminho do install.sh), os demais parâmetros são as opções extras exatas.
run_installer() {
    local answers="$1"
    local script_path="$SCRIPT_DIRECTORY/install.sh"
    local -a extra=(--binary "$BINARY")
    if (($# >= 2)); then
        script_path="$2"
        extra=("${@:3}")
    fi
    local command_line
    command_line="$(printf '%q ' "$script_path" --language pt-BR "${extra[@]}" --no-start)"
    printf '%s\n' "$answers" | script -qec "$command_line" /dev/null | tee "${INSTALLER_LOG:-/dev/null}"
}

discord_and_amp_answers() {
    local token="$1"
    printf '%s\n' "$GUILD_ID" "$PANEL_ID" "$AUDIT_ID" "$OWNER_ID" '' '' '' \
        '' '' 'http://amp.example.invalid:8080' 'jogos.example.invalid' \
        "$token" "$token" "$AMP_PASSWORD" "$AMP_PASSWORD" 's'
}

decrypted() {
    systemd-creds decrypt --name="$1" "/etc/credstore.encrypted/ampcontrol.$1" -
}

assert_secret() {
    local name="$1"
    local expected="$2"
    local actual
    actual="$(decrypted "$name" | od -An -c | tr -s ' ')"
    [[ "$actual" == "$(printf '%s' "$expected" | od -An -c | tr -s ' ')" ]] ||
        fail "$(msg 'credencial gravada com conteúdo diferente do digitado' 'credential stored with content different from the input'): $name:$actual"
}

check_installation() {
    local token="$1"
    assert_secret discord_token "$token"
    assert_secret amp_password "$AMP_PASSWORD"

    grep -qx "guild_id = \"$GUILD_ID\"" /etc/ampcontrol/config.toml || fail "guild_id"
    grep -qx 'allow_discord_administrators = false' /etc/ampcontrol/config.toml || fail "allow_discord_administrators"
    grep -q '"instance": "Palworld01"' /var/lib/ampcontrol/data/idle_servers.json || fail "idle_servers.json"
    ! grep -q 'Valheim01' /var/lib/ampcontrol/data/idle_servers.json || fail "$(msg 'instância não escolhida no Idle' 'instance not chosen for Idle')"

    systemd-analyze verify /etc/systemd/system/ampcontrol.service || fail "systemd-analyze verify"
    visudo -cf /etc/sudoers.d/ampcontrol >/dev/null || fail "sudoers"
    local inventory
    inventory="$(sudo -u ampcontrol sudo -n -u amp /usr/local/bin/ampcontrol-amp list)"
    [[ "$inventory" == *'Palworld01'* ]] ||
        fail "$(msg 'o usuário do serviço não consegue usar o wrapper' 'the service user cannot use the wrapper')"
    ! sudo -u ampcontrol sudo -n -u amp /bin/true 2>/dev/null ||
        fail "$(msg 'o sudoers permite mais que o wrapper' 'sudoers allows more than the wrapper')"

    # O wrapper recusa um amp-runtime.conf que outro usuário possa alterar.
    local runtime_config=/etc/ampcontrol/amp-runtime.conf
    for unsafe in 'chmod 0664' 'chmod 0646' 'chown amp'; do
        $unsafe "$runtime_config"
        ! sudo -u ampcontrol sudo -n -u amp /usr/local/bin/ampcontrol-amp list >/dev/null 2>&1 ||
            fail "$(msg 'o wrapper aceitou um amp-runtime.conf inseguro' 'the wrapper accepted an unsafe amp-runtime.conf'): $unsafe"
        chown root:root "$runtime_config"
        chmod 0644 "$runtime_config"
    done
    chmod g+w /etc/ampcontrol
    ! sudo -u ampcontrol sudo -n -u amp /usr/local/bin/ampcontrol-amp list >/dev/null 2>&1 ||
        fail "$(msg 'o wrapper aceitou /etc/ampcontrol gravável pelo grupo' 'the wrapper accepted a group-writable /etc/ampcontrol')"
    chmod g-w /etc/ampcontrol
    sudo -u ampcontrol sudo -n -u amp /usr/local/bin/ampcontrol-amp list >/dev/null ||
        fail "$(msg 'o wrapper não voltou a funcionar com as permissões corretas' 'the wrapper did not recover with correct permissions')"

    # --check-config fora do systemd: segredos pelas variáveis legadas.
    (cd /var/lib/ampcontrol && sudo -u ampcontrol env -i PATH=/usr/bin:/bin \
        AMPCONTROL_CONFIG=/etc/ampcontrol/config.toml DISCORD_TOKEN="$token" AMP_PASSWORD="$AMP_PASSWORD" \
        /usr/lib/ampcontrol/ampcontrol --check-config) ||
        fail "$(msg 'o binário recusou a configuração gerada pelo instalador' 'the binary rejected the configuration generated by the installer')"
}

printf '== %s\n' "$(msg 'Instalação nova' 'Fresh installation')"
INSTALLER_LOG="$(mktemp)"
run_installer "$(printf '1\n'; discord_and_amp_answers "$TOKEN_FIRST")"
EXPECTED_INVITE="https://discord.com/oauth2/authorize?client_id=123456789012345678&scope=bot+applications.commands&permissions=125952&guild_id=${GUILD_ID}&disable_guild_select=true"
grep -qF "$EXPECTED_INVITE" "$INSTALLER_LOG" ||
    fail "$(msg 'o instalador não mostrou o link de convite esperado' 'the installer did not show the expected invite link')"
if grep -qF "$TOKEN_FIRST" "$INSTALLER_LOG"; then
    fail "$(msg 'o instalador imprimiu o token' 'the installer printed the token')"
fi
rm -f -- "$INSTALLER_LOG"
unset INSTALLER_LOG
check_installation "$TOKEN_FIRST"

printf '== %s\n' "$(msg 'Reexecução para trocar o token, mantendo o Idle' 'Re-run to rotate the token, keeping Idle')"
run_installer "$(printf '\n'; discord_and_amp_answers "$TOKEN_SECOND")"
check_installation "$TOKEN_SECOND"

printf '== %s\n' "$(msg 'Reinstalação a partir do pacote de release, sem Go e sem --binary' 'Reinstall from the release package, without Go or --binary')"
! command -v go >/dev/null 2>&1 ||
    fail "$(msg 'Go está instalado; o cenário do pacote de release perderia o sentido' 'Go is installed; the release package scenario would be meaningless')"
PACKAGE_DIRECTORY="$(mktemp -d)"
RELEASE_DIRECTORY="$(mktemp -d)"
BAD_RELEASE_DIRECTORY="$(mktemp -d)"
trap 'rm -rf -- "$PACKAGE_DIRECTORY" "$RELEASE_DIRECTORY" "$BAD_RELEASE_DIRECTORY"' EXIT
# Pacote de release: scripts, packaging, config, docs e o binário; sem go.mod.
for entry in scripts packaging config docs; do
    cp -r "$SCRIPT_DIRECTORY/../$entry" "$PACKAGE_DIRECTORY/$entry"
done
install -m 0755 "$BINARY" "$PACKAGE_DIRECTORY/ampcontrol"
run_installer "$(printf '\n'; discord_and_amp_answers "$TOKEN_FIRST")" "$PACKAGE_DIRECTORY/scripts/install.sh"
check_installation "$TOKEN_FIRST"
cmp "$PACKAGE_DIRECTORY/ampcontrol" /usr/lib/ampcontrol/ampcontrol ||
    fail "$(msg 'o binário instalado não é o do pacote de release' 'the installed binary is not the release package binary')"

printf '== %s\n' "$(msg 'Instalação com um comando (get.sh) a partir de uma release local' 'One-command install (get.sh) from a local release')"
RELEASE_VERSION=v0.0.0-test
RELEASE_PACKAGE="ampcontrol_${RELEASE_VERSION}_linux_amd64"
mkdir -p "$RELEASE_DIRECTORY/$RELEASE_VERSION" "$RELEASE_DIRECTORY/stage"
cp -r "$PACKAGE_DIRECTORY" "$RELEASE_DIRECTORY/stage/$RELEASE_PACKAGE"
tar -C "$RELEASE_DIRECTORY/stage" -czf "$RELEASE_DIRECTORY/$RELEASE_VERSION/$RELEASE_PACKAGE.tar.gz" "$RELEASE_PACKAGE"
(cd "$RELEASE_DIRECTORY/$RELEASE_VERSION" && sha256sum ./*.tar.gz >checksums.txt)
# Mesmo tratamento do build-release.sh: versão embutida no get.sh.
sed "s/@AMPCONTROL_VERSION@/$RELEASE_VERSION/g" "$SCRIPT_DIRECTORY/get.sh" >"$RELEASE_DIRECTORY/get.sh"

get_command="$(printf 'cat %q | AMPCONTROL_DOWNLOAD_BASE=%q bash -s -- --language pt-BR --no-start' \
    "$RELEASE_DIRECTORY/get.sh" "file://$RELEASE_DIRECTORY")"
printf '%s\n' "$(printf '\n'; discord_and_amp_answers "$TOKEN_SECOND")" | script -qec "$get_command" /dev/null
check_installation "$TOKEN_SECOND"
cmp "$PACKAGE_DIRECTORY/ampcontrol" /usr/lib/ampcontrol/ampcontrol ||
    fail "$(msg 'o binário instalado não é o do pacote baixado pelo get.sh' 'the installed binary is not the package downloaded by get.sh')"

# Checksum errado: o get.sh aborta antes de tocar na instalação.
mkdir -p "$BAD_RELEASE_DIRECTORY/$RELEASE_VERSION"
cp "$RELEASE_DIRECTORY/$RELEASE_VERSION/$RELEASE_PACKAGE.tar.gz" "$BAD_RELEASE_DIRECTORY/$RELEASE_VERSION/"
printf 'x' >>"$BAD_RELEASE_DIRECTORY/$RELEASE_VERSION/$RELEASE_PACKAGE.tar.gz"
cp "$RELEASE_DIRECTORY/$RELEASE_VERSION/checksums.txt" "$BAD_RELEASE_DIRECTORY/$RELEASE_VERSION/"
if bad_output="$(AMPCONTROL_DOWNLOAD_BASE="file://$BAD_RELEASE_DIRECTORY" bash "$RELEASE_DIRECTORY/get.sh" --language pt-BR --no-start </dev/null 2>&1)"; then
    fail "$(msg 'o get.sh aceitou um pacote com checksum errado' 'get.sh accepted a package with a wrong checksum')"
fi
[[ "$bad_output" == *checksum* ]] ||
    fail "$(msg 'a recusa não menciona o checksum' 'the rejection does not mention the checksum'): $bad_output"
cmp "$PACKAGE_DIRECTORY/ampcontrol" /usr/lib/ampcontrol/ampcontrol ||
    fail "$(msg 'a instalação mudou apesar do checksum errado' 'the installation changed despite the wrong checksum')"

printf '%s\n' "$(msg 'Ensaio do instalador: OK' 'Installer rehearsal: OK')"
