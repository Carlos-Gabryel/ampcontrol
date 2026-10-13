#!/usr/bin/env bash
# Instalação com um comando: baixa o pacote de release da arquitetura do host,
# confere o SHA-256 e executa o instalador do pacote. Uso:
#   curl -fsSL https://github.com/Carlos-Gabryel/ampcontrol/releases/latest/download/get.sh | sudo bash
# Todos os argumentos são repassados ao scripts/install.sh.

set -Eeuo pipefail

readonly REPOSITORY_URL="https://github.com/Carlos-Gabryel/ampcontrol"
readonly DOWNLOAD_BASE="${AMPCONTROL_DOWNLOAD_BASE:-$REPOSITORY_URL/releases/download}"
# O build da release troca o marcador abaixo pela versão; a comparação é montada
# em duas partes para o sed não reescrevê-la também.
RELEASE_VERSION="${AMPCONTROL_VERSION:-@AMPCONTROL_VERSION@}"
readonly PLACEHOLDER='@AMPCONTROL_''VERSION@'
TEMP_DIRECTORY=""
GET_LANGUAGE="${AMPCONTROL_LANGUAGE:-}"

# Idioma das mensagens deste script: --language, AMPCONTROL_LANGUAGE, $LANG.
previous=""
for argument in "$@"; do
    if [[ "$previous" == "--language" ]]; then GET_LANGUAGE="$argument"; fi
    previous="$argument"
done
if [[ "$GET_LANGUAGE" != "pt-BR" && "$GET_LANGUAGE" != "en-US" ]]; then
    if [[ "${LANG:-}" == pt* ]]; then GET_LANGUAGE="pt-BR"; else GET_LANGUAGE="en-US"; fi
fi

msg() {
    if [[ "$GET_LANGUAGE" == "en-US" ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi
}

cleanup() {
    if [[ -n "$TEMP_DIRECTORY" && -d "$TEMP_DIRECTORY" ]]; then
        rm -rf -- "$TEMP_DIRECTORY"
    fi
}
trap cleanup EXIT

fail() {
    printf '%s: %s\n' "$(msg 'Erro' 'Error')" "$1" >&2
    exit 1
}

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    fail "$(msg 'execute como root, por exemplo' 'run as root, for example'): curl -fsSL $REPOSITORY_URL/releases/latest/download/get.sh | sudo bash"
fi

for required in curl tar sha256sum uname mktemp; do
    command -v "$required" >/dev/null 2>&1 || fail "$(msg 'comando necessário não encontrado' 'required command not found'): $required"
done

if [[ "$RELEASE_VERSION" == "$PLACEHOLDER" ]]; then
    resolved_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$REPOSITORY_URL/releases/latest")" ||
        fail "$(msg 'não foi possível descobrir a última versão' 'could not determine the latest version')"
    RELEASE_VERSION="${resolved_url##*/}"
fi
[[ "$RELEASE_VERSION" =~ ^v[0-9] ]] ||
    fail "$(msg 'versão inválida' 'invalid version'): $RELEASE_VERSION"

case "$(uname -m)" in
    x86_64) ARCH="amd64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    *) fail "$(msg 'arquitetura não suportada' 'unsupported architecture'): $(uname -m)" ;;
esac

PACKAGE_NAME="ampcontrol_${RELEASE_VERSION}_linux_${ARCH}"
TEMP_DIRECTORY="$(mktemp -d)"

printf '%s %s (%s)...\n' "$(msg 'Baixando o AmpControl' 'Downloading AmpControl')" "$RELEASE_VERSION" "$ARCH"
curl -fsSL -o "$TEMP_DIRECTORY/$PACKAGE_NAME.tar.gz" "$DOWNLOAD_BASE/$RELEASE_VERSION/$PACKAGE_NAME.tar.gz" ||
    fail "$(msg 'falha ao baixar o pacote' 'failed to download the package')"
curl -fsSL -o "$TEMP_DIRECTORY/checksums.txt" "$DOWNLOAD_BASE/$RELEASE_VERSION/checksums.txt" ||
    fail "$(msg 'falha ao baixar checksums.txt' 'failed to download checksums.txt')"

# Confere só a linha do pacote baixado (formato "<hash>  ./<nome>" ou "<hash>  <nome>").
(
    cd "$TEMP_DIRECTORY"
    expected_line="$(grep -E "^[0-9a-f]{64}  (\./)?${PACKAGE_NAME}\.tar\.gz\$" checksums.txt || true)"
    [[ -n "$expected_line" ]] ||
        fail "$(msg 'checksum ausente em checksums.txt para' 'checksum missing in checksums.txt for') $PACKAGE_NAME.tar.gz"
    printf '%s\n' "$expected_line" | sha256sum -c - >/dev/null 2>&1 ||
        fail "$(msg 'checksum SHA-256 não confere; instalação abortada' 'SHA-256 checksum mismatch; installation aborted')"
) || exit 1
printf '%s\n' "$(msg 'Checksum SHA-256 conferido.' 'SHA-256 checksum verified.')"

tar -xzf "$TEMP_DIRECTORY/$PACKAGE_NAME.tar.gz" -C "$TEMP_DIRECTORY" ||
    fail "$(msg 'falha ao extrair o pacote' 'failed to extract the package')"
PACKAGE_DIRECTORY="$TEMP_DIRECTORY/$PACKAGE_NAME"
[[ -f "$PACKAGE_DIRECTORY/scripts/install.sh" && -f "$PACKAGE_DIRECTORY/ampcontrol" ]] ||
    fail "$(msg 'pacote incompleto (faltam scripts/install.sh ou ampcontrol)' 'incomplete package (scripts/install.sh or ampcontrol missing)')"

# Com "curl | bash" o stdin é este script; o instalador precisa do terminal.
if ! { : </dev/tty; } 2>/dev/null; then
    fail "$(msg 'sem terminal para as perguntas do instalador; use o método manual (docs/INSTALLATION.md, seção 4)' 'no terminal available for the installer questions; use the manual method (docs/INSTALLATION.en.md, section 4)')"
fi

printf '%s\n' "$(msg 'Executando o instalador...' 'Running the installer...')"
status=0
"$PACKAGE_DIRECTORY/scripts/install.sh" "$@" </dev/tty || status=$?
exit "$status"
