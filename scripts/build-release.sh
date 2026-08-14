#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly SCRIPT_DIRECTORY
PROJECT_DIRECTORY="$(cd -- "$SCRIPT_DIRECTORY/.." && pwd -P)"
readonly PROJECT_DIRECTORY
readonly VERSION="${1:-dev}"
readonly DIST_DIRECTORY="${AMPCONTROL_DIST_DIRECTORY:-$PROJECT_DIRECTORY/dist}"
readonly LANGUAGE="${AMPCONTROL_LANGUAGE:-pt-BR}"
msg() { if [[ "$LANGUAGE" == "en-US" ]]; then printf '%s' "$2"; else printf '%s' "$1"; fi; }

[[ "$VERSION" =~ ^[A-Za-z0-9._-]+$ ]] || {
    printf '%s: %s\n' "$(msg 'Versão inválida' 'Invalid version')" "$VERSION" >&2
    exit 2
}
command -v go >/dev/null 2>&1 || {
    printf '%s\n' "$(msg 'Go não encontrado.' 'Go was not found.')" >&2
    exit 1
}

mkdir -p "$DIST_DIRECTORY"
if [[ -n "$(find "$DIST_DIRECTORY" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    printf '%s: %s\n' "$(msg 'O diretório de saída precisa estar vazio' 'The output directory must be empty')" "$DIST_DIRECTORY" >&2
    exit 1
fi

for architecture in amd64 arm64; do
    package_name="ampcontrol_${VERSION}_linux_${architecture}"
    package_directory="$DIST_DIRECTORY/$package_name"
    mkdir -p "$package_directory/config" "$package_directory/packaging/systemd" "$package_directory/scripts"

    (
        cd "$PROJECT_DIRECTORY"
        CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" \
            go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$package_directory/ampcontrol" ./cmd/ampcontrol
    )
    install -m 0644 \
        "$PROJECT_DIRECTORY/README.md" \
        "$PROJECT_DIRECTORY/README.en.md" \
        "$PROJECT_DIRECTORY/LICENSE" \
        "$PROJECT_DIRECTORY/SECURITY.md" \
        "$PROJECT_DIRECTORY/SECURITY.en.md" \
        "$PROJECT_DIRECTORY/CONTRIBUTING.md" \
        "$PROJECT_DIRECTORY/CONTRIBUTING.en.md" \
        "$PROJECT_DIRECTORY/THIRD_PARTY_NOTICES.md" \
        "$PROJECT_DIRECTORY/THIRD_PARTY_NOTICES.en.md" \
        "$package_directory/"
    cp -a "$PROJECT_DIRECTORY/docs" "$package_directory/docs"
    install -m 0644 "$PROJECT_DIRECTORY/config/ampcontrol.example.toml" "$PROJECT_DIRECTORY/config/ampcontrol.example.en.toml" "$PROJECT_DIRECTORY/config/idle.json" "$package_directory/config/"
    install -m 0644 "$PROJECT_DIRECTORY/packaging/systemd/ampcontrol.service" "$package_directory/packaging/systemd/"
    install -m 0755 \
        "$PROJECT_DIRECTORY/scripts/install.sh" \
        "$PROJECT_DIRECTORY/scripts/migrate-legacy.sh" \
        "$PROJECT_DIRECTORY/scripts/migration-preflight.sh" \
        "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" \
        "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" \
        "$PROJECT_DIRECTORY/scripts/legacy_config.py" \
        "$package_directory/scripts/"

    tar -C "$DIST_DIRECTORY" -czf "$DIST_DIRECTORY/$package_name.tar.gz" "$package_name"
    rm -rf -- "$package_directory"
done

(
    cd "$DIST_DIRECTORY"
    sha256sum ./*.tar.gz >checksums.txt
)
printf '%s %s\n' "$(msg 'Artefatos criados em' 'Artifacts created at')" "$DIST_DIRECTORY"
